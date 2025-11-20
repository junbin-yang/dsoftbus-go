package frame

import (
	"fmt"
	"sync"

	"github.com/junbin-yang/dsoftbus-go/pkg/authentication"
	"github.com/junbin-yang/dsoftbus-go/pkg/bus_center"
	"github.com/junbin-yang/dsoftbus-go/pkg/device_auth"
	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
	"github.com/junbin-yang/dsoftbus-go/pkg/transmission"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

var (
	gIsInit bool
	gMutex  sync.Mutex
)

// InitSoftBusServer 初始化软总线服务器
func InitSoftBusServer() error {
	gMutex.Lock()
	defer gMutex.Unlock()

	if gIsInit {
		return nil
	}

	logger.Info("[Frame] 正在初始化软总线框架...")

	// 1. 初始化Discovery (先初始化以获取设备信息)
	if err := discServerInit(); err != nil {
		logger.Errorf("[Frame] Discovery初始化失败: %v", err)
		return err
	}

	// 2. 初始化Bus Center (使用Discovery的设备信息)
	if err := busCenterServerInit(); err != nil {
		logger.Errorf("[Frame] Bus Center初始化失败: %v", err)
		serverModuleDeinit()
		return err
	}

	// 3. 初始化Authentication
	if err := authInit(); err != nil {
		logger.Errorf("[Frame] Authentication初始化失败: %v", err)
		serverModuleDeinit()
		return err
	}

	// 4. 初始化Transmission
	if err := transServerInit(); err != nil {
		logger.Errorf("[Frame] Transmission初始化失败: %v", err)
		serverModuleDeinit()
		return err
	}

	gIsInit = true
	logger.Info("[Frame] 软总线框架初始化成功")
	return nil
}

// GetServerIsInit 获取服务器初始化状态
func GetServerIsInit() bool {
	gMutex.Lock()
	defer gMutex.Unlock()
	return gIsInit
}

// DeinitSoftBusServer 反初始化软总线服务器
func DeinitSoftBusServer() {
	gMutex.Lock()
	defer gMutex.Unlock()

	if !gIsInit {
		return
	}

	logger.Info("[Frame] 正在关闭软总线框架...")
	serverModuleDeinit()
	gIsInit = false
	logger.Info("[Frame] 软总线框架已关闭")
}

// serverModuleDeinit 反初始化所有模块
func serverModuleDeinit() {
	transmission.TransServerDeinit()
	authentication.StopSocketListening()
	authentication.AuthDeviceDeinit()
	device_auth.DestroyDeviceAuthService()
	service.DiscCoapDeinit()
	bc := bus_center.GetInstance()
	bc.Stop()
}

// busCenterServerInit 初始化Bus Center
func busCenterServerInit() error {
	bc := bus_center.GetInstance()
	if err := bc.Start(); err != nil {
		return fmt.Errorf("Bus Center启动失败: %v", err)
	}

	// 从Discovery获取本地设备信息并设置到Bus Center
	localDevInfo := service.DiscCoapGetDeviceInfo()
	bc.SetLocalDeviceInfo(&bus_center.LocalDeviceInfo{
		UDID:       localDevInfo.DeviceId,
		UUID:       localDevInfo.DeviceId,
		DeviceID:   localDevInfo.DeviceId,
		DeviceName: localDevInfo.Name,
		DeviceType: "PC",
	})

	logger.Info("[Frame] Bus Center已初始化")
	return nil
}

// discServerInit 初始化Discovery服务
func discServerInit() error {
	if err := service.InitService(); err != nil {
		return fmt.Errorf("Discovery服务初始化失败: %v", err)
	}
	logger.Info("[Frame] Discovery服务已初始化")
	return nil
}

// authInit 初始化Authentication服务
func authInit() error {
	// 初始化DeviceAuth服务
	if err := device_auth.InitDeviceAuthService(); err != nil {
		return fmt.Errorf("DeviceAuth服务初始化失败: %v", err)
	}
	logger.Info("[Frame] DeviceAuth服务已初始化")

	// 初始化AuthDevice（认证管理器）
	// 创建默认回调，转发认证事件到Bus Center
	bc := bus_center.GetInstance()
	authCallback := &authentication.AuthConnCallback{
		OnConnOpened: func(requestId uint32, authId int64) {
			logger.Infof("[Frame] 认证连接已建立: requestId=%d, authId=%d", requestId, authId)
			// 构建基本的NodeInfo（详细信息由JoinLNN填充）
			node := &bus_center.NodeInfo{
				AuthSeq: authId,
				Status:  bus_center.StatusOnline,
			}
			bc.NotifyAuthSuccess(requestId, authId, node)
		},
		OnConnOpenFailed: func(requestId uint32, reason int32) {
			logger.Warnf("[Frame] 认证连接失败: requestId=%d, reason=%d", requestId, reason)
			bc.NotifyAuthFailed(requestId, reason)
		},
	}
	if err := authentication.AuthDeviceInit(authCallback); err != nil {
		return fmt.Errorf("AuthDevice初始化失败: %v", err)
	}
	logger.Info("[Frame] AuthDevice已初始化")

	// 启动认证TCP监听
	authPort, err := authentication.StartSocketListening(authentication.Auth, "0.0.0.0", 0)
	if err != nil {
		return fmt.Errorf("启动认证TCP监听失败: %v", err)
	}

	// 更新认证端口到Bus Center和Discovery
	bc.UpdateAuthPort(authPort)
	service.UpdateAuthPortToCoapService(authPort)
	logger.Infof("[Frame] 认证TCP监听已启动，端口: %d", authPort)

	return nil
}

// transServerInit 初始化Transmission服务
func transServerInit() error {
	if err := transmission.TransServerInit(); err != nil {
		return fmt.Errorf("Transmission服务初始化失败: %v", err)
	}
	logger.Info("[Frame] Transmission服务已初始化")
	return nil
}
