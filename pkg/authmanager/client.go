package authmanager

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// AuthClient 用于发起认证的客户端，负责与远程设备建立连接并发送认证相关请求
type AuthClient struct {
	localDevInfo *DeviceInfo // 本地设备信息
	conn         net.Conn    // 与远程设备的网络连接
	authConn     *AuthConn   // 认证连接信息
	seqNum       int64       // 消息序列号（用于标识请求顺序）
}

// NewAuthClient 创建新的认证客户端实例
// 参数：
//   - devInfo：本地设备信息
// 返回：
//   - 初始化后的AuthClient实例
func NewAuthClient(devInfo *DeviceInfo) *AuthClient {
	return &AuthClient{
		localDevInfo: devInfo,
		seqNum:       1, // 序列号从1开始
	}
}

// Connect 连接到远程设备
// 参数：
//   - remoteIP：远程设备IP地址
//   - remotePort：远程设备端口
// 返回：
//   - 错误信息（连接失败时）
func (c *AuthClient) Connect(remoteIP string, remotePort int) error {
	addr := fmt.Sprintf("%s:%d", remoteIP, remotePort)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("连接到%s失败: %v", addr, err)
	}

	c.conn = conn
	c.authConn = &AuthConn{
		DeviceIP:    remoteIP,
		OnlineState: OnlineUnknown, // 初始状态为未知
		AuthState:   AuthUnknown,   // 初始认证状态为未知
	}

	log.Infof("[AUTH] 已连接到%s", addr)
	return nil
}

// Close 关闭与远程设备的连接
func (c *AuthClient) Close() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// SendGetAuthInfo 发送获取认证信息的请求
// 返回：
//   - 错误信息（发送失败时）
func (c *AuthClient) SendGetAuthInfo() error {
	// 构建获取认证信息的消息
	msg := &GetDeviceIDMsg{
		Cmd:      CmdGetAuthInfo,          // 命令类型：获取认证信息
		Data:     c.localDevInfo.DeviceID, // 本地设备ID
		DeviceID: c.localDevInfo.DeviceID, // 本地设备ID
	}

	// 通过信任引擎模块发送消息
	err := AuthConnPostMessage(c.conn, ModuleTrustEngine, 0, c.seqNum, msg, nil)
	if err != nil {
		return fmt.Errorf("发送获取认证信息请求失败: %v", err)
	}

	c.seqNum++ // 递增序列号
	log.Info("[AUTH] 已发送GetAuthInfo请求")
	return nil
}

// SendVerifyIP 发送IP验证请求
// 参数：
//   - authPort：本地认证端口
//   - sessionPort：本地会话端口
// 返回：
//   - 错误信息（发送失败时）
func (c *AuthClient) SendVerifyIP(authPort, sessionPort int) error {
	// 构建IP验证消息
	msg := &VerifyIPMsg{
		Code:          CodeVerifyIP,              // 消息类型：IP验证
		BusMaxVersion: BusV2,                     // 支持的最高总线版本
		BusMinVersion: BusV2,                     // 支持的最低总线版本
		AuthPort:      authPort,                  // 本地认证端口
		SessionPort:   sessionPort,               // 本地会话端口
		ConnCap:       DeviceConnCapWifi,         // 连接能力：支持WiFi
		DeviceName:    c.localDevInfo.DeviceName, // 本地设备名称
		DeviceType:    DeviceTypeDefault,         // 设备类型：默认类型
		DeviceID:      c.localDevInfo.DeviceID,   // 本地设备ID
		VersionType:   c.localDevInfo.Version,    // 本地设备版本
	}

	// 通过连接管理模块发送消息
	err := AuthConnPostMessage(c.conn, ModuleConnection, 0, c.seqNum, msg, nil)
	if err != nil {
		return fmt.Errorf("发送IP验证请求失败: %v", err)
	}

	c.seqNum++ // 递增序列号
	log.Info("[AUTH] 已发送VerifyIP请求")
	return nil
}

