package authentication

import (
	"fmt"
	"sync"
	"time"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// AuthConnection 认证连接（对应C的连接管理）
type AuthConnection struct {
	ConnId     uint64        // 连接ID（64位：高32位=connType，低32位=fd）
	Fd         int           // 文件描述符
	ConnInfo   *AuthConnInfo // 连接信息
	IsServer   bool          // 是否服务端连接
	CreateTime time.Time     // 创建时间
}

// ConnectRequest 连接请求
type ConnectRequest struct {
	RequestId uint32        // 请求ID
	ConnInfo  *AuthConnInfo // 连接信息
	StartTime time.Time     // 请求开始时间
}

// AuthConnectionManager Auth连接管理器
type AuthConnectionManager struct {
	listener    *AuthConnListener              // 上层监听器
	connections map[uint64]*AuthConnection     // connId -> connection
	fdToConnId  map[int]uint64                 // fd -> connId（快速查找）
	requests    map[uint32]*ConnectRequest     // requestId -> request
	mu          sync.RWMutex                   // 读写锁
}

// 全局连接管理器
var (
	g_authConnManager   *AuthConnectionManager
	g_authConnManagerMu sync.Mutex
)

// AuthConnInit 初始化认证连接管理器
// 对应C的AuthConnInit
//
// 参数:
//   - listener: 上层监听器
//
// 返回:
//   - error: 错误信息
func AuthConnInit(listener *AuthConnListener) error {
	if listener == nil {
		return fmt.Errorf("listener cannot be nil")
	}

	g_authConnManagerMu.Lock()
	defer g_authConnManagerMu.Unlock()

	if g_authConnManager != nil {
		log.Warnf("[AUTH_CONN] AuthConnectionManager already initialized, reinitializing...")
	}

	g_authConnManager = &AuthConnectionManager{
		listener:    listener,
		connections: make(map[uint64]*AuthConnection),
		fdToConnId:  make(map[int]uint64),
		requests:    make(map[uint32]*ConnectRequest),
	}

	// 注册Socket层回调
	socketCallback := &SocketCallback{
		OnConnected: func(module ListenerModule, fd int, isClient bool) {
			onAuthSocketConnected(fd, isClient)
		},
		OnDisconnected: func(fd int) {
			onAuthSocketDisconnected(fd)
		},
		OnDataReceived: func(module ListenerModule, fd int, head *AuthDataHead, data []byte) {
			onAuthSocketDataReceived(fd, head, data)
		},
	}

	err := SetSocketCallback(socketCallback)
	if err != nil {
		g_authConnManager = nil
		return fmt.Errorf("failed to set socket callback: %w", err)
	}

	log.Infof("[AUTH_CONN] Auth connection manager initialized")
	return nil
}

// AuthConnDeinit 反初始化认证连接管理器
// 对应C的AuthConnDeinit
func AuthConnDeinit() {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	if manager == nil {
		g_authConnManagerMu.Unlock()
		return
	}

	// 收集所有需要断开的连接（避免在持有锁时调用DisconnectAuthDevice导致死锁）
	manager.mu.Lock()
	connIds := make([]uint64, 0, len(manager.connections))
	for connId := range manager.connections {
		connIds = append(connIds, connId)
	}
	manager.mu.Unlock()

	// 设置为nil（后续DisconnectAuthDevice调用会发现manager已销毁）
	g_authConnManager = nil
	g_authConnManagerMu.Unlock()

	// 断开所有连接（在释放锁后进行）
	for _, connId := range connIds {
		manager.mu.Lock()
		conn, exists := manager.connections[connId]
		if !exists {
			manager.mu.Unlock()
			continue
		}
		fd := conn.Fd
		delete(manager.connections, connId)
		delete(manager.fdToConnId, fd)
		manager.mu.Unlock()

		log.Infof("[AUTH_CONN] Disconnecting device: connId=%d, fd=%d", connId, fd)
		SocketDisconnectDevice(Auth, fd)
	}

	log.Infof("[AUTH_CONN] Auth connection manager deinitialized")
}

// ConnectAuthDevice 连接认证设备
// 对应C的ConnectAuthDevice
//
// 参数:
//   - requestId: 请求ID
//   - connInfo: 连接信息
//
// 返回:
//   - error: 错误信息
func ConnectAuthDevice(requestId uint32, connInfo *AuthConnInfo) error {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		return fmt.Errorf("auth connection manager not initialized")
	}

	if connInfo == nil {
		return fmt.Errorf("connInfo cannot be nil")
	}

	// 保存请求信息
	manager.mu.Lock()
	manager.requests[requestId] = &ConnectRequest{
		RequestId: requestId,
		ConnInfo:  connInfo,
		StartTime: time.Now(),
	}
	manager.mu.Unlock()

	log.Infof("[AUTH_CONN] Connecting to device: requestId=%d, ip=%s, port=%d",
		requestId, connInfo.Ip, connInfo.Port)

	// 调用底层连接
	fd, err := SocketConnectDevice(connInfo.Ip, connInfo.Port)
	if err != nil {
		// 移除请求
		manager.mu.Lock()
		delete(manager.requests, requestId)
		manager.mu.Unlock()

		// 通知连接失败
		if manager.listener != nil && manager.listener.OnConnectResult != nil {
			manager.listener.OnConnectResult(requestId, 0, -1, connInfo)
		}

		return fmt.Errorf("failed to connect device: %w", err)
	}

	log.Debugf("[AUTH_CONN] Socket connected: requestId=%d, fd=%d", requestId, fd)
	return nil
}

