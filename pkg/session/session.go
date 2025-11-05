package session

import (
	"net"
	"sync"
	"time"
)

// TcpSession 表示一个TCP会话
type TcpSession struct {
	// 会话标识
	SessionName string // 会话名称
	DeviceID    string // 对端设备ID
	GroupID     string // 组ID
	SessionID   int    // 会话ID（文件描述符）

	// 会话密钥
	SessionKey [SessionKeyLength]byte // 会话密钥（256位）

	// 序列号管理（与C版本兼容，使用int32）
	SendSeqNum int32   // 发送序列号（原子递增）
	RecvSeqNums []int32 // 接收序列号列表（防重放攻击）

	// 网络连接
	Conn net.Conn // 网络连接

	// 会话状态
	State      int  // 会话状态
	IsAccepted bool // 是否为被动接受的连接
	BusVersion int  // 总线版本

	// 时间戳
	CreatedAt time.Time // 创建时间
	LastActive time.Time // 最后活跃时间

	// 互斥锁
	mu sync.RWMutex
}

// NewTcpSession 创建新的TCP会话
func NewTcpSession(conn net.Conn, isAccepted bool) *TcpSession {
	now := time.Now()
	session := &TcpSession{
		SessionName: DefaultUnknownSessionName,
		SessionID:   -1, // 将由管理器分配
		Conn:        conn,
		State:       SessionStateConnected,
		IsAccepted:  isAccepted,
		BusVersion:  DefaultBusVersion,
		RecvSeqNums: make([]int32, 0, 100),
		CreatedAt:   now,
		LastActive:  now,
	}
	return session
}

// GetSessionID 获取会话ID
func (s *TcpSession) GetSessionID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionID
}

// SetSessionID 设置会话ID
func (s *TcpSession) SetSessionID(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionID = id
}

// GetSessionName 获取会话名称
func (s *TcpSession) GetSessionName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionName
}

// SetSessionName 设置会话名称
func (s *TcpSession) SetSessionName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionName = name
}

// GetDeviceID 获取设备ID
func (s *TcpSession) GetDeviceID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DeviceID
}

// SetDeviceID 设置设备ID
func (s *TcpSession) SetDeviceID(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DeviceID = deviceID
}

// GetSessionKey 获取会话密钥副本
func (s *TcpSession) GetSessionKey() [SessionKeyLength]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SessionKey
}

// SetSessionKey 设置会话密钥
func (s *TcpSession) SetSessionKey(key [SessionKeyLength]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SessionKey = key
}

// GetState 获取会话状态
func (s *TcpSession) GetState() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// SetState 设置会话状态
func (s *TcpSession) SetState(state int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}

// NextSendSeqNum 获取下一个发送序列号（原子递增）
func (s *TcpSession) NextSendSeqNum() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SendSeqNum++
	return s.SendSeqNum
}

// CheckAndAddRecvSeqNum 检查并添加接收序列号（防重放攻击）
func (s *TcpSession) CheckAndAddRecvSeqNum(seqNum int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已存在（重放攻击）
	for _, existing := range s.RecvSeqNums {
		if existing == seqNum {
			return false // 重放攻击
		}
	}

	// 添加到列表
	s.RecvSeqNums = append(s.RecvSeqNums, seqNum)

	// 限制列表长度（保留最近1000个）
	if len(s.RecvSeqNums) > 1000 {
		s.RecvSeqNums = s.RecvSeqNums[len(s.RecvSeqNums)-1000:]
	}

	return true
}

// UpdateLastActive 更新最后活跃时间
func (s *TcpSession) UpdateLastActive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastActive = time.Now()
}

// GetRemoteAddr 获取远程地址
func (s *TcpSession) GetRemoteAddr() string {
	if s.Conn != nil {
		return s.Conn.RemoteAddr().String()
	}
	return ""
}

// Close 关闭会话
func (s *TcpSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == SessionStateClosed {
		return nil // 已经关闭
	}

	s.State = SessionStateClosed

	if s.Conn != nil {
		err := s.Conn.Close()
		s.Conn = nil
		return err
	}

	return nil
}

// IsClosed 检查会话是否已关闭
func (s *TcpSession) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == SessionStateClosed
}

// IsOpened 检查会话是否已打开
func (s *TcpSession) IsOpened() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State == SessionStateOpened
}
