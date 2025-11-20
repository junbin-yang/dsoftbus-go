/*
 * auth_session.go - 认证会话层
 * 对应C代码: auth_session_fsm.c
 *
 * 这一层负责：
 * 1. 管理认证会话状态机
 * 2. 设备ID交换
 * 3. 调用HiChain进行设备认证
 * 4. 处理认证数据路由
 */

package authentication

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/junbin-yang/dsoftbus-go/pkg/context"
	"github.com/junbin-yang/dsoftbus-go/pkg/device_auth"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// AuthSessionState 认证会话状态
type AuthSessionState int

const (
	StateInit         AuthSessionState = 0 // 初始状态
	StateSyncDeviceId AuthSessionState = 1 // 同步设备ID
	StateDeviceAuth   AuthSessionState = 2 // 设备认证（HiChain）
	StateAuthDone     AuthSessionState = 3 // 认证完成
	StateFailed       AuthSessionState = 4 // 认证失败
)

// AuthSession 认证会话（对应C的AuthFsm）
type AuthSession struct {
	AuthSeq        int64            // 认证序列号
	RequestId      uint32           // 请求ID
	ConnId         uint64           // 连接ID
	ConnInfo       *AuthConnInfo    // 连接信息
	IsServer       bool             // 是否服务端
	State          AuthSessionState // 当前状态
	AuthManager    *AuthManager     // 关联的AuthManager
	CreateTime     time.Time        // 创建时间
	LastUpdateTime time.Time        // 最后更新时间
	mu             sync.RWMutex     // 保护状态
}

// AuthSessionManager 全局会话管理器
type AuthSessionManager struct {
	sessions      map[int64]*AuthSession // authSeq -> session
	connIdToSeq   map[uint64]int64       // connId -> authSeq
	seqCounter    int64                  // 序列号计数器
	mu            sync.RWMutex
	initialized   bool
}

var (
	sessionManager     *AuthSessionManager
	sessionManagerOnce sync.Once
)

// getAuthSessionManager 获取会话管理器单例
func getAuthSessionManager() *AuthSessionManager {
	sessionManagerOnce.Do(func() {
		sessionManager = &AuthSessionManager{
			sessions:    make(map[int64]*AuthSession),
			connIdToSeq: make(map[uint64]int64),
			initialized: false,
		}
	})
	return sessionManager
}

// AuthSessionInit 初始化会话管理器
func AuthSessionInit() error {
	mgr := getAuthSessionManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if mgr.initialized {
		log.Warn("[AUTH_SESSION] Session manager already initialized")
		return nil
	}

	mgr.initialized = true
	log.Info("[AUTH_SESSION] Session manager initialized")
	return nil
}

// AuthSessionDeinit 反初始化会话管理器
func AuthSessionDeinit() {
	mgr := getAuthSessionManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if !mgr.initialized {
		return
	}

	// 清理所有会话
	for authSeq := range mgr.sessions {
		delete(mgr.sessions, authSeq)
	}
	mgr.connIdToSeq = make(map[uint64]int64)
	mgr.initialized = false

	log.Info("[AUTH_SESSION] Session manager deinitialized")
}

// AuthSessionStartAuth 启动认证会话
// 对应C代码: AuthSessionStartAuth()
func AuthSessionStartAuth(authSeq int64, requestId uint32, connId uint64,
	connInfo *AuthConnInfo, isServer bool) error {

	mgr := getAuthSessionManager()
	mgr.mu.Lock()

	if !mgr.initialized {
		mgr.mu.Unlock()
		return fmt.Errorf("session manager not initialized")
	}

	// 检查是否已存在
	if _, exists := mgr.sessions[authSeq]; exists {
		mgr.mu.Unlock()
		return fmt.Errorf("auth session already exists: authSeq=%d", authSeq)
	}

	// 创建新会话
	session := &AuthSession{
		AuthSeq:        authSeq,
		RequestId:      requestId,
		ConnId:         connId,
		ConnInfo:       connInfo,
		IsServer:       isServer,
		State:          StateInit,
		CreateTime:     time.Now(),
		LastUpdateTime: time.Now(),
	}

	mgr.sessions[authSeq] = session
	mgr.connIdToSeq[connId] = authSeq
	mgr.mu.Unlock()

	log.Infof("[AUTH_SESSION] Auth session started: authSeq=%d, requestId=%d, connId=%d, isServer=%v",
		authSeq, requestId, connId, isServer)

	// 启动状态机
	return session.startStateMachine()
}

