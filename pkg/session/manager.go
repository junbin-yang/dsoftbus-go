package session

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// TcpSessionManager TCP会话管理器
type TcpSessionManager struct {
	// 服务器模式标志
	asServer bool

	// 本地监听地址
	localIP string
	port    int

	// TCP监听器
	listener net.Listener

	// 会话映射（sessionID -> TcpSession）
	sessionMap map[int]*TcpSession

	// 会话服务器映射（serverKey -> SessionServer）
	serverMap map[string]*SessionServer

	// 会话ID计数器（自动递增）
	nextSessionID int32

	// 认证管理器引用（用于获取会话密钥和设备信息）
	authMgr AuthManagerInterface

	// 运行状态
	running bool
	stopCh  chan struct{}

	// 互斥锁
	mu sync.RWMutex

	// 等待组（用于优雅关闭）
	wg sync.WaitGroup
}

// AuthManagerInterface 认证管理器接口（避免循环依赖）
type AuthManagerInterface interface {
	GetSessionKeyByIndex(index int) ([]byte, error)
	GetLocalDeviceID() string
}

// NewTcpSessionManager 创建新的TCP会话管理器
func NewTcpSessionManager(asServer bool, localIP string, authMgr AuthManagerInterface) *TcpSessionManager {
	return &TcpSessionManager{
		asServer:      asServer,
		localIP:       localIP,
		authMgr:       authMgr,
		sessionMap:    make(map[int]*TcpSession),
		serverMap:     make(map[string]*SessionServer),
		nextSessionID: 1000, // 从1000开始分配会话ID
		stopCh:        make(chan struct{}),
	}
}

// Start 启动会话管理器
func (m *TcpSessionManager) Start() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return 0, ErrManagerAlreadyStarted
	}

	// 创建TCP监听器（端口自动分配）
	addr := fmt.Sprintf("%s:0", m.localIP)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("failed to start TCP listener: %w", err)
	}

	m.listener = listener
	m.running = true

	// 获取实际监听端口
	tcpAddr := listener.Addr().(*net.TCPAddr)
	m.port = tcpAddr.Port

	logger.Infof("Session manager started on %s:%d", m.localIP, m.port)

	// 启动接受连接的goroutine
	m.wg.Add(1)
	go m.acceptLoop()

	return m.port, nil
}

// Stop 停止会话管理器
func (m *TcpSessionManager) Stop() error {
	m.mu.Lock()

	if !m.running {
		m.mu.Unlock()
		return ErrManagerNotStarted
	}

	m.running = false
	close(m.stopCh)

	// 关闭监听器
	if m.listener != nil {
		m.listener.Close()
	}

	// 关闭所有会话
	for _, session := range m.sessionMap {
		session.Close()
	}

	m.mu.Unlock()

	// 等待所有goroutine结束
	m.wg.Wait()

	logger.Info("Session manager stopped")
	return nil
}

// acceptLoop 接受连接循环
func (m *TcpSessionManager) acceptLoop() {
	defer m.wg.Done()

	for {
		select {
		case <-m.stopCh:
			return
		default:
		}

		conn, err := m.listener.Accept()
		if err != nil {
			// 检查是否是因为停止导致的错误
			select {
			case <-m.stopCh:
				return
			default:
				logger.Errorf("Failed to accept connection: %v", err)
				continue
			}
		}

		logger.Infof("New connection from %s", conn.RemoteAddr())

		// 为每个连接启动处理goroutine
		m.wg.Add(1)
		go m.handleConnection(conn)
	}
}

// handleConnection 处理单个连接
func (m *TcpSessionManager) handleConnection(conn net.Conn) {
	defer m.wg.Done()

	// 创建会话
	session := NewTcpSession(conn, true)
	sessionID := m.allocateSessionID()
	session.SetSessionID(sessionID)

	// 添加到会话映射
	if err := m.addSession(session); err != nil {
		logger.Errorf("Failed to add session %d: %v", sessionID, err)
		conn.Close()
		return
	}

	logger.Infof("Session %d created for %s", sessionID, conn.RemoteAddr())

	// 处理会话（握手和数据传输）
	if err := m.processSession(session); err != nil {
		logger.Errorf("Session %d error: %v", sessionID, err)
	}

	// 清理会话
	m.removeSession(sessionID)
	logger.Infof("Session %d closed", sessionID)
}

// allocateSessionID 分配新的会话ID
func (m *TcpSessionManager) allocateSessionID() int {
	return int(atomic.AddInt32(&m.nextSessionID, 1))
}

// addSession 添加会话到映射
func (m *TcpSessionManager) addSession(session *TcpSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionID := session.GetSessionID()

	// 检查是否超过最大会话数
	if len(m.sessionMap) >= MaxSessionSumNum {
		return ErrMaxSessionsReached
	}

	// 检查会话ID是否已存在
	if _, exists := m.sessionMap[sessionID]; exists {
		return ErrSessionExists
	}

	m.sessionMap[sessionID] = session
	return nil
}

