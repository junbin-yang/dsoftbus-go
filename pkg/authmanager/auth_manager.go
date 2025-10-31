package authmanager

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// AuthManager 管理所有认证连接和会话的核心管理器
type AuthManager struct {
	authPort       int                // 认证端口
	sessionPort    int                // 会话端口
	localDevInfo   *DeviceInfo        // 本地设备信息
	connMap        map[int]*AuthConn  // 认证连接映射（fd -> AuthConn）
	connMapMu      sync.RWMutex       // connMap的读写锁
	netConnMap     map[int]net.Conn   // 网络连接映射（fd -> net.Conn）
	netConnMapMu   sync.RWMutex       // netConnMap的读写锁
	sessionKeyMgr  *SessionKeyManager // 会话密钥管理器
	authSessions   []*AuthSession     // 认证会话数组（固定大小）
	authSessionsMu sync.Mutex         // authSessions的互斥锁
	nextSessionID  uint32             // 下一个可用的会话ID
	authInterface  *AuthInterface     // 认证接口（与HiChain模块交互）
}

// NewAuthManager 创建一个新的认证管理器
// 参数：
//   - authPort：认证端口
//   - sessionPort：会话端口
//   - devInfo：本地设备信息
// 返回：
//   - 初始化后的AuthManager实例
func NewAuthManager(authPort, sessionPort int, devInfo *DeviceInfo) *AuthManager {
	mgr := &AuthManager{
		authPort:      authPort,
		sessionPort:   sessionPort,
		localDevInfo:  devInfo,
		connMap:       make(map[int]*AuthConn),
		netConnMap:    make(map[int]net.Conn),
		sessionKeyMgr: NewSessionKeyManager(),
		authSessions:  make([]*AuthSession, AuthSessionMaxNum),
		nextSessionID: 1, // 会话ID从1开始编号
	}
	mgr.authInterface = NewAuthInterface(mgr) // 初始化认证接口
	return mgr
}

// AddAuthConn 添加新的认证连接
// 参数：
//   - fd：文件描述符（唯一标识连接）
//   - ip：设备IP地址
// 返回：
//   - 新创建的AuthConn实例
//   - 错误（若连接数达上限则返回ErrMaxConnectionsReached）
func (m *AuthManager) AddAuthConn(fd int, ip string) (*AuthConn, error) {
	m.connMapMu.Lock()
	defer m.connMapMu.Unlock()

	// 检查是否已达最大连接数限制
	if len(m.connMap) >= AuthConnMaxNum {
		return nil, ErrMaxConnectionsReached
	}

	// 创建新的认证连接
	conn := &AuthConn{
		Fd:          fd,
		DeviceIP:    ip,
		OnlineState: OnlineUnknown, // 初始状态为未知
		AuthState:   AuthUnknown,   // 初始认证状态为未知
		DB: DataBuffer{
			Buf:  make([]byte, DefaultBufSize),
			Size: DefaultBufSize,
			Used: 0,
		},
	}

	m.connMap[fd] = conn
	return conn, nil
}

// FindAuthConnByFd 通过文件描述符查找认证连接
// 参数：
//   - fd：文件描述符
// 返回：
//   - 找到的AuthConn（若不存在则返回nil）
func (m *AuthManager) FindAuthConnByFd(fd int) *AuthConn {
	m.connMapMu.RLock()
	defer m.connMapMu.RUnlock()
	return m.connMap[fd]
}

// GetOnlineAuthConnByIP 通过IP查找在线的认证连接
// 参数：
//   - ip：设备IP地址
// 返回：
//   - 找到的在线AuthConn（若不存在则返回nil）
func (m *AuthManager) GetOnlineAuthConnByIP(ip string) *AuthConn {
	m.connMapMu.RLock()
	defer m.connMapMu.RUnlock()

	for _, conn := range m.connMap {
		if conn.DeviceIP == ip && conn.OnlineState == OnlineYes {
			return conn
		}
	}
	return nil
}

// RemoveAuthConn 移除认证连接
// 参数：
//   - fd：文件描述符
func (m *AuthManager) RemoveAuthConn(fd int) {
	m.connMapMu.Lock()
	defer m.connMapMu.Unlock()

	if conn, ok := m.connMap[fd]; ok {
		// 清除关联的会话密钥
		m.sessionKeyMgr.ClearSessionKeyByFd(fd)
		// 从映射中删除连接
		delete(m.connMap, fd)
		log.Infof("[AUTH] 已移除连接 fd=%d, ip=%s", conn.Fd, conn.DeviceIP)
	}
}

