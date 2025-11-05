package session

// ISessionListener 会话监听器接口
// 应用层需要实现此接口以接收会话事件
type ISessionListener interface {
	// OnSessionOpened 当会话打开时调用
	// 参数：sessionID - 会话ID
	// 返回：0表示成功，非0表示错误
	OnSessionOpened(sessionID int) int

	// OnSessionClosed 当会话关闭时调用
	// 参数：sessionID - 会话ID
	OnSessionClosed(sessionID int)

	// OnBytesReceived 当接收到数据时调用
	// 参数：
	//   sessionID - 会话ID
	//   data - 接收到的数据
	OnBytesReceived(sessionID int, data []byte)
}

// SessionServer 会话服务器
// 表示一个注册的会话服务，可以接收特定会话名称的连接
type SessionServer struct {
	// 模块名称
	ModuleName string

	// 会话名称
	SessionName string

	// 监听器
	Listener ISessionListener

	// 关联的会话列表（最多MaxSessionNum个）
	Sessions map[int]*TcpSession

	// 是否激活
	Active bool
}

// NewSessionServer 创建新的会话服务器
func NewSessionServer(moduleName, sessionName string, listener ISessionListener) *SessionServer {
	return &SessionServer{
		ModuleName:  moduleName,
		SessionName: sessionName,
		Listener:    listener,
		Sessions:    make(map[int]*TcpSession),
		Active:      true,
	}
}

// AddSession 添加会话到服务器
func (ss *SessionServer) AddSession(session *TcpSession) error {
	if len(ss.Sessions) >= MaxSessionNum {
		return ErrMaxSessionsReached
	}
	ss.Sessions[session.GetSessionID()] = session
	return nil
}

// RemoveSession 从服务器移除会话
func (ss *SessionServer) RemoveSession(sessionID int) {
	delete(ss.Sessions, sessionID)
}

// GetSession 获取会话
func (ss *SessionServer) GetSession(sessionID int) (*TcpSession, bool) {
	session, ok := ss.Sessions[sessionID]
	return session, ok
}

// SessionCount 获取当前会话数量
func (ss *SessionServer) SessionCount() int {
	return len(ss.Sessions)
}

// makeServerKey 生成服务器键
// 用于在管理器中唯一标识一个SessionServer
func makeServerKey(moduleName, sessionName string) string {
	return moduleName + ":" + sessionName
}
