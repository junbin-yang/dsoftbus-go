package authentication

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/junbin-yang/dsoftbus-go/pkg/device_auth"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// AuthManager 相关常量
	AuthManagerInitialAuthId int64 = 1000 // 起始AuthId
	AuthManagerInitialSeq    int64 = 1000 // 起始序列号
)

const (
	// 认证结果常量
	AuthResultSuccess        int32 = 0  // 认证成功
	AuthResultFailed         int32 = -1 // 认证失败
	AuthResultTimeout        int32 = -2 // 认证超时
	AuthResultConnectionLost int32 = -3 // 连接断开
)

// ============================================================================
// 回调接口定义
// ============================================================================

// AuthConnCallback 认证连接回调（对应C的AuthConnCallback）
// 用于应用层接收认证事件通知
type AuthConnCallback struct {
	// OnConnOpened 连接建立成功回调
	// requestId: 请求ID
	// authId: 认证ID
	OnConnOpened func(requestId uint32, authId int64)

	// OnConnOpenFailed 连接建立失败回调
	// requestId: 请求ID
	// reason: 失败原因
	OnConnOpenFailed func(requestId uint32, reason int32)

	// OnDataReceived 数据接收回调
	// authId: 认证ID
	// head: 数据头
	// data: 数据内容
	OnDataReceived func(authId int64, head *AuthDataHead, data []byte)
}

// ============================================================================
// 核心结构定义
// ============================================================================

// AuthManager 认证管理器
// 管理单个认证会话的所有信息和状态
type AuthManager struct {
	AuthId         int64              // 认证ID（唯一标识）
	AuthSeq        int64              // 认证序列号
	ConnId         uint64             // 连接ID
	ConnInfo       *AuthConnInfo      // 连接信息
	IsServer       bool               // 是否服务端
	HasAuthPassed  bool               // 是否已通过认证
	LastActiveTime time.Time          // 最后活跃时间
	SessionKeyMgr  *SessionKeyManager // Session Key管理器
	HiChainHandle  interface{}        // HiChain句柄（预留）
	DeviceInfo     *DeviceInfo        // 对端设备信息
	RequestId      uint32             // 原始请求ID（用于回调）

	mu sync.RWMutex // 保护结构体字段
}

// AuthManagerService 认证管理服务
// 全局单例，管理所有认证会话
type AuthManagerService struct {
	managers       map[int64]*AuthManager // authId -> AuthManager映射
	connIdToAuthId map[uint64]int64       // connId -> authId映射
	sessionKeyMgr  *SessionKeyManager     // 全局Session Key管理器
	callback       *AuthConnCallback      // 应用层回调
	initialized    bool                   // 是否已初始化
	authIdCounter  int64                  // AuthId计数器（原子递增）
	seqCounter     int64                  // Seq计数器（原子递增）

	mu sync.RWMutex // 保护服务状态
}

// 全局AuthManagerService实例
var (
	globalAuthMgrService *AuthManagerService
	authMgrServiceOnce   sync.Once
)

// getAuthManagerService 获取全局AuthManagerService实例（单例）
func getAuthManagerService() *AuthManagerService {
	authMgrServiceOnce.Do(func() {
		globalAuthMgrService = &AuthManagerService{
			managers:       make(map[int64]*AuthManager),
			connIdToAuthId: make(map[uint64]int64),
			sessionKeyMgr:  NewSessionKeyManager(),
			authIdCounter:  AuthManagerInitialAuthId,
			seqCounter:     AuthManagerInitialSeq,
		}
	})
	return globalAuthMgrService
}

// ============================================================================
// 初始化和反初始化
// ============================================================================