// GetAuthSessionBySeqID 通过序列号查找认证会话
// 参数：
//   - seqID：序列号
// 返回：
//   - 找到的AuthSession（若不存在则返回nil）
func (m *AuthManager) GetAuthSessionBySeqID(seqID int64) *AuthSession {
	m.authSessionsMu.Lock()
	defer m.authSessionsMu.Unlock()

	for i := 0; i < AuthSessionMaxNum; i++ {
		if m.authSessions[i] != nil && m.authSessions[i].IsUsed && m.authSessions[i].SeqID == seqID {
			return m.authSessions[i]
		}
	}
	return nil
}

// GetAuthSessionBySessionID 通过会话ID查找认证会话
// 参数：
//   - sessionID：会话ID
// 返回：
//   - 找到的AuthSession（若不存在则返回nil）
func (m *AuthManager) GetAuthSessionBySessionID(sessionID uint32) *AuthSession {
	m.authSessionsMu.Lock()
	defer m.authSessionsMu.Unlock()

	for i := 0; i < AuthSessionMaxNum; i++ {
		if m.authSessions[i] != nil && m.authSessions[i].IsUsed && m.authSessions[i].SessionID == sessionID {
			return m.authSessions[i]
		}
	}
	return nil
}

// CreateAuthSession 创建新的认证会话
// 参数：
//   - conn：关联的认证连接
//   - seqID：序列号
// 返回：
//   - 新创建的AuthSession（若会话数达上限则返回nil）
func (m *AuthManager) CreateAuthSession(conn *AuthConn, seqID int64) *AuthSession {
	m.authSessionsMu.Lock()
	defer m.authSessionsMu.Unlock()

	// 查找空闲的会话槽位
	for i := 0; i < AuthSessionMaxNum; i++ {
		if m.authSessions[i] == nil || !m.authSessions[i].IsUsed {
			session := &AuthSession{
				IsUsed:    true,
				SeqID:     seqID,
				SessionID: m.nextSessionID,
				Conn:      conn,
			}
			m.authSessions[i] = session
			m.nextSessionID++ // 递增会话ID
			return session
		}
	}
	return nil
}

// DeleteAuthSession 通过会话ID删除认证会话
// 参数：
//   - sessionID：会话ID
func (m *AuthManager) DeleteAuthSession(sessionID uint32) {
	m.authSessionsMu.Lock()
	defer m.authSessionsMu.Unlock()

	for i := 0; i < AuthSessionMaxNum; i++ {
		if m.authSessions[i] != nil && m.authSessions[i].SessionID == sessionID {
			m.authSessions[i].IsUsed = false
			m.authSessions[i] = nil // 释放会话
			break
		}
	}
}

// DecryptMessage 解密接收到的消息
// 参数：
//   - module：模块类型
//   - data：加密的消息数据
// 返回：
//   - 解密后的明文数据
//   - 错误（解密失败或密钥不存在时）
func (m *AuthManager) DecryptMessage(module int, data []byte) ([]byte, error) {
	// 不需要加密的模块直接返回原始数据
	if !ModuleUseCipherText(module) {
		return data, nil
	}

	// 对于支持加密的模块，检查数据长度
	// 如果数据长度不足以包含加密头部，说明是明文（初始认证阶段）
	if len(data) < MessageEncryptOverheadLen {
		// 数据太短，不可能是加密数据，当作明文处理
		return data, nil
		//return nil, ErrInvalidMessage
	}

	// 提取密钥索引（前4字节）
	index := int(binary.LittleEndian.Uint32(data[:4]))
	skey := m.sessionKeyMgr.GetSessionKeyByIndex(index)
	if skey == nil {
		//return nil, fmt.Errorf("未找到索引为%d的会话密钥", index)
		// 没有找到会话密钥，说明还在初始认证阶段，当作明文处理
		return data, nil
	}

	// 找到会话密钥，尝试解密数据（跳过4字节索引）
	plaintext, err := DecryptTransData(skey.Key[:], data[4:])
	if err != nil {
		// 解密失败，可能数据损坏或密钥不匹配
		return nil, err
	}

	return plaintext, nil
}

