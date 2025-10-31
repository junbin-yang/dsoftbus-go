package authmanager

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

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

	log.Infof("[AUTH] 收到响应: 模块=%d, 序列号=%d, 标志位=%d, 数据长度=%d\n", pkt.Module, pkt.Seq, pkt.Flags, pkt.DataLen)

	// 根据模块类型解析响应
	switch pkt.Module {
	case ModuleTrustEngine:
		var msg GetDeviceIDMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("解析GetDeviceIDMsg失败: %v", err)
		}
		log.Infof("[AUTH] 收到GetDeviceIDMsg: 命令=%s, 数据=%s, 设备ID=%s", msg.Cmd, msg.Data, msg.DeviceID)
		// 更新认证连接信息
		c.authConn.DeviceID = msg.Data
		c.authConn.AuthID = msg.DeviceID

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