// startStateMachine 启动状态机
func (s *AuthSession) startStateMachine() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.IsServer {
		// 服务端：等待客户端发送设备ID
		s.State = StateSyncDeviceId
		log.Infof("[AUTH_SESSION] Server waiting for device ID: authSeq=%d", s.AuthSeq)
	} else {
		// 客户端：发送设备ID并启动HiChain认证
		s.State = StateSyncDeviceId
		log.Infof("[AUTH_SESSION] Client starting device auth: authSeq=%d", s.AuthSeq)

		// 直接进入HiChain认证状态
		s.State = StateDeviceAuth
		return s.startHiChainAuth()
	}

	return nil
}

// startHiChainAuth 启动HiChain认证
func (s *AuthSession) startHiChainAuth() error {
	// 获取本地设备信息
	localDevInfo, err := GetLocalDeviceInfo()
	if err != nil {
		log.Errorf("[AUTH_SESSION] Failed to get local device info: %v", err)
		s.State = StateFailed
		return err
	}

	// 创建AuthSessionContext（客户端和服务端都需要）
	ctx := &context.AuthSessionContext{
		ChannelID:     int(s.ConnId),
		PinCode:       "888888",
		RequestID:     s.AuthSeq,
		LocalDeviceID: localDevInfo.UDID,
		PeerDeviceID:  "",
	}
	context.SetAuthSessionContext(int(s.AuthSeq), ctx)
	log.Infof("[AUTH_SESSION] Created AuthSessionContext: authSeq=%d, connId=%d, isServer=%v", s.AuthSeq, s.ConnId, s.IsServer)

	// 获取GroupAuthManager实例
	ga, err := device_auth.GetGaInstance()
	if err != nil {
		log.Errorf("[AUTH_SESSION] Failed to get GroupAuthManager: %v", err)
		s.State = StateFailed
		return err
	}

	// 构建认证参数
	authParams := `{"peerUdid":"peer-device","serviceType":"softbus_auth"}`

	// 创建device_auth回调
	callback := s.createDeviceAuthCallback()

	// 发起认证
	log.Infof("[AUTH_SESSION] Starting HiChain auth: authSeq=%d", s.AuthSeq)
	err = ga.AuthDevice(device_auth.AnyOsAccount, s.AuthSeq, authParams, callback)
	if err != nil {
		log.Errorf("[AUTH_SESSION] Failed to start HiChain auth: %v", err)
		s.State = StateFailed
		return err
	}

	return nil
}

// createDeviceAuthCallback 创建device_auth回调
func (s *AuthSession) createDeviceAuthCallback() *device_auth.DeviceAuthCallback {
	return &device_auth.DeviceAuthCallback{
		// OnTransmit: HiChain需要发送数据
		OnTransmit: func(requestId int64, data []byte) bool {
			channelData := &AuthChannelData{
				Module: ModuleAuthSdk,
				Flag:   0,
				Seq:    requestId,
				Len:    uint32(len(data)),
				Data:   data,
			}
			// 使用connId的低32位作为channelId (fd)
			channelId := int(s.ConnId & 0xFFFFFFFF)
			log.Infof("[AUTH_SESSION] Posting channel data: channelId=%d, module=%d, seq=%d, len=%d", channelId, channelData.Module, channelData.Seq, channelData.Len)
			err := AuthPostChannelData(channelId, channelData)
			if err != nil {
				log.Errorf("[AUTH_SESSION] Failed to post channel data: %v", err)
				return false
			}
			log.Infof("[AUTH_SESSION] Successfully posted channel data")
			return true
		},

		// OnSessionKeyReturned: HiChain派生了会话密钥
		OnSessionKeyReturned: func(requestId int64, sessionKey []byte) {
			if s.AuthManager != nil {
				_, err := s.AuthManager.SessionKeyMgr.SetSessionKey(s.AuthManager.AuthId, sessionKey)
				if err != nil {
					log.Errorf("[AUTH_SESSION] Failed to store session key: %v", err)
				} else {
					log.Infof("[AUTH_SESSION] Session key stored: authSeq=%d", s.AuthSeq)
				}
			}
		},

		// OnFinish: HiChain认证成功
		OnFinish: func(requestId int64, operationCode int32, returnData string) {
			s.mu.Lock()
			s.State = StateAuthDone
			s.mu.Unlock()

			log.Infof("[AUTH_SESSION] HiChain auth finished: authSeq=%d", s.AuthSeq)

			// 标记AuthManager认证成功
			if s.AuthManager != nil {
				s.AuthManager.mu.Lock()
				s.AuthManager.HasAuthPassed = true
				s.AuthManager.mu.Unlock()
			}

			// 通知应用层认证成功
			s.notifyAuthResult(AuthResultSuccess)
		},

		// OnError: HiChain认证失败
		OnError: func(requestId int64, operationCode int32, errorCode int32, errorReturn string) {
			s.mu.Lock()
			s.State = StateFailed
			s.mu.Unlock()

			log.Errorf("[AUTH_SESSION] HiChain auth failed: authSeq=%d, errorCode=%d",
				s.AuthSeq, errorCode)

			// 通知应用层认证失败
			s.notifyAuthResult(AuthResultFailed)
		},

		// OnRequest: HiChain请求参数（预留）
		OnRequest: func(requestId int64, operationCode int32, reqParams string) string {
			return "{}"
		},
	}
}