// AuthDeviceInit 初始化认证设备模块（对应C的AuthDeviceInit）
// callback: 应用层回调接口
func AuthDeviceInit(callback *AuthConnCallback) error {
	service := getAuthManagerService()
	service.mu.Lock()
	defer service.mu.Unlock()

	if callback == nil {
		return fmt.Errorf("callback cannot be nil")
	}

	// 如果已初始化，先清理
	if service.initialized {
		log.Warn("[AUTH_MGR] AuthManagerService already initialized, reinitializing...")
		service.deinitInternal()
	}

	// 初始化 device_auth 服务（HiChain）
	if err := device_auth.InitDeviceAuthService(); err != nil {
		return fmt.Errorf("failed to init device auth service: %w", err)
	}
	log.Info("[AUTH_MGR] Device auth service initialized")

	// 初始化 auth_session 管理器
	if err := AuthSessionInit(); err != nil {
		device_auth.DestroyDeviceAuthService()
		return fmt.Errorf("failed to init auth session manager: %w", err)
	}
	log.Info("[AUTH_MGR] Auth session manager initialized")

	// 初始化Auth Connection层
	authConnListener := &AuthConnListener{
		OnConnectResult: onAuthConnectResult,
		OnDisconnected:  onAuthDisconnected,
		OnDataReceived:  onAuthDataReceived,
	}

	if err := AuthConnInit(authConnListener); err != nil {
		device_auth.DestroyDeviceAuthService() // 回滚
		return fmt.Errorf("failed to init auth connection: %w", err)
	}

	service.callback = callback
	service.initialized = true

	log.Info("[AUTH_MGR] Auth manager service initialized")
	return nil
}

// AuthDeviceDeinit 反初始化认证设备模块（对应C的AuthDeviceDeinit）
func AuthDeviceDeinit() {
	service := getAuthManagerService()
	service.mu.Lock()
	defer service.mu.Unlock()

	service.deinitInternal()
}

// deinitInternal 内部反初始化（需要持有锁）
func (s *AuthManagerService) deinitInternal() {
	if !s.initialized {
		return
	}

	// 收集所有connId（先收集，避免在迭代时修改map）
	connIds := make([]uint64, 0, len(s.managers))
	for _, mgr := range s.managers {
		if mgr.ConnId != 0 {
			connIds = append(connIds, mgr.ConnId)
		}
	}

	// 清空管理器映射（先清空，避免在关闭连接时再次触发回调修改map）
	s.managers = make(map[int64]*AuthManager)
	s.connIdToAuthId = make(map[uint64]int64)

	// 释放锁后关闭底层连接
	s.mu.Unlock()
	for _, connId := range connIds {
		DisconnectAuthDevice(connId)
	}
	s.mu.Lock()

	// 反初始化Auth Connection层
	AuthConnDeinit()

	// 反初始化 auth_session 管理器
	AuthSessionDeinit()
	log.Info("[AUTH_MGR] Auth session manager deinitialized")

	// 销毁 device_auth 服务
	device_auth.DestroyDeviceAuthService()
	log.Info("[AUTH_MGR] Device auth service destroyed")

	// 清理剩余资源
	s.sessionKeyMgr = NewSessionKeyManager()
	s.callback = nil
	s.initialized = false

	log.Info("[AUTH_MGR] Auth manager service deinitialized")
}

// ============================================================================
// 连接管理
// ============================================================================

// AuthOpenConnection 打开认证连接（公开接口，对应C的AuthOpenConnection）
// connInfo: 连接信息
// requestId: 请求ID（用于异步回调匹配）
func AuthOpenConnection(connInfo *AuthConnInfo, requestId uint32) error {
	return AuthDeviceOpenConn(connInfo, requestId, nil)
}

// AuthDeviceOpenConn 打开认证连接（对应C的AuthDeviceOpenConn）
// connInfo: 连接信息
// requestId: 请求ID（用于异步回调匹配）
// callback: 连接回调（可选，如果为nil则使用全局callback）
func AuthDeviceOpenConn(connInfo *AuthConnInfo, requestId uint32, callback *AuthConnCallback) error {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	service.mu.RUnlock()

	if !initialized {
		return fmt.Errorf("auth manager service not initialized")
	}

	if connInfo == nil {
		return fmt.Errorf("connInfo cannot be nil")
	}

	// 更新全局回调（如果提供）
	if callback != nil {
		service.mu.Lock()
		service.callback = callback
		service.mu.Unlock()
	}

	// 创建AuthManager（预创建，等待连接建立）
	authId := atomic.AddInt64(&service.authIdCounter, 1)
	authSeq := atomic.AddInt64(&service.seqCounter, 1)

	manager := &AuthManager{
		AuthId:         authId,
		AuthSeq:        authSeq,
		ConnInfo:       connInfo,
		IsServer:       false, // 主动连接为客户端
		HasAuthPassed:  false,
		LastActiveTime: time.Now(),
		SessionKeyMgr:  service.sessionKeyMgr,
		RequestId:      requestId,
	}

	// 暂存到map（等待OnConnectResult确认）
	service.mu.Lock()
	service.managers[authId] = manager
	service.mu.Unlock()

	log.Infof("[AUTH_MGR] Opening auth connection: authId=%d, requestId=%d, ip=%s, port=%d",
		authId, requestId, connInfo.Ip, connInfo.Port)

	// 调用Auth Connection层建立连接
	if err := ConnectAuthDevice(requestId, connInfo); err != nil {
		// 连接失败，清理AuthManager
		service.mu.Lock()
		delete(service.managers, authId)
		service.mu.Unlock()
		return fmt.Errorf("failed to connect auth device: %w", err)
	}

	return nil
}