// DisconnectAuthDevice 断开认证设备连接
// 对应C的DisconnectAuthDevice
//
// 参数:
//   - connId: 连接ID
func DisconnectAuthDevice(connId uint64) {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		log.Warnf("[AUTH_CONN] Cannot disconnect: manager not initialized")
		return
	}

	manager.mu.Lock()
	conn, exists := manager.connections[connId]
	if !exists {
		manager.mu.Unlock()
		log.Warnf("[AUTH_CONN] Connection not found: connId=%d", connId)
		return
	}

	fd := conn.Fd
	delete(manager.connections, connId)
	delete(manager.fdToConnId, fd)
	manager.mu.Unlock()

	log.Infof("[AUTH_CONN] Disconnecting device: connId=%d, fd=%d", connId, fd)

	// 调用底层断开
	SocketDisconnectDevice(Auth, fd)
}

// PostAuthData 发送认证数据
// 对应C的PostAuthData
//
// 参数:
//   - connId: 连接ID
//   - head: 数据头
//   - data: 数据内容
//
// 返回:
//   - error: 错误信息
func PostAuthData(connId uint64, head *AuthDataHead, data []byte) error {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		return fmt.Errorf("auth connection manager not initialized")
	}

	manager.mu.RLock()
	conn, exists := manager.connections[connId]
	manager.mu.RUnlock()

	if !exists {
		return fmt.Errorf("connection not found: connId=%d", connId)
	}

	log.Debugf("[AUTH_CONN] Posting auth data: connId=%d, fd=%d, module=%d, seq=%d, len=%d",
		connId, conn.Fd, head.Module, head.Seq, head.Len)

	// 调用底层发送
	return SocketPostBytes(conn.Fd, head, data)
}

// GetConnInfo 获取连接信息
// 对应C的GetConnInfoByConnectionId
//
// 参数:
//   - connId: 连接ID
//
// 返回:
//   - *AuthConnInfo: 连接信息
//   - error: 错误信息
func GetConnInfo(connId uint64) (*AuthConnInfo, error) {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		return nil, fmt.Errorf("auth connection manager not initialized")
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	conn, exists := manager.connections[connId]
	if !exists {
		return nil, fmt.Errorf("connection not found: connId=%d", connId)
	}

	return conn.ConnInfo, nil
}

// ============================================================================
// 内部回调处理函数
// ============================================================================