// OnVerifyIP 处理IP验证消息
// 参数：
//   - conn：认证连接
//   - netConn：网络连接
//   - seq：序列号
//   - request：IP验证请求消息
func (m *AuthManager) OnVerifyIP(conn *AuthConn, netConn net.Conn, seq int64, request *VerifyIPMsg) {
	log.Infof("[AUTH] 收到来自%s的IP验证请求", conn.DeviceIP)

	// 本地支持的版本信息
	connInfo := &ConnInfo{
		MaxVersion: BusV2,
		MinVersion: BusV2,
	}

	// 协商版本（取双方支持的交集）
	maxVersion := request.BusMaxVersion
	minVersion := request.BusMinVersion
	if maxVersion > connInfo.MaxVersion {
		maxVersion = connInfo.MaxVersion
	}
	if minVersion < connInfo.MinVersion {
		minVersion = connInfo.MinVersion
	}
	// 更新连接的版本和端口信息
	conn.BusVersion = maxVersion
	conn.AuthPort = request.AuthPort
	conn.SessionPort = request.SessionPort

	// 构建回复消息
	reply := &VerifyIPMsg{
		Code:          CodeVerifyIP,
		BusMaxVersion: maxVersion,
		BusMinVersion: minVersion,
		AuthPort:      m.authPort,
		SessionPort:   m.sessionPort,
		ConnCap:       DeviceConnCapWifi, // 支持WiFi连接
		DeviceName:    m.localDevInfo.DeviceName,
		DeviceType:    DeviceTypeDefault,
		DeviceID:      m.localDevInfo.DeviceID,
		VersionType:   m.localDevInfo.Version,
	}

	// 发送回复
	err := AuthConnPostMessage(netConn, ModuleConnection, FlagReply, seq, reply, nil)
	if err != nil {
		log.Errorf("[AUTH] IP验证回复发送失败: %v", err)
		return
	}

	// 更新连接状态为在线
	conn.OnlineState = OnlineYes
	log.Info("[AUTH] IP验证完成，设备已在线")
}

// OnVerifyDeviceID 处理设备ID验证消息
// 参数：
//   - conn：认证连接
//   - netConn：网络连接
//   - seq：序列号
//   - request：设备ID验证请求消息
func (m *AuthManager) OnVerifyDeviceID(conn *AuthConn, netConn net.Conn, seq int64, request *VerifyDeviceIDMsg) {
	log.Infof("[AUTH] 收到来自%s的设备ID验证请求，DeviceID=%s", conn.DeviceIP, request.DeviceID)

	// 构建回复消息（返回本地设备ID）
	reply := &VerifyDeviceIDMsg{
		Code:     CodeVerifyDevID,
		DeviceID: m.localDevInfo.DeviceID,
	}

	// 发送回复
	err := AuthConnPostMessage(netConn, ModuleConnection, FlagReply, seq, reply, nil)
	if err != nil {
		log.Errorf("[AUTH] 设备ID验证回复发送失败: %v", err)
	}
}

// OnMsgOpenChannelReq 处理打开通道请求
// 参数：
//   - conn：认证连接
//   - netConn：网络连接
//   - seq：序列号
//   - msg：通道打开请求消息
func (m *AuthManager) OnMsgOpenChannelReq(conn *AuthConn, netConn net.Conn, seq int64, msg *GetDeviceIDMsg) {
	log.Infof("[AUTH] 收到来自%s的通道打开请求", conn.DeviceIP)

	// 从请求中提取设备信息并更新连接
	conn.DeviceID = msg.Data
	conn.AuthID = msg.DeviceID

	// 构建回复消息（返回本地设备ID）
	reply := &GetDeviceIDMsg{
		Cmd:      CmdRetAuthInfo,
		Data:     m.localDevInfo.DeviceID,
		DeviceID: m.localDevInfo.DeviceID,
	}

	// 发送回复
	err := AuthConnPostMessage(netConn, ModuleTrustEngine, FlagReply, seq, reply, nil)
	if err != nil {
		log.Errorf("[AUTH] 通道打开请求回复发送失败: %v", err)
		return
	}

	log.Info("[AUTH] 通道打开请求处理完成")
}

// OnModuleMessageReceived 处理特定模块的消息
// 参数：
//   - conn：认证连接
//   - netConn：网络连接
//   - pkt：数据包头部
//   - msgData：消息数据（已解密）
func (m *AuthManager) OnModuleMessageReceived(conn *AuthConn, netConn net.Conn, pkt *Packet, msgData []byte) {
	switch pkt.Module {
	case ModuleTrustEngine:
		// 非回复消息才处理（避免重复处理自己发出的消息）
		if (pkt.Flags & FlagReply) == 0 {
			var msg GetDeviceIDMsg
			if err := json.Unmarshal(msgData, &msg); err != nil {
				log.Errorf("[AUTH] 解析GetDeviceIDMsg失败: %v", err)
				return
			}
			m.OnMsgOpenChannelReq(conn, netConn, pkt.Seq, &msg)
		}

	case ModuleConnection:
		// 解析CODE字段确定消息类型
		var codeMsg struct {
			Code int `json:"CODE"`
		}
		if err := json.Unmarshal(msgData, &codeMsg); err != nil {
			log.Errorf("[AUTH] 解析CODE字段失败: %v", err)
			return
		}

		// 根据CODE分发到对应处理函数
		switch codeMsg.Code {
		case CodeVerifyIP:
			var msg VerifyIPMsg
			if err := json.Unmarshal(msgData, &msg); err != nil {
				log.Errorf("[AUTH] 解析VerifyIPMsg失败: %v", err)
				return
			}
			m.OnVerifyIP(conn, netConn, pkt.Seq, &msg)

		case CodeVerifyDevID:
			var msg VerifyDeviceIDMsg
			if err := json.Unmarshal(msgData, &msg); err != nil {
				log.Errorf("[AUTH] 解析VerifyDeviceIDMsg失败: %v", err)
				return
			}
			m.OnVerifyDeviceID(conn, netConn, pkt.Seq, &msg)
		}
	}
}