// AuthDeviceCloseConn 关闭认证连接（对应C的AuthDeviceCloseConn）
// authId: 认证ID
func AuthDeviceCloseConn(authId int64) {
	service := getAuthManagerService()
	service.mu.Lock()
	initialized := service.initialized
	manager := service.managers[authId]

	if !initialized {
		service.mu.Unlock()
		log.Warn("[AUTH_MGR] Service not initialized in AuthDeviceCloseConn")
		return
	}

	if manager == nil {
		service.mu.Unlock()
		log.Warnf("[AUTH_MGR] AuthManager not found: authId=%d", authId)
		return
	}

	connId := manager.ConnId

	// 立即清理AuthManager映射（避免依赖异步的OnDisconnected回调）
	delete(service.managers, authId)
	delete(service.connIdToAuthId, connId)
	service.mu.Unlock()

	log.Infof("[AUTH_MGR] Closing auth connection: authId=%d, connId=%d", authId, connId)

	// 断开底层连接
	DisconnectAuthDevice(connId)

	log.Infof("[AUTH_MGR] Auth manager removed: authId=%d, connId=%d", authId, connId)
}

// ============================================================================
// 数据发送
// ============================================================================

// AuthDevicePostTransData 发送传输数据（对应C的AuthDevicePostTransData）
// authId: 认证ID
// module: 模块ID
// flag: 标志位
// data: 数据内容
func AuthDevicePostTransData(authId int64, module int32, flag int32, data []byte) error {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	manager := service.managers[authId]
	service.mu.RUnlock()

	if !initialized {
		return fmt.Errorf("auth manager service not initialized")
	}

	if manager == nil {
		return fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	manager.mu.RLock()
	connId := manager.ConnId
	authSeq := manager.AuthSeq
	manager.mu.RUnlock()

	// 构造数据头
	head := &AuthDataHead{
		DataType: ModuleToDataType(module),
		Module:   module,
		Seq:      authSeq,
		Flag:     flag,
		Len:      uint32(len(data)),
	}

	log.Infof("[AUTH_MGR] Posting trans data: authId=%d, connId=%d, module=%d, len=%d",
		authId, connId, module, len(data))

	// 调用Auth Connection层发送数据
	if err := PostAuthData(connId, head, data); err != nil {
		return fmt.Errorf("failed to post auth data: %w", err)
	}

	// 更新活跃时间
	manager.mu.Lock()
	manager.LastActiveTime = time.Now()
	manager.mu.Unlock()

	return nil
}

// ============================================================================
// Session Key 管理
// ============================================================================

// AuthManagerSetSessionKey 设置会话密钥（对应C的AuthManagerSetSessionKey）
// authId: 认证ID
// sessionKey: 会话密钥（16字节）
func AuthManagerSetSessionKey(authId int64, sessionKey []byte) error {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	manager := service.managers[authId]
	service.mu.RUnlock()

	if !initialized {
		return fmt.Errorf("auth manager service not initialized")
	}

	if manager == nil {
		return fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	// 调用SessionKeyManager设置密钥
	index, err := manager.SessionKeyMgr.SetSessionKey(authId, sessionKey)
	if err != nil {
		return fmt.Errorf("failed to set session key: %w", err)
	}

	log.Infof("[AUTH_MGR] Session key set: authId=%d, index=%d", authId, index)
	return nil
}

// AuthManagerGetSessionKey 获取会话密钥（对应C的AuthManagerGetSessionKey）
// authId: 认证ID
// index: 密钥索引
func AuthManagerGetSessionKey(authId int64, index int32) (*SessionKey, error) {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	manager := service.managers[authId]
	service.mu.RUnlock()

	if !initialized {
		return nil, fmt.Errorf("auth manager service not initialized")
	}

	if manager == nil {
		return nil, fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	// 调用SessionKeyManager获取密钥
	key, err := manager.SessionKeyMgr.GetSessionKey(authId, index)
	if err != nil {
		return nil, fmt.Errorf("failed to get session key: %w", err)
	}

	return key, nil
}

// AuthManagerGetLatestSessionKey 获取最新会话密钥
// authId: 认证ID
func AuthManagerGetLatestSessionKey(authId int64) (*SessionKey, error) {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	manager := service.managers[authId]
	service.mu.RUnlock()

	if !initialized {
		return nil, fmt.Errorf("auth manager service not initialized")
	}

	if manager == nil {
		return nil, fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	// 调用SessionKeyManager获取最新密钥
	key, err := manager.SessionKeyMgr.GetLatestSessionKey(authId)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest session key: %w", err)
	}

	return key, nil
}

// ============================================================================
// 设备信息查询
// ============================================================================

// AuthDeviceGetConnInfo 获取连接信息（对应C的AuthDeviceGetConnInfo）
// authId: 认证ID
func AuthDeviceGetConnInfo(authId int64) (*AuthConnInfo, error) {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	manager := service.managers[authId]
	service.mu.RUnlock()

	if !initialized {
		return nil, fmt.Errorf("auth manager service not initialized")
	}

	if manager == nil {
		return nil, fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	return manager.ConnInfo, nil
}

// AuthDeviceGetDeviceUuid 获取对端设备UUID（对应C的AuthDeviceGetDeviceUuid）
// authId: 认证ID
func AuthDeviceGetDeviceUuid(authId int64) (string, error) {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	manager := service.managers[authId]
	service.mu.RUnlock()

	if !initialized {
		return "", fmt.Errorf("auth manager service not initialized")
	}

	if manager == nil {
		return "", fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	if manager.DeviceInfo == nil {
		return "", fmt.Errorf("device info not available")
	}

	return manager.DeviceInfo.UUID, nil
}

// AuthDeviceGetVersion 获取软总线版本（对应C的AuthDeviceGetVersion）
// authId: 认证ID
func AuthDeviceGetVersion(authId int64) (*SoftBusVersion, error) {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	manager := service.managers[authId]
	service.mu.RUnlock()

	if !initialized {
		return nil, fmt.Errorf("auth manager service not initialized")
	}

	if manager == nil {
		return nil, fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	if manager.DeviceInfo == nil {
		return nil, fmt.Errorf("device info not available")
	}

	return &manager.DeviceInfo.Version, nil
}

// AuthDeviceGetServerSide 获取服务端标识（对应C的AuthDeviceGetServerSide）
// authId: 认证ID
func AuthDeviceGetServerSide(authId int64) (bool, error) {
	service := getAuthManagerService()
	service.mu.RLock()
	initialized := service.initialized
	manager := service.managers[authId]
	service.mu.RUnlock()

	if !initialized {
		return false, fmt.Errorf("auth manager service not initialized")
	}

	if manager == nil {
		return false, fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	return manager.IsServer, nil
}

// ============================================================================
// 查找函数
// ============================================================================

// GetAuthManagerByAuthId 根据AuthId获取AuthManager
// authId: 认证ID
func GetAuthManagerByAuthId(authId int64) (*AuthManager, error) {
	service := getAuthManagerService()
	service.mu.RLock()
	defer service.mu.RUnlock()

	manager := service.managers[authId]
	if manager == nil {
		return nil, fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	return manager, nil
}

// GetAuthManagerByConnId 根据ConnId获取AuthManager
// connId: 连接ID
func GetAuthManagerByConnId(connId uint64) (*AuthManager, error) {
	service := getAuthManagerService()
	service.mu.RLock()
	defer service.mu.RUnlock()

	authId, exists := service.connIdToAuthId[connId]
	if !exists {
		return nil, fmt.Errorf("auth manager not found for connId=%d", connId)
	}

	manager := service.managers[authId]
	if manager == nil {
		return nil, fmt.Errorf("auth manager not found: authId=%d", authId)
	}

	return manager, nil
}

// ============================================================================
// Auth Connection 回调处理
// ============================================================================

// onAuthConnectResult Auth Connection连接结果回调
func onAuthConnectResult(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo) {
	service := getAuthManagerService()

	log.Infof("[AUTH_MGR] OnConnectResult: requestId=%d, connId=%d, result=%d",
		requestId, connId, result)

	// 查找对应的AuthManager（通过requestId）
	service.mu.Lock()
	var manager *AuthManager
	for _, mgr := range service.managers {
		if mgr.RequestId == requestId {
			manager = mgr
			break
		}
	}

	if manager == nil {
		service.mu.Unlock()
		log.Warnf("[AUTH_MGR] AuthManager not found for requestId=%d", requestId)
		return
	}

	authId := manager.AuthId

	if result == 0 {
		// 连接成功
		manager.ConnId = connId
		service.connIdToAuthId[connId] = authId
		service.mu.Unlock()

		log.Infof("[AUTH_MGR] Connection established: authId=%d, connId=%d", authId, connId)

		// 调用auth_session启动认证流程
		err := AuthSessionStartAuth(manager.AuthSeq, requestId, connId, connInfo, false)
		if err != nil {
			log.Errorf("[AUTH_MGR] Failed to start auth session: %v", err)
			// 认证启动失败，通知应用层
			if service.callback != nil && service.callback.OnConnOpenFailed != nil {
				service.callback.OnConnOpenFailed(requestId, AuthResultFailed)
			}
			return
		}

		// 将AuthSession关联到AuthManager
		session, _ := GetAuthSessionByConnId(connId)
		if session != nil {
			session.AuthManager = manager
		}
	} else {
		// 连接失败
		delete(service.managers, authId)
		service.mu.Unlock()

		// 调用应用层回调
		if service.callback != nil && service.callback.OnConnOpenFailed != nil {
			service.callback.OnConnOpenFailed(requestId, result)
		}

		log.Errorf("[AUTH_MGR] Connection failed: requestId=%d, result=%d", requestId, result)
	}
}

// onAuthDisconnected Auth Connection断开回调
func onAuthDisconnected(connId uint64, connInfo *AuthConnInfo) {
	service := getAuthManagerService()

	log.Infof("[AUTH_MGR] OnDisconnected: connId=%d", connId)

	service.mu.Lock()
	authId, exists := service.connIdToAuthId[connId]
	if !exists {
		service.mu.Unlock()
		log.Warnf("[AUTH_MGR] AuthId not found for connId=%d", connId)
		return
	}

	manager := service.managers[authId]
	delete(service.managers, authId)
	delete(service.connIdToAuthId, connId)
	service.mu.Unlock()

	if manager != nil {
		log.Infof("[AUTH_MGR] Auth manager removed: authId=%d, connId=%d", authId, connId)
	}
}

// onAuthDataReceived Auth Connection数据接收回调
func onAuthDataReceived(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte) {
	service := getAuthManagerService()

	service.mu.Lock()
	authId, exists := service.connIdToAuthId[connId]
	if !exists {
		// 服务端收到第一个认证数据时，创建AuthManager和AuthSession
		if fromServer && head.Module == ModuleAuthSdk {
			authId = atomic.AddInt64(&service.authIdCounter, 1)
			authSeq := atomic.AddInt64(&service.seqCounter, 1)

			manager := &AuthManager{
				AuthId:         authId,
				AuthSeq:        authSeq,
				ConnId:         connId,
				ConnInfo:       connInfo,
				IsServer:       true,
				HasAuthPassed:  false,
				LastActiveTime: time.Now(),
				SessionKeyMgr:  service.sessionKeyMgr,
				RequestId:      0,
			}

			service.managers[authId] = manager
			service.connIdToAuthId[connId] = authId
			service.mu.Unlock()

			log.Infof("[AUTH_MGR] Created server AuthManager: authId=%d, connId=%d", authId, connId)

			// 启动认证会话并关联AuthManager
			if err := AuthSessionStartAuth(authSeq, 0, connId, connInfo, true); err != nil {
				log.Errorf("[AUTH_MGR] Failed to start auth session: %v", err)
				return
			}

			// 关联AuthSession到AuthManager
			session, err := GetAuthSessionByConnId(connId)
			if err == nil {
				session.AuthManager = manager
			}

			// 继续处理数据
			service.mu.Lock()
		} else {
			service.mu.Unlock()
			log.Warnf("[AUTH_MGR] AuthId not found for connId=%d", connId)
			return
		}
	}

	manager := service.managers[authId]
	service.mu.Unlock()

	if manager == nil {
		log.Warnf("[AUTH_MGR] AuthManager not found: authId=%d", authId)
		return
	}

	// 更新活跃时间
	manager.mu.Lock()
	manager.LastActiveTime = time.Now()
	manager.mu.Unlock()

	log.Infof("[AUTH_MGR] OnDataReceived: authId=%d, connId=%d, module=%d, len=%d",
		authId, connId, head.Module, len(data))

	// 消息路由：根据模块类型路由到auth_session层
	switch head.Module {
	case ModuleTrustEngine:
		// MODULE_TRUST_ENGINE (1) - 设备ID交换
		err := AuthSessionProcessDevIdData(head.Seq, data)
		if err != nil {
			log.Errorf("[AUTH_MGR] Failed to process device ID data: %v", err)
		}

	case ModuleAuthSdk:
		// MODULE_AUTH_SDK (3) - HiChain认证数据
		err := AuthSessionProcessAuthData(head.Seq, data)
		if err != nil {
			log.Errorf("[AUTH_MGR] Failed to process auth data: %v", err)
		}

	case ModuleAuthConnection:
		// MODULE_AUTH_CONNECTION (5) - 设备信息交换
		// TODO: 实现 AuthSessionProcessDevInfoData
		log.Warnf("[AUTH_MGR] MODULE_AUTH_CONNECTION not yet implemented")

	case ModuleAuthMsg:
		// MODULE_AUTH_MSG (9) - 业务数据，直接回调到应用层
		if service.callback != nil && service.callback.OnDataReceived != nil {
			service.callback.OnDataReceived(authId, head, data)
		}

	default:
		// 其他模块，直接回调到应用层
		if service.callback != nil && service.callback.OnDataReceived != nil {
			service.callback.OnDataReceived(authId, head, data)
		}
	}
}

// ============================================================================
// 工具函数
// ============================================================================

// GetAllAuthManagers 获取所有AuthManager（用于调试）
func GetAllAuthManagers() []*AuthManager {
	service := getAuthManagerService()
	service.mu.RLock()
	defer service.mu.RUnlock()

	managers := make([]*AuthManager, 0, len(service.managers))
	for _, mgr := range service.managers {
		managers = append(managers, mgr)
	}
	return managers
}

// ============================================================================
// HiChain 集成
// ============================================================================

// handleHiChainData 处理 HiChain 认证数据
func handleHiChainData(manager *AuthManager, head *AuthDataHead, data []byte) {
	log.Infof("[AUTH_MGR] Handling HiChain data: authId=%d, seq=%d, len=%d",
		manager.AuthId, head.Seq, len(data))

	// 获取 device_auth GroupAuthManager 实例
	ga, err := device_auth.GetGaInstance()
	if err != nil {
		log.Errorf("[AUTH_MGR] Failed to get GroupAuthManager: %v", err)
		return
	}

	// 创建 device_auth 回调
	callback := createDeviceAuthCallback(manager)

	// 调用 ProcessData 处理认证数据
	if err := ga.ProcessData(head.Seq, data, callback); err != nil {
		log.Errorf("[AUTH_MGR] HiChain ProcessData failed: %v", err)
		// 通知应用层认证失败
		notifyAuthResult(manager, AuthResultFailed)
	}
}

// StartHiChainAuth 启动 HiChain 认证（客户端发起）
func StartHiChainAuth(manager *AuthManager) error {
	if manager == nil {
		return fmt.Errorf("manager is nil")
	}

	log.Infof("[AUTH_MGR] Starting HiChain auth: authId=%d", manager.AuthId)

	// 获取 device_auth GroupAuthManager 实例
	ga, err := device_auth.GetGaInstance()
	if err != nil {
		return fmt.Errorf("failed to get GroupAuthManager: %w", err)
	}

	// 构建认证参数
	// TODO: 从 manager.DeviceInfo 获取对端 UDID
	authParams := `{"peerUdid":"peer-device","serviceType":"softbus_auth"}`

	// 创建 device_auth 回调
	callback := createDeviceAuthCallback(manager)

	// 发起认证
	if err := ga.AuthDevice(device_auth.AnyOsAccount, manager.AuthSeq, authParams, callback); err != nil {
		return fmt.Errorf("failed to start HiChain auth: %w", err)
	}

	log.Infof("[AUTH_MGR] HiChain auth started: authId=%d, authSeq=%d", manager.AuthId, manager.AuthSeq)
	return nil
}

// createDeviceAuthCallback 创建 device_auth 回调
func createDeviceAuthCallback(manager *AuthManager) *device_auth.DeviceAuthCallback {
	return &device_auth.DeviceAuthCallback{
		// OnTransmit: HiChain 需要发送数据
		OnTransmit: func(requestId int64, data []byte) bool {
			log.Infof("[AUTH_MGR] HiChain OnTransmit: authId=%d, requestId=%d, len=%d",
				manager.AuthId, requestId, len(data))

			// 通过 AuthConnection 发送 MODULE_AUTH_SDK 数据
			head := &AuthDataHead{
				Module: ModuleAuthSdk,
				Seq:    requestId,
				Flag:   0,
			}

			err := PostAuthData(manager.ConnId, head, data)
			if err != nil {
				log.Errorf("[AUTH_MGR] Failed to send HiChain data: %v", err)
				return false
			}
			return true
		},

		// OnSessionKeyReturned: HiChain 派生了会话密钥
		OnSessionKeyReturned: func(requestId int64, sessionKey []byte) {
			log.Infof("[AUTH_MGR] HiChain OnSessionKeyReturned: authId=%d, keyLen=%d",
				manager.AuthId, len(sessionKey))

			// 存储会话密钥到 SessionKeyManager
			_, err := manager.SessionKeyMgr.SetSessionKey(manager.AuthId, sessionKey)
			if err != nil {
				log.Errorf("[AUTH_MGR] Failed to store session key: %v", err)
			}
		},

		// OnFinish: HiChain 认证成功
		OnFinish: func(requestId int64, operationCode int32, returnData string) {
			log.Infof("[AUTH_MGR] HiChain OnFinish: authId=%d, opCode=%d",
				manager.AuthId, operationCode)

			// 标记认证成功
			manager.mu.Lock()
			manager.HasAuthPassed = true
			manager.mu.Unlock()

			// 通知应用层认证成功
			notifyAuthResult(manager, AuthResultSuccess)
		},

		// OnError: HiChain 认证失败
		OnError: func(requestId int64, operationCode int32, errorCode int32, errorReturn string) {
			log.Errorf("[AUTH_MGR] HiChain OnError: authId=%d, errorCode=%d, error=%s",
				manager.AuthId, errorCode, errorReturn)

			// 通知应用层认证失败
			notifyAuthResult(manager, AuthResultFailed)
		},

		// OnRequest: HiChain 请求参数（预留）
		OnRequest: func(requestId int64, operationCode int32, reqParams string) string {
			log.Infof("[AUTH_MGR] HiChain OnRequest: authId=%d, opCode=%d",
				manager.AuthId, operationCode)
			return "{}"
		},
	}
}

// notifyAuthResult 通知应用层认证结果
func notifyAuthResult(manager *AuthManager, result int32) {
	service := getAuthManagerService()
	service.mu.RLock()
	callback := service.callback
	service.mu.RUnlock()

	if callback == nil {
		return
	}

	if result == AuthResultSuccess {
		// 认证成功：调用 OnConnOpened
		if callback.OnConnOpened != nil {
			callback.OnConnOpened(manager.RequestId, manager.AuthId)
		}
	} else {
		// 认证失败：调用 OnConnOpenFailed
		if callback.OnConnOpenFailed != nil {
			callback.OnConnOpenFailed(manager.RequestId, result)
		}
	}
}

// GetAuthManagerCount 获取AuthManager数量（用于调试）
func GetAuthManagerCount() int {
	service := getAuthManagerService()
	service.mu.RLock()
	defer service.mu.RUnlock()

	return len(service.managers)
}