// SendVerifyDeviceID 发送设备ID验证请求
// 返回：
//   - 错误信息（发送失败时）
func (c *AuthClient) SendVerifyDeviceID() error {
	// 构建设备ID验证消息
	msg := &VerifyDeviceIDMsg{
		Code:     CodeVerifyDevID,         // 消息类型：设备ID验证
		DeviceID: c.localDevInfo.DeviceID, // 本地设备ID
	}

	// 通过连接管理模块发送消息
	err := AuthConnPostMessage(c.conn, ModuleConnection, 0, c.seqNum, msg, nil)
	if err != nil {
		return fmt.Errorf("发送设备ID验证请求失败: %v", err)
	}

	c.seqNum++ // 递增序列号
	log.Info("[AUTH] 已发送VerifyDeviceID请求")
	return nil
}

// ReceiveResponse 接收并处理远程设备的响应
// 返回：
//   - 错误信息（接收或处理失败时）
func (c *AuthClient) ReceiveResponse() error {
	buf := make([]byte, DefaultBufSize) // 缓冲区

	// 设置5秒读取超时
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{}) // 退出时清除超时设置

	// 读取响应数据
	n, err := c.conn.Read(buf)
	if err != nil {
		return fmt.Errorf("接收响应失败: %v", err)
	}

	// 验证数据包长度是否至少包含头部
	if n < PacketHeadSize {
		return fmt.Errorf("响应过短: %d字节", n)
	}

	// 解析数据包头部
	pkt, err := ParsePacketHead(buf, 0)
	if err != nil {
		return fmt.Errorf("解析数据包失败: %v", err)
	}

	// 验证数据包是否完整
	if n < PacketHeadSize+pkt.DataLen {
		return fmt.Errorf("数据包不完整: 已接收%d，需要%d", n, PacketHeadSize+pkt.DataLen)
	}

	// 提取数据部分
	data := buf[PacketHeadSize : PacketHeadSize+pkt.DataLen]

	// 清理C字符串终止符（真实鸿蒙设备可能在JSON末尾添加\x00）
	data = coap.CleanJSONData(data)

	log.Infof("[AUTH] 收到响应: 模块=%d, 序列号=%d, 标志位=%d, 数据长度=%d\n", pkt.Module, pkt.Seq, pkt.Flags, pkt.DataLen)

	// 根据模块类型解析响应
	switch pkt.Module {
	case ModuleTrustEngine:
		var msg GetDeviceIDMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("解析GetDeviceIDMsg失败: %v", err)
		}
		log.Infof("[AUTH] 收到GetDeviceIDMsg: 命令=%s, 数据=%s, 设备ID=%s", msg.Cmd, msg.Data, msg.DeviceID)

		// 提取真实的设备ID（处理{"UDID":"xxx"}格式）
		extractedDeviceID := coap.ExtractDeviceID(msg.DeviceID)
		extractedData := coap.ExtractDeviceID(msg.Data)

		// 更新认证连接信息
		c.authConn.DeviceID = extractedData
		c.authConn.AuthID = extractedDeviceID

		log.Infof("[AUTH] 提取的设备ID: Data=%s, AuthID=%s", extractedData, extractedDeviceID)

	case ModuleConnection:
		// 先解析CODE字段确定消息类型
		var codeMsg struct {
			Code int `json:"CODE"`
		}
		if err := json.Unmarshal(data, &codeMsg); err != nil {
			return fmt.Errorf("解析CODE字段失败: %v", err)
		}

		switch codeMsg.Code {
		case CodeVerifyIP:
			var msg VerifyIPMsg
			if err := json.Unmarshal(data, &msg); err != nil {
				return fmt.Errorf("解析VerifyIPMsg失败: %v", err)
			}
			log.Infof("[AUTH] 收到VerifyIPMsg: 设备ID=%s, 设备名称=%s, 认证端口=%d, 会话端口=%d", msg.DeviceID, msg.DeviceName, msg.AuthPort, msg.SessionPort)
			// 更新认证连接信息
			c.authConn.AuthPort = msg.AuthPort
			c.authConn.SessionPort = msg.SessionPort
			c.authConn.OnlineState = OnlineYes // 标记为在线

		case CodeVerifyDevID:
			var msg VerifyDeviceIDMsg
			if err := json.Unmarshal(data, &msg); err != nil {
				return fmt.Errorf("解析VerifyDeviceIDMsg失败: %v", err)
			}
			log.Infof("[AUTH] 收到VerifyDeviceIDMsg: 设备ID=%s", msg.DeviceID)
		}
	}

	return nil
}

// GetAuthConn 返回认证连接信息
// 返回：
//   - 认证连接信息（AuthConn实例）
func (c *AuthClient) GetAuthConn() *AuthConn {
	return c.authConn
}

