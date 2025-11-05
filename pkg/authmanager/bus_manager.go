package authmanager

import (
	"fmt"
	"sync"

	"github.com/junbin-yang/dsoftbus-go/pkg/session"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// BusManager 管理认证总线，负责启动和停止认证服务器与会话服务器，协调认证流程
type BusManager struct {
	authServer   *TCPServer                 // 认证服务器（处理认证相关连接）
	sessionMgr   *session.TcpSessionManager // 会话管理器（处理认证后的会话连接）
	authMgr      *AuthManager               // 认证管理器（核心认证逻辑）
	localDevInfo *DeviceInfo                // 本地设备信息
	started      bool                       // 总线是否已启动的标志
	mu           sync.Mutex                 // 保护启动状态和资源的互斥锁
}

// NewBusManager 创建新的总线管理器实例
// 参数：
//   - devInfo：本地设备信息
// 返回：
//   - 初始化后的BusManager实例
func NewBusManager(devInfo *DeviceInfo) *BusManager {
	return &BusManager{
		localDevInfo: devInfo,
		started:      false, // 初始状态为未启动
	}
}

// Start 启动总线管理器（包括认证服务器和会话服务器）
// 返回：
//   - 错误信息（启动失败时）
func (bm *BusManager) Start() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.started {
		return nil
	}

	// 启动认证服务器
	authMgr := NewAuthManager(0, 0, bm.localDevInfo)
	authServer := NewTCPServer(authMgr)

	// 监听本地IP的随机端口（端口0表示自动分配）
	authAddr := fmt.Sprintf("%s:0", bm.localDevInfo.DeviceIP)
	if err := authServer.Start(authAddr); err != nil {
		return fmt.Errorf("启动认证服务器失败: %v", err)
	}

	// 获取认证服务器实际监听的端口
	authPort := authServer.GetPort()
	log.Infof("[AUTH] 认证服务器已在端口%d启动", authPort)

	// 启动会话管理器（使用新的TcpSessionManager）
	sessionMgr := session.NewTcpSessionManager(true, bm.localDevInfo.DeviceIP, authMgr)
	sessionPort, err := sessionMgr.Start()
	if err != nil {
		authServer.Stop() // 会话服务器启动失败时，关闭已启动的认证服务器
		return fmt.Errorf("启动会话服务器失败: %v", err)
	}

	log.Infof("[SESSION] 会话管理器已在端口%d启动", sessionPort)

	// 更新认证管理器的实际端口
	authMgr.authPort = authPort
	authMgr.sessionPort = sessionPort
	bm.localDevInfo.DevicePort = authPort // 本地设备端口设为认证端口

	// 保存启动的组件
	bm.authServer = authServer
	bm.sessionMgr = sessionMgr
	bm.authMgr = authMgr
	bm.started = true

	log.Info("[AUTH] BusManager启动成功")
	return nil
}

// Stop 停止总线管理器（包括所有服务器）
// 返回：
//   - 错误信息（停止失败时）
func (bm *BusManager) Stop() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.started {
		return nil
	}

	// 停止认证服务器
	if bm.authServer != nil {
		bm.authServer.Stop()
	}
	// 停止会话管理器
	if bm.sessionMgr != nil {
		bm.sessionMgr.Stop()
	}

	// 重置状态
	bm.localDevInfo.DevicePort = -1
	bm.started = false

	log.Info("[AUTH] BusManager已停止")
	return nil
}

// GetLocalDeviceInfo 返回本地设备信息
// 返回：
//   - 本地设备信息（DeviceInfo实例）
func (bm *BusManager) GetLocalDeviceInfo() *DeviceInfo {
	return bm.localDevInfo
}

// GetAuthManager 返回认证管理器
// 返回：
//   - 认证管理器（AuthManager实例）
func (bm *BusManager) GetAuthManager() *AuthManager {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.authMgr
}

// IsStarted 检查总线管理器是否已启动
// 返回：
//   - 启动状态（true表示已启动，false表示未启动）
func (bm *BusManager) IsStarted() bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.started
}

// GetSessionManager 返回会话管理器
// 返回：
//   - 会话管理器（TcpSessionManager实例）
func (bm *BusManager) GetSessionManager() *session.TcpSessionManager {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.sessionMgr
}

// CreateSessionServer 创建会话服务器（便捷方法）
// 参数：
//   - moduleName：模块名称
//   - sessionName：会话名称
//   - listener：会话监听器
// 返回：错误信息
func (bm *BusManager) CreateSessionServer(moduleName, sessionName string, listener session.ISessionListener) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.started || bm.sessionMgr == nil {
		return fmt.Errorf("BusManager未启动")
	}

	return bm.sessionMgr.CreateSessionServer(moduleName, sessionName, listener)
}

// RemoveSessionServer 移除会话服务器（便捷方法）
// 参数：
//   - moduleName：模块名称
//   - sessionName：会话名称
// 返回：错误信息
func (bm *BusManager) RemoveSessionServer(moduleName, sessionName string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.started || bm.sessionMgr == nil {
		return fmt.Errorf("BusManager未启动")
	}

	return bm.sessionMgr.RemoveSessionServer(moduleName, sessionName)
}

// GetSessionServer 获取会话服务器（便捷方法）
// 参数：
//   - sessionName：完整的会话名称（moduleName/sessionName格式）
// 返回：SessionServer或nil
func (bm *BusManager) GetSessionServer(sessionName string) *session.SessionServer {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.started || bm.sessionMgr == nil {
		return nil
	}

	return bm.sessionMgr.GetSessionServer(sessionName)
}

// SendBytes 发送数据到会话（便捷方法）
// 参数：
//   - sessionID：会话ID
//   - data：要发送的数据
// 返回：错误信息
func (bm *BusManager) SendBytes(sessionID int, data []byte) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.started || bm.sessionMgr == nil {
		return fmt.Errorf("BusManager未启动")
	}

	return bm.sessionMgr.SendBytes(sessionID, data)
}

// CloseSession 关闭会话（便捷方法）
// 参数：
//   - sessionID：会话ID
// 返回：错误信息
func (bm *BusManager) CloseSession(sessionID int) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.started || bm.sessionMgr == nil {
		return fmt.Errorf("BusManager未启动")
	}

	return bm.sessionMgr.CloseSession(sessionID)
}

// OpenSession 作为客户端打开到远程设备的会话
// 参数：
//   - peerIP：对端IP地址
//   - peerPort：对端会话端口
//   - sessionName：会话名称
//   - myDeviceId：本地设备ID
// 返回：sessionID和错误信息
func (bm *BusManager) OpenSession(peerIP string, peerPort int, sessionName, myDeviceId string) (int, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.started || bm.sessionMgr == nil {
		return -1, fmt.Errorf("BusManager未启动")
	}

	return bm.sessionMgr.OpenSession(peerIP, peerPort, sessionName, myDeviceId)
}
