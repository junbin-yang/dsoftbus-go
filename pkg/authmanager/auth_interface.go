package authmanager

import (
	"net"
	"sync"

	"github.com/junbin-yang/dsoftbus-go/pkg/authmanager/hichain"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// AuthInterface 管理HiChain认证会话的接口，作为AuthManager与HiChain模块的中间层
type AuthInterface struct {
	authMgr     *AuthManager                      // 关联的认证管理器
	hcHandles   map[uint32]*hichain.HiChainHandle // HiChain会话句柄映射（sessionID -> 句柄）
	hcHandlesMu sync.Mutex                        // hcHandles的互斥锁
}

// NewAuthInterface 创建新的认证接口实例
// 参数：
//   - authMgr：关联的认证管理器
// 返回：
//   - 初始化后的AuthInterface实例
func NewAuthInterface(authMgr *AuthManager) *AuthInterface {
	return &AuthInterface{
		authMgr:   authMgr,
		hcHandles: make(map[uint32]*hichain.HiChainHandle),
	}
}

// OnTransmit 通过连接发送HiChain数据（实现HiChain的传输回调）
// 参数：
//   - identity：会话标识信息
//   - data：待发送的数据
// 返回：
//   - 错误信息（发送失败时）
func (ai *AuthInterface) OnTransmit(identity *hichain.SessionIdentity, data []byte) error {
	log.Infof("[AUTH_INTERFACE] 发送数据: 会话=%d, 长度=%d", identity.SessionID, len(data))

	// 查找对应的认证会话
	session := ai.authMgr.GetAuthSessionBySessionID(identity.SessionID)
	if session == nil || session.Conn == nil {
		return ErrSessionNotFound
	}

	// 获取网络连接
	conn := ai.getNetConnByFd(session.Conn.Fd)
	if conn == nil {
		return ErrConnectionClosed
	}

	// 通过AUTH_SDK模块发送数据
	return AuthConnPostBytes(conn, ModuleAuthSDK, 0, session.SeqID, data, nil)
}

// GetProtocolParams 获取协议参数（实现HiChain的参数获取回调）
// 参数：
//   - identity：会话标识信息
//   - operationCode：操作码
// 返回：
//   - 协议参数
//   - 错误信息（会话不存在时）
func (ai *AuthInterface) GetProtocolParams(identity *hichain.SessionIdentity, operationCode int32) (*hichain.ProtocolParams, error) {
	log.Infof("[AUTH_INTERFACE] 获取协议参数: 会话=%d, 操作码=%d", identity.SessionID, operationCode)

	// 查找对应的认证会话
	session := ai.authMgr.GetAuthSessionBySessionID(identity.SessionID)
	if session == nil || session.Conn == nil {
		return nil, ErrSessionNotFound
	}

	// 构建并返回协议参数
	params := &hichain.ProtocolParams{
		KeyLength:  AuthSessionKeyLen,                // 会话密钥长度（16字节）
		SelfAuthID: ai.authMgr.localDevInfo.DeviceID, // 本地设备ID
		PeerAuthID: session.Conn.AuthID,              // 对端设备认证ID
	}

	return params, nil
}

// SetSessionKey 存储派生的会话密钥（实现HiChain的密钥设置回调）
// 参数：
//   - identity：会话标识信息
//   - sessionKey：HiChain生成的会话密钥
// 返回：
//   - 错误信息（存储失败时）
func (ai *AuthInterface) SetSessionKey(identity *hichain.SessionIdentity, sessionKey *hichain.SessionKey) error {
	log.Infof("[AUTH_INTERFACE] 设置会话密钥: 会话=%d, 密钥长度=%d", identity.SessionID, sessionKey.Length)

	// 查找对应的认证会话
	session := ai.authMgr.GetAuthSessionBySessionID(identity.SessionID)
	if session == nil || session.Conn == nil {
		return ErrSessionNotFound
	}

	// 将会话密钥添加到密钥管理器
	// 重要：使用设备ID作为标识，索引为0
	// 在HarmonyOS中，认证阶段协商的密钥用于后续的Session建立
	index := 0
	deviceID := session.Conn.DeviceID
	err := ai.authMgr.sessionKeyMgr.AddSessionKey(deviceID, index, sessionKey.Key)
	if err != nil {
		log.Errorf("[AUTH_INTERFACE] 添加会话密钥失败: %v", err)
		return err
	}

	log.Infof("[AUTH_INTERFACE] 会话密钥添加成功, deviceID=%s, index=%d", deviceID, index)
	return nil
}

// SetServiceResult 处理认证结果（实现HiChain的结果通知回调）
// 参数：
//   - identity：会话标识信息
//   - result：认证结果（0表示成功）
// 返回：
//   - 错误信息（处理失败时）
func (ai *AuthInterface) SetServiceResult(identity *hichain.SessionIdentity, result int32) error {
	log.Infof("[AUTH_INTERFACE] 认证结果: 会话=%d, 结果=%d", identity.SessionID, result)

	if result == hichain.HCOk {
		log.Infof("[AUTH_INTERFACE] 会话%d认证成功", identity.SessionID)
	} else {
		log.Errorf("[AUTH_INTERFACE] 会话%d认证失败", identity.SessionID)

		// 认证失败时清除会话密钥
		session := ai.authMgr.GetAuthSessionBySessionID(identity.SessionID)
		if session != nil {
			ai.authMgr.sessionKeyMgr.ClearSessionKeyBySeq(session.SeqID)
		}
	}

	// 清理HiChain句柄
	ai.destroyHiChain(identity.SessionID)

	// 删除认证会话
	ai.authMgr.DeleteAuthSession(identity.SessionID)

	return nil
}

// ConfirmReceiveRequest 确认接收请求（实现HiChain的接收确认回调）
// 参数：
//   - identity：会话标识信息
//   - operationCode：操作码
// 返回：
//   - 处理结果（HCOk表示成功）
func (ai *AuthInterface) ConfirmReceiveRequest(identity *hichain.SessionIdentity, operationCode int32) int32 {
	log.Infof("[AUTH_INTERFACE] 确认接收请求: 会话=%d, 操作码=%d", identity.SessionID, operationCode)
	return hichain.HCOk // 总是返回成功
}

// initHiChain 初始化指定会话的HiChain实例
// 参数：
//   - sessionID：会话ID
// 返回：
//   - 错误信息（初始化失败时）
func (ai *AuthInterface) initHiChain(sessionID uint32) error {
	ai.hcHandlesMu.Lock()
	defer ai.hcHandlesMu.Unlock()

	// 检查是否已初始化
	if _, exists := ai.hcHandles[sessionID]; exists {
		return nil
	}

	log.Infof("[AUTH_INTERFACE] 初始化会话%d的HiChain", sessionID)

	// 创建会话标识
	identity := &hichain.SessionIdentity{
		SessionID:     sessionID,
		PackageName:   AuthDefaultID, // 默认包名
		ServiceType:   AuthDefaultID, // 默认服务类型
		OperationCode: 0,             // 操作码（预留）
	}

	// 创建HiChain回调函数（绑定当前AuthInterface的方法）
	callback := &hichain.HCCallBack{
		OnTransmit:            ai.OnTransmit,
		GetProtocolParams:     ai.GetProtocolParams,
		SetSessionKey:         ai.SetSessionKey,
		SetServiceResult:      ai.SetServiceResult,
		ConfirmReceiveRequest: ai.ConfirmReceiveRequest,
	}

	// 获取HiChain实例（设备类型为配件）
	handle, err := hichain.GetInstance(identity, hichain.HCAccessory, callback)
	if err != nil {
		log.Errorf("[AUTH_INTERFACE] 获取HiChain实例失败: %v", err)
		return err
	}

	ai.hcHandles[sessionID] = handle
	log.Info("[AUTH_INTERFACE] HiChain初始化成功")

	return nil
}

// destroyHiChain 销毁指定会话的HiChain实例
// 参数：
//   - sessionID：会话ID
func (ai *AuthInterface) destroyHiChain(sessionID uint32) {
	ai.hcHandlesMu.Lock()
	defer ai.hcHandlesMu.Unlock()

	if handle, exists := ai.hcHandles[sessionID]; exists {
		hichain.Destroy(&handle)        // 调用HiChain的销毁方法
		delete(ai.hcHandles, sessionID) // 从映射中移除
		log.Infof("[AUTH_INTERFACE] 销毁会话%d的HiChain", sessionID)
	}
}

// ProcessReceivedData 处理接收到的HiChain数据
// 参数：
//   - sessionID：会话ID
//   - data：接收到的数据
// 返回：
//   - 错误信息（处理失败时）
func (ai *AuthInterface) ProcessReceivedData(sessionID uint32, data []byte) error {
	// 初始化HiChain（如未初始化）
	if err := ai.initHiChain(sessionID); err != nil {
		ai.authMgr.DeleteAuthSession(sessionID)
		return err
	}

	// 获取HiChain句柄
	ai.hcHandlesMu.Lock()
	handle := ai.hcHandles[sessionID]
	ai.hcHandlesMu.Unlock()

	if handle == nil {
		return ErrSessionNotFound
	}

	// 交由HiChain处理数据
	return handle.ReceiveData(data)
}

// getNetConnByFd 通过文件描述符获取网络连接（辅助方法）
// 参数：
//   - fd：文件描述符
// 返回：
//   - 对应的网络连接（net.Conn）
func (ai *AuthInterface) getNetConnByFd(fd int) net.Conn {
	// 实际实现由tcp_server提供，此处通过AuthManager获取
	return ai.authMgr.getNetConnByFd(fd)
}