// PerformHiChainAuth 执行完整的HiChain认证流程（与真实鸿蒙设备兼容）
// 返回：
//   - 协商完成的会话密钥
//   - 错误信息（认证失败时）
//
// 说明：此方法实现了完整的HiChain协议，包括：
// 1. AUTH_START - 发起认证请求
// 2. AUTH_CHALLENGE - 接收挑战
// 3. AUTH_RESPONSE - 发送响应
// 4. AUTH_CONFIRM - 接收确认
// 5. 密钥派生和保存
//
// 注意：本方法会启动一个goroutine接收AUTH_SDK消息，该goroutine在认证完成后会自动停止
func (c *AuthClient) PerformHiChainAuth() ([]byte, error) {
	log.Info("[AUTH_CLIENT] 开始HiChain认证流程")

	// 创建客户端认证接口
	sessionID := uint32(c.seqNum) // 使用序列号作为会话ID
	clientInterface := NewClientAuthInterface(c, c.conn, sessionID)

	// 启动HiChain认证
	if err := clientInterface.StartAuth(); err != nil {
		return nil, fmt.Errorf("启动HiChain认证失败: %w", err)
	}

	// 创建停止信号通道
	stopChan := make(chan struct{})

	// 循环接收并处理AUTH_SDK模块的消息
	go func() {
		defer func() {
			log.Info("[AUTH_CLIENT] AUTH_SDK消息接收goroutine已停止")
		}()

		for {
			select {
			case <-stopChan:
				// 收到停止信号，退出goroutine
				return
			default:
				// 继续接收消息
			}

			// 接收数据
			buf := make([]byte, DefaultBufSize)
			c.conn.SetReadDeadline(time.Now().Add(30 * time.Second)) // 30秒超时
			n, err := c.conn.Read(buf)
			if err != nil {
				// 检查是否是停止信号导致的错误
				select {
				case <-stopChan:
					return
				default:
				}

				log.Errorf("[AUTH_CLIENT] 接收数据失败: %v", err)
				clientInterface.authComplete <- fmt.Errorf("接收数据失败: %w", err)
				return
			}

			// 验证数据包长度
			if n < PacketHeadSize {
				log.Errorf("[AUTH_CLIENT] 数据包过短: %d字节", n)
				continue
			}

			// 解析数据包头部
			pkt, err := ParsePacketHead(buf, 0)
			if err != nil {
				log.Errorf("[AUTH_CLIENT] 解析数据包失败: %v", err)
				continue
			}

			// 只处理AUTH_SDK模块的消息
			if pkt.Module < ModuleHiChain || pkt.Module > ModuleAuthSDK {
				log.Debugf("[AUTH_CLIENT] 收到非AUTH_SDK消息，模块=%d（忽略）", pkt.Module)
				continue
			}

			// 验证数据包完整性
			if n < PacketHeadSize+pkt.DataLen {
				log.Errorf("[AUTH_CLIENT] 数据包不完整")
				continue
			}

			// 提取数据部分
			data := buf[PacketHeadSize : PacketHeadSize+pkt.DataLen]
			log.Infof("[AUTH_CLIENT] 收到AUTH_SDK消息: 模块=%d, 长度=%d", pkt.Module, pkt.DataLen)

			// 交由HiChain处理
			if err := clientInterface.ProcessReceivedData(data); err != nil {
				log.Errorf("[AUTH_CLIENT] HiChain处理失败: %v", err)
				clientInterface.authComplete <- fmt.Errorf("HiChain处理失败: %w", err)
				return
			}
		}
	}()

	// 等待认证完成
	err := clientInterface.WaitForCompletion()

	// 停止接收goroutine
	close(stopChan)

	// 短暂延迟，确保goroutine退出
	time.Sleep(100 * time.Millisecond)

	if err != nil {
		return nil, fmt.Errorf("HiChain认证失败: %w", err)
	}

	// 获取会话密钥
	sessionKey := clientInterface.GetSessionKey()
	if sessionKey == nil || len(sessionKey) == 0 {
		return nil, fmt.Errorf("未获取到会话密钥")
	}

	log.Infof("[AUTH_CLIENT] HiChain认证成功，密钥长度=%d字节", len(sessionKey))
	return sessionKey, nil
}