// removeSession 从映射中移除会话
func (m *TcpSessionManager) removeSession(sessionID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessionMap[sessionID]
	if !exists {
		return
	}

	// 从对应的SessionServer中移除
	sessionName := session.GetSessionName()
	if sessionName != DefaultUnknownSessionName {
		for _, server := range m.serverMap {
			if server.SessionName == sessionName {
				server.RemoveSession(sessionID)
				// 调用OnSessionClosed回调
				if server.Listener != nil {
					server.Listener.OnSessionClosed(sessionID)
				}
				break
			}
		}
	}

	// 关闭会话
	session.Close()

	// 从映射中删除
	delete(m.sessionMap, sessionID)
}

// GetSession 获取会话
func (m *TcpSessionManager) GetSession(sessionID int) (*TcpSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessionMap[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	return session, nil
}

// CreateSessionServer 创建会话服务器
func (m *TcpSessionManager) CreateSessionServer(moduleName, sessionName string, listener ISessionListener) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return ErrManagerNotStarted
	}

	// 检查参数
	if moduleName == "" || sessionName == "" {
		return ErrInvalidParameter
	}
	if listener == nil {
		return ErrNilListener
	}

	// 检查是否超过最大服务器数
	if len(m.serverMap) >= MaxSessionServerNum {
		return ErrMaxServersReached
	}

	serverKey := makeServerKey(moduleName, sessionName)

	// 检查服务器是否已存在
	if _, exists := m.serverMap[serverKey]; exists {
		return ErrServerExists
	}

	// 创建会话服务器
	server := NewSessionServer(moduleName, sessionName, listener)
	m.serverMap[serverKey] = server

	logger.Infof("Session server created: %s/%s", moduleName, sessionName)
	return nil
}

// RemoveSessionServer 移除会话服务器
func (m *TcpSessionManager) RemoveSessionServer(moduleName, sessionName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	serverKey := makeServerKey(moduleName, sessionName)

	server, exists := m.serverMap[serverKey]
	if !exists {
		return ErrServerNotFound
	}

	// 关闭该服务器的所有会话
	for sessionID := range server.Sessions {
		if session, ok := m.sessionMap[sessionID]; ok {
			session.Close()
			delete(m.sessionMap, sessionID)
		}
	}

	// 删除服务器
	delete(m.serverMap, serverKey)

	logger.Infof("Session server removed: %s/%s", moduleName, sessionName)
	return nil
}

// GetSessionServer 获取会话服务器
func (m *TcpSessionManager) GetSessionServer(sessionName string) *SessionServer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 遍历查找匹配的服务器
	for _, server := range m.serverMap {
		if server.SessionName == sessionName {
			return server
		}
	}

	return nil
}

// GetPort 获取监听端口
func (m *TcpSessionManager) GetPort() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.port
}

// IsRunning 检查管理器是否运行中
func (m *TcpSessionManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// OpenSession 作为客户端主动打开到远程设备的会话
// 参数：
//   - peerIP: 对端IP地址
//   - peerPort: 对端会话端口
//   - sessionName: 会话名称
//   - myDeviceId: 本地设备ID
// 返回：sessionID和错误信息
func (m *TcpSessionManager) OpenSession(peerIP string, peerPort int, sessionName, myDeviceId string) (int, error) {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return -1, ErrManagerNotStarted
	}

	// 检查是否超过最大会话数
	if len(m.sessionMap) >= MaxSessionSumNum {
		m.mu.Unlock()
		return -1, ErrMaxSessionsReached
	}
	m.mu.Unlock()

	// 连接到远程设备
	addr := fmt.Sprintf("%s:%d", peerIP, peerPort)
	logger.Infof("Connecting to remote device at %s for session '%s'", addr, sessionName)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return -1, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	// 创建会话（客户端模式）
	session := NewTcpSession(conn, false)
	sessionID := m.allocateSessionID()
	session.SetSessionID(sessionID)
	session.SetSessionName(sessionName)
	session.SetDeviceID(myDeviceId)

	// 添加到会话映射
	if err := m.addSession(session); err != nil {
		logger.Errorf("Failed to add session %d: %v", sessionID, err)
		conn.Close()
		return -1, err
	}

	logger.Infof("Client session %d created for %s", sessionID, conn.RemoteAddr())

	// 启动客户端会话处理
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer m.removeSession(sessionID)

		// 发送会话请求
		if err := m.sendRequestMessage(session); err != nil {
			logger.Errorf("Client session %d handshake failed: %v", sessionID, err)
			return
		}

		logger.Infof("Client session %d handshake completed", sessionID)

		// 启动接收循环
		if err := m.receiveLoop(session); err != nil {
			logger.Errorf("Client session %d receive loop error: %v", sessionID, err)
		}
	}()

	return sessionID, nil
}