// OnDataReceived 处理接收到的数据包
// 参数：
//   - conn：认证连接
//   - netConn：网络连接
//   - pkt：数据包头部
//   - data：数据包负载
func (m *AuthManager) OnDataReceived(conn *AuthConn, netConn net.Conn, pkt *Packet, data []byte) {
	log.Infof("[AUTH] 收到数据 模块=%d, 序列号=%d, 数据长度=%d", pkt.Module, pkt.Seq, pkt.DataLen)

	// 对于AUTH_SDK模块，交由HiChain处理认证
	if pkt.Module > ModuleHiChain && pkt.Module <= ModuleAuthSDK {
		log.Info("[AUTH] 收到AUTH_SDK数据，交由HiChain处理")

		// 获取或创建认证会话
		session := m.GetAuthSessionBySeqID(pkt.Seq)
		if session == nil {
			session = m.CreateAuthSession(conn, pkt.Seq)
			if session == nil {
				log.Error("[AUTH] 创建认证会话失败")
				return
			}
		}

		// 交由HiChain处理数据
		err := m.authInterface.ProcessReceivedData(session.SessionID, data)
		if err != nil {
			log.Errorf("[AUTH] HiChain处理失败: %v", err)
		}
		return
	}

	// 解密消息（如果需要）
	msgData, err := m.DecryptMessage(pkt.Module, data)
	if err != nil {
		log.Errorf("[AUTH] 消息解密失败: %v", err)
		return
	}

	// 分发给模块特定处理函数
	m.OnModuleMessageReceived(conn, netConn, pkt, msgData)
}

// RegisterNetConn 注册网络连接
// 参数：
//   - fd：文件描述符
//   - conn：网络连接
func (m *AuthManager) RegisterNetConn(fd int, conn net.Conn) {
	m.netConnMapMu.Lock()
	defer m.netConnMapMu.Unlock()
	m.netConnMap[fd] = conn
}

// UnregisterNetConn 注销网络连接
// 参数：
//   - fd：文件描述符
func (m *AuthManager) UnregisterNetConn(fd int) {
	m.netConnMapMu.Lock()
	defer m.netConnMapMu.Unlock()
	delete(m.netConnMap, fd)
}

// getNetConnByFd 通过文件描述符获取网络连接
// 参数：
//   - fd：文件描述符
// 返回：
//   - 对应的net.Conn（若不存在则返回nil）
func (m *AuthManager) getNetConnByFd(fd int) net.Conn {
	m.netConnMapMu.RLock()
	defer m.netConnMapMu.RUnlock()
	return m.netConnMap[fd]
}

// ProcessPackets 处理缓冲区中接收到的数据包
// 参数：
//   - conn：认证连接
//   - netConn：网络连接
//   - buf：数据缓冲区
//   - used：已使用的缓冲区大小
// 返回：
//   - 已处理的字节数（-1表示解析失败）
func (m *AuthManager) ProcessPackets(conn *AuthConn, netConn net.Conn, buf []byte, used int) int {
	processed := 0 // 已处理的字节数

	// 循环处理缓冲区中的完整数据包
	for processed+PacketHeadSize < used {
		// 解析数据包头部
		pkt, err := ParsePacketHead(buf, processed)
		if err != nil {
			log.Errorf("[AUTH] 解析数据包头部失败: %v", err)
			return -1
		}

		dataLen := pkt.DataLen
		// 检查数据是否完整（头部+数据）
		if dataLen > PacketDataSize || processed+PacketHeadSize+dataLen > used {
			// 数据不完整，退出循环等待更多数据
			break
		}

		// 移动指针到数据部分
		processed += PacketHeadSize
		data := buf[processed : processed+dataLen]

		// 处理数据包
		m.OnDataReceived(conn, netConn, pkt, data)
		processed += dataLen
	}

	return processed
}
