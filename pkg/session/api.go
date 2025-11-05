package session

import "fmt"

// 全局会话管理器实例
var globalManager *TcpSessionManager

// InitSessionManager 初始化全局会话管理器
// 参数：
//   - asServer: 是否作为服务器模式
//   - localIP: 本地IP地址
// 返回：
//   - port: 监听端口（如果是服务器模式）
//   - error: 错误信息
func InitSessionManager(asServer bool, localIP string, authMgr AuthManagerInterface) (int, error) {
	if globalManager != nil {
		return 0, ErrManagerAlreadyStarted
	}

	globalManager = NewTcpSessionManager(asServer, localIP, authMgr)
	port, err := globalManager.Start()
	if err != nil {
		globalManager = nil
		return 0, err
	}

	return port, nil
}

// DestroySessionManager 销毁全局会话管理器
func DestroySessionManager() error {
	if globalManager == nil {
		return ErrManagerNotStarted
	}

	err := globalManager.Stop()
	globalManager = nil
	return err
}

// GetSessionManager 获取全局会话管理器
func GetSessionManager() *TcpSessionManager {
	return globalManager
}

// CreateSessionServer 创建会话服务器
// 参数：
//   - moduleName: 模块名称
//   - sessionName: 会话名称
//   - listener: 会话监听器
// 返回：错误信息
//
// 示例：
//   listener := &MySessionListener{}
//   err := session.CreateSessionServer("myModule", "mySession", listener)
func CreateSessionServer(moduleName, sessionName string, listener ISessionListener) error {
	if globalManager == nil {
		return ErrManagerNotStarted
	}

	return globalManager.CreateSessionServer(moduleName, sessionName, listener)
}

// RemoveSessionServer 移除会话服务器
// 参数：
//   - moduleName: 模块名称
//   - sessionName: 会话名称
// 返回：错误信息
func RemoveSessionServer(moduleName, sessionName string) error {
	if globalManager == nil {
		return ErrManagerNotStarted
	}

	return globalManager.RemoveSessionServer(moduleName, sessionName)
}

// SendBytes 发送数据到会话
// 参数：
//   - sessionID: 会话ID
//   - data: 要发送的数据
// 返回：错误信息
//
// 示例：
//   err := session.SendBytes(sessionID, []byte("Hello, World!"))
func SendBytes(sessionID int, data []byte) error {
	if globalManager == nil {
		return ErrManagerNotStarted
	}

	return globalManager.SendBytes(sessionID, data)
}

// GetMySessionName 获取本地会话名称
// 参数：
//   - sessionID: 会话ID
// 返回：
//   - sessionName: 会话名称
//   - error: 错误信息
func GetMySessionName(sessionID int) (string, error) {
	if globalManager == nil {
		return "", ErrManagerNotStarted
	}

	session, err := globalManager.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	return session.GetSessionName(), nil
}

// GetPeerSessionName 获取对端会话名称
// 参数：
//   - sessionID: 会话ID
// 返回：
//   - sessionName: 对端会话名称
//   - error: 错误信息
func GetPeerSessionName(sessionID int) (string, error) {
	// 对于当前实现，本地和对端会话名称相同
	return GetMySessionName(sessionID)
}

// GetPeerDeviceID 获取对端设备ID
// 参数：
//   - sessionID: 会话ID
// 返回：
//   - deviceID: 对端设备ID
//   - error: 错误信息
func GetPeerDeviceID(sessionID int) (string, error) {
	if globalManager == nil {
		return "", ErrManagerNotStarted
	}

	session, err := globalManager.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	return session.GetDeviceID(), nil
}

// CloseSession 关闭会话
// 参数：
//   - sessionID: 会话ID
//
// 注意：关闭会话会触发OnSessionClosed回调
func CloseSession(sessionID int) error {
	if globalManager == nil {
		return ErrManagerNotStarted
	}

	return globalManager.CloseSession(sessionID)
}

// GetSessionPort 获取会话服务端口
// 返回：监听端口号
func GetSessionPort() (int, error) {
	if globalManager == nil {
		return 0, ErrManagerNotStarted
	}

	return globalManager.GetPort(), nil
}

// IsSessionManagerRunning 检查会话管理器是否运行
// 返回：true表示运行中
func IsSessionManagerRunning() bool {
	if globalManager == nil {
		return false
	}

	return globalManager.IsRunning()
}

// GetSessionInfo 获取会话详细信息
// 参数：
//   - sessionID: 会话ID
// 返回：会话信息的格式化字符串
func GetSessionInfo(sessionID int) (string, error) {
	if globalManager == nil {
		return "", ErrManagerNotStarted
	}

	session, err := globalManager.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"Session[ID=%d, Name=%s, DeviceID=%s, State=%d, RemoteAddr=%s]",
		session.GetSessionID(),
		session.GetSessionName(),
		session.GetDeviceID(),
		session.GetState(),
		session.GetRemoteAddr(),
	), nil
}

// OpenSession 作为客户端主动打开到远程设备的会话
// 参数：
//   - peerIP: 对端IP地址
//   - peerPort: 对端会话端口
//   - sessionName: 会话名称
//   - myDeviceId: 本地设备ID
// 返回：sessionID和错误信息
//
// 示例：
//   sessionID, err := session.OpenSession("192.168.1.100", 12345, "mySession", "device123")
func OpenSession(peerIP string, peerPort int, sessionName, myDeviceId string) (int, error) {
	if globalManager == nil {
		return -1, ErrManagerNotStarted
	}

	return globalManager.OpenSession(peerIP, peerPort, sessionName, myDeviceId)
}
