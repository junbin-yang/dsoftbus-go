package transmission

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/junbin-yang/dsoftbus-go/pkg/authentication"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// Session 会话
type Session struct {
	SessionID   int32  // 会话ID
	SessionName string // 会话名称
	PeerName    string // 对端会话名称
	AuthID      int64  // 认证ID
	ChannelID   int    // 通道ID
	IsServer    bool   // 是否服务端
}

// SessionServer 会话服务器
type SessionServer struct {
	SessionName string                 // 会话名称
	OnBind      func(sessionID int32)  // 绑定回调
	OnShutdown  func(sessionID int32)  // 关闭回调
	OnBytes     func(sessionID int32, data []byte) // 接收字节回调
	OnMessage   func(sessionID int32, data []byte) // 接收消息回调
}

// SessionManager 会话管理器
type SessionManager struct {
	sessions       map[int32]*Session       // sessionID -> Session
	servers        map[string]*SessionServer // sessionName -> SessionServer
	sessionCounter int32                     // 会话ID计数器
	mu             sync.RWMutex
}

var (
	globalSessionMgr     *SessionManager
	sessionMgrOnce       sync.Once
)

// getSessionManager 获取全局会话管理器
func getSessionManager() *SessionManager {
	sessionMgrOnce.Do(func() {
		globalSessionMgr = &SessionManager{
			sessions:       make(map[int32]*Session),
			servers:        make(map[string]*SessionServer),
			sessionCounter: 1000,
		}
	})
	return globalSessionMgr
}

// CreateSessionServer 创建会话服务器
func CreateSessionServer(pkgName string, sessionName string, listener *SessionServer) error {
	mgr := getSessionManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if _, exists := mgr.servers[sessionName]; exists {
		return fmt.Errorf("session server already exists: %s", sessionName)
	}

	listener.SessionName = sessionName
	mgr.servers[sessionName] = listener

	log.Infof("[SESSION] Created session server: %s", sessionName)
	return nil
}

// RemoveSessionServer 移除会话服务器
func RemoveSessionServer(pkgName string, sessionName string) error {
	mgr := getSessionManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	delete(mgr.servers, sessionName)
	log.Infof("[SESSION] Removed session server: %s", sessionName)
	return nil
}

// OpenSession 打开会话
func OpenSession(sessionName string, peerName string, peerNetworkID string, authID int64) (int32, error) {
	mgr := getSessionManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	// 生成会话ID
	sessionID := atomic.AddInt32(&mgr.sessionCounter, 1)

	// 创建会话
	session := &Session{
		SessionID:   sessionID,
		SessionName: sessionName,
		PeerName:    peerName,
		AuthID:      authID,
		ChannelID:   int(authID), // 使用authID作为channelID
		IsServer:    false,
	}

	mgr.sessions[sessionID] = session

	log.Infof("[SESSION] Opened session: sessionID=%d, name=%s, peer=%s, channelID=%d", sessionID, sessionName, peerName, session.ChannelID)
	return sessionID, nil
}

// CloseSession 关闭会话
func CloseSession(sessionID int32) error {
	mgr := getSessionManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	session, exists := mgr.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %d", sessionID)
	}

	delete(mgr.sessions, sessionID)
	log.Infof("[SESSION] Closed session: sessionID=%d", sessionID)

	// 触发关闭回调
	if server, ok := mgr.servers[session.SessionName]; ok && server.OnShutdown != nil {
		go server.OnShutdown(sessionID)
	}

	return nil
}

// SendBytes 发送字节数据
func SendBytes(sessionID int32, data []byte) error {
	mgr := getSessionManager()
	mgr.mu.RLock()
	session, exists := mgr.sessions[sessionID]
	mgr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %d", sessionID)
	}

	// 使用认证通道发送数据
	channelData := &authentication.AuthChannelData{
		Module: authentication.ModuleAuthMsg,
		Flag:   0,
		Seq:    int64(sessionID),
		Len:    uint32(len(data)),
		Data:   data,
	}

	if err := authentication.AuthPostChannelData(session.ChannelID, channelData); err != nil {
		return fmt.Errorf("failed to send bytes: %w", err)
	}

	log.Infof("[SESSION] Sent bytes: sessionID=%d, len=%d", sessionID, len(data))
	return nil
}

// SendMessage 发送消息数据
func SendMessage(sessionID int32, data []byte) error {
	// SendMessage 与 SendBytes 实现相同
	return SendBytes(sessionID, data)
}

// GetSession 获取会话
func GetSession(sessionID int32) (*Session, error) {
	mgr := getSessionManager()
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	session, exists := mgr.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %d", sessionID)
	}

	return session, nil
}

// GetSessionServer 获取会话服务器
func GetSessionServer(sessionName string) (*SessionServer, error) {
	mgr := getSessionManager()
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	server, exists := mgr.servers[sessionName]
	if !exists {
		return nil, fmt.Errorf("session server not found: %s", sessionName)
	}

	return server, nil
}
