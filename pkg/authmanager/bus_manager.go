package authmanager

import (
	"fmt"
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// BusManager 管理认证总线，负责启动和停止认证服务器与会话服务器，协调认证流程
type BusManager struct {
	authServer    *TCPServer   // 认证服务器（处理认证相关连接）
	sessionServer *TCPServer   // 会话服务器（处理认证后的会话连接）
	authMgr       *AuthManager // 认证管理器（核心认证逻辑）
	localDevInfo  *DeviceInfo  // 本地设备信息
	started       bool         // 总线是否已启动的标志
	mu            sync.Mutex   // 保护启动状态和资源的互斥锁
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

	// 启动会话服务器（占位实现，完整版本中会处理实际会话）
	sessionServer := NewTCPServer(authMgr)
	sessionAddr := fmt.Sprintf("%s:0", bm.localDevInfo.DeviceIP)
	if err := sessionServer.Start(sessionAddr); err != nil {
		authServer.Stop() // 会话服务器启动失败时，关闭已启动的认证服务器
		return fmt.Errorf("启动会话服务器失败: %v", err)
	}

	// 获取会话服务器实际监听的端口
	sessionPort := sessionServer.GetPort()
	log.Infof("[AUTH] 会话服务器已在端口%d启动", sessionPort)

	// 更新认证管理器的实际端口
	authMgr.authPort = authPort
	authMgr.sessionPort = sessionPort
	bm.localDevInfo.DevicePort = authPort // 本地设备端口设为认证端口

	// 保存启动的组件
	bm.authServer = authServer
	bm.sessionServer = sessionServer
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
	// 停止会话服务器
	if bm.sessionServer != nil {
		bm.sessionServer.Stop()
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