// onAuthSocketConnected Socket连接成功回调
func onAuthSocketConnected(fd int, isClient bool) {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		log.Warnf("[AUTH_CONN] Manager not initialized in onAuthSocketConnected")
		return
	}

	log.Debugf("[AUTH_CONN] Socket connected: fd=%d, isClient=%v", fd, isClient)

	// 客户端连接：需要匹配请求
	if isClient {
		manager.mu.Lock()
		defer manager.mu.Unlock()

		// 查找对应的请求（通过时间顺序，找最早的未处理请求）
		var matchedRequest *ConnectRequest
		var matchedRequestId uint32
		for requestId, req := range manager.requests {
			if matchedRequest == nil || req.StartTime.Before(matchedRequest.StartTime) {
				matchedRequest = req
				matchedRequestId = requestId
			}
		}

		if matchedRequest == nil {
			log.Warnf("[AUTH_CONN] No pending request found for fd=%d", fd)
			return
		}

		// 生成ConnId
		connId := GenConnId(matchedRequest.ConnInfo.Type, int32(fd))

		// 创建连接记录
		conn := &AuthConnection{
			ConnId:     connId,
			Fd:         fd,
			ConnInfo:   matchedRequest.ConnInfo,
			IsServer:   false,
			CreateTime: time.Now(),
		}

		manager.connections[connId] = conn
		manager.fdToConnId[fd] = connId

		// 移除请求
		delete(manager.requests, matchedRequestId)

		log.Infof("[AUTH_CONN] Client connection established: connId=%d, fd=%d, requestId=%d",
			connId, fd, matchedRequestId)

		// 通知上层连接成功
		if manager.listener != nil && manager.listener.OnConnectResult != nil {
			manager.listener.OnConnectResult(matchedRequestId, connId, 0, matchedRequest.ConnInfo)
		}

	} else {
		// 服务端连接：从底层获取连接信息
		connInfoRaw, _, err := SocketGetConnInfo(fd)
		if err != nil {
			log.Errorf("[AUTH_CONN] Failed to get conn info for server fd=%d: %v", fd, err)
			return
		}

		manager.mu.Lock()
		defer manager.mu.Unlock()

		// 生成ConnId（服务端默认使用WiFi类型）
		connId := GenConnId(AuthLinkTypeWifi, int32(fd))

		// 创建连接记录
		conn := &AuthConnection{
			ConnId:   connId,
			Fd:       fd,
			ConnInfo: connInfoRaw,
			IsServer: true,
			CreateTime: time.Now(),
		}

		manager.connections[connId] = conn
		manager.fdToConnId[fd] = connId

		log.Infof("[AUTH_CONN] Server connection established: connId=%d, fd=%d", connId, fd)

		// 服务端连接不需要通知OnConnectResult（没有requestId）
	}
}

// onAuthSocketDisconnected Socket断开连接回调
func onAuthSocketDisconnected(fd int) {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		log.Warnf("[AUTH_CONN] Manager not initialized in onAuthSocketDisconnected")
		return
	}

	manager.mu.Lock()
	connId, exists := manager.fdToConnId[fd]
	if !exists {
		manager.mu.Unlock()
		log.Warnf("[AUTH_CONN] Connection not found for fd=%d", fd)
		return
	}

	conn := manager.connections[connId]
	delete(manager.connections, connId)
	delete(manager.fdToConnId, fd)
	manager.mu.Unlock()

	log.Infof("[AUTH_CONN] Connection disconnected: connId=%d, fd=%d", connId, fd)

	// 通知上层断开连接
	if manager.listener != nil && manager.listener.OnDisconnected != nil {
		manager.listener.OnDisconnected(connId, conn.ConnInfo)
	}
}

// onAuthSocketDataReceived Socket接收数据回调
func onAuthSocketDataReceived(fd int, head *AuthDataHead, data []byte) {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		log.Warnf("[AUTH_CONN] Manager not initialized in onAuthSocketDataReceived")
		return
	}

	manager.mu.RLock()
	connId, exists := manager.fdToConnId[fd]
	if !exists {
		manager.mu.RUnlock()
		log.Warnf("[AUTH_CONN] Connection not found for fd=%d", fd)
		return
	}

	conn := manager.connections[connId]
	manager.mu.RUnlock()

	log.Debugf("[AUTH_CONN] Data received: connId=%d, fd=%d, module=%d, seq=%d, len=%d",
		connId, fd, head.Module, head.Seq, len(data))

	// 通知上层接收数据
	if manager.listener != nil && manager.listener.OnDataReceived != nil {
		manager.listener.OnDataReceived(connId, conn.ConnInfo, conn.IsServer, head, data)
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// GetAuthConnectionByConnId 根据ConnId获取连接（内部使用）
func GetAuthConnectionByConnId(connId uint64) (*AuthConnection, error) {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		return nil, fmt.Errorf("auth connection manager not initialized")
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	conn, exists := manager.connections[connId]
	if !exists {
		return nil, fmt.Errorf("connection not found: connId=%d", connId)
	}

	return conn, nil
}

// GetAuthConnectionByFd 根据Fd获取连接（内部使用）
func GetAuthConnectionByFd(fd int) (*AuthConnection, error) {
	g_authConnManagerMu.Lock()
	manager := g_authConnManager
	g_authConnManagerMu.Unlock()

	if manager == nil {
		return nil, fmt.Errorf("auth connection manager not initialized")
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	connId, exists := manager.fdToConnId[fd]
	if !exists {
		return nil, fmt.Errorf("connection not found for fd=%d", fd)
	}

	conn := manager.connections[connId]
	return conn, nil
}