// notifyAuthResult 通知认证结果
func (s *AuthSession) notifyAuthResult(result int32) {
	service := getAuthManagerService()
	if service.callback == nil {
		return
	}

	if result == AuthResultSuccess {
		if service.callback.OnConnOpened != nil {
			authId := int64(0)
			if s.AuthManager != nil {
				authId = s.AuthManager.AuthId
			}
			service.callback.OnConnOpened(s.RequestId, authId)
		}
	} else {
		if service.callback.OnConnOpenFailed != nil {
			service.callback.OnConnOpenFailed(s.RequestId, result)
		}
	}
}

// AuthSessionProcessDevIdData 处理设备ID数据
// 对应C代码: AuthSessionProcessDevIdData()
func AuthSessionProcessDevIdData(authSeq int64, data []byte) error {
	mgr := getAuthSessionManager()
	mgr.mu.RLock()
	session, exists := mgr.sessions[authSeq]
	mgr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("auth session not found: authSeq=%d", authSeq)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	log.Infof("[AUTH_SESSION] Processing device ID data: authSeq=%d, len=%d", authSeq, len(data))

	// TODO: 解析设备ID数据
	// TODO: 验证设备ID

	// 服务端收到设备ID后，启动HiChain认证
	if session.IsServer && session.State == StateSyncDeviceId {
		session.State = StateDeviceAuth
		return session.startHiChainAuth()
	}

	return nil
}

// AuthSessionProcessAuthData 处理HiChain认证数据
// 对应C代码: AuthSessionProcessAuthData()
func AuthSessionProcessAuthData(authSeq int64, data []byte) error {
	mgr := getAuthSessionManager()
	mgr.mu.RLock()
	session, exists := mgr.sessions[authSeq]
	mgr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("auth session not found: authSeq=%d", authSeq)
	}

	log.Infof("[AUTH_SESSION] Processing auth data: authSeq=%d, len=%d", authSeq, len(data))

	// 服务端首次收到认证数据时，需要先启动HiChain
	session.mu.Lock()
	if session.IsServer && session.State == StateSyncDeviceId {
		session.State = StateDeviceAuth
		session.mu.Unlock()
		if err := session.startHiChainAuth(); err != nil {
			return fmt.Errorf("failed to start HiChain auth: %w", err)
		}
	} else {
		session.mu.Unlock()
	}

	// 调用device_auth处理数据
	ga, err := device_auth.GetGaInstance()
	if err != nil {
		return fmt.Errorf("failed to get GroupAuthManager: %w", err)
	}

	callback := session.createDeviceAuthCallback()
	return ga.ProcessData(authSeq, data, callback)
}

// AuthSessionHandleAuthFinish 处理认证完成
func AuthSessionHandleAuthFinish(authSeq int64) error {
	mgr := getAuthSessionManager()
	mgr.mu.RLock()
	session, exists := mgr.sessions[authSeq]
	mgr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("auth session not found: authSeq=%d", authSeq)
	}

	session.mu.Lock()
	session.State = StateAuthDone
	session.mu.Unlock()

	log.Infof("[AUTH_SESSION] Auth finished: authSeq=%d", authSeq)
	return nil
}

// AuthSessionHandleAuthError 处理认证错误
func AuthSessionHandleAuthError(authSeq int64, reason int32) error {
	mgr := getAuthSessionManager()
	mgr.mu.RLock()
	session, exists := mgr.sessions[authSeq]
	mgr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("auth session not found: authSeq=%d", authSeq)
	}

	session.mu.Lock()
	session.State = StateFailed
	session.mu.Unlock()

	log.Errorf("[AUTH_SESSION] Auth error: authSeq=%d, reason=%d", authSeq, reason)
	return nil
}

// GetAuthSessionByConnId 根据ConnId获取会话
func GetAuthSessionByConnId(connId uint64) (*AuthSession, error) {
	mgr := getAuthSessionManager()
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	authSeq, exists := mgr.connIdToSeq[connId]
	if !exists {
		return nil, fmt.Errorf("auth session not found for connId=%d", connId)
	}

	session, exists := mgr.sessions[authSeq]
	if !exists {
		return nil, fmt.Errorf("auth session not found: authSeq=%d", authSeq)
	}

	return session, nil
}

// generateAuthSeq 生成认证序列号
func generateAuthSeq() int64 {
	mgr := getAuthSessionManager()
	return atomic.AddInt64(&mgr.seqCounter, 1)
}
