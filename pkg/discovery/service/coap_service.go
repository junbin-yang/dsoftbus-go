package service

import (
	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/network"
)

const (
	DEVICE_DEFAULT_TYPE          DeviceType = DeviceTypeUnknown
	DEVICE_DEFAULT_IFC                      = "eth0"
	DEVICE_DEFAULT_NAME                     = "GoDevice"
	DEVICE_DEFAULT_VERSION                  = "1.0.0"
	DEVICE_DEFAULT_ID                       = "1234567890"
	DEVICE_DEFAULT_HASH                     = "0"
	DEVICE_DEFAULT_SERVICE_DATA             = "port:-1"
	DEVICE_DEFAULT_DISCOVER_MODE            = 0
)

var (
	g_deviceInfo = LocalDeviceInfo{
		Name:             DEVICE_DEFAULT_NAME,         // 设备名称
		DeviceId:         DEVICE_DEFAULT_ID,           // 设备ID
		NetworkName:      DEVICE_DEFAULT_IFC,          // 设备网络接口名
		Type:             DEVICE_DEFAULT_TYPE,         // 设备类型
		Version:          DEVICE_DEFAULT_VERSION,      // 设备版本
		ServiceData:      DEVICE_DEFAULT_SERVICE_DATA, // 设备服务数据
		CapabilityBitmap: []uint32{1},                 // 设备能力位图
	}
	g_deviceInfo_lock sync.RWMutex
	g_net_mgr         *network.Manager
)

func DiscCoapInit() error {
	var err error
	g_net_mgr, err = network.NewManager()
	if err != nil {
		log.Fatalf("网络管理器初始化失败: %v", err)
	}
	err = g_net_mgr.Start()
	if err != nil {
		return errors.New("启动网络监控失败: %v", err)
	}

	registerProviders()

	if coap.CoapInitDiscovery() != 0 {
		return errors.New("初始化发现监听服务失败")
	}
	log.Infof("CoAP discovery listener started on UDP port %d", coap.COAP_DEFAULT_PORT)
}

func DiscCoapDeinit() {
	defer g_net_mgr.Stop()
	coap.CoapDeinitDiscovery()
}

func DiscCoapRegisterDeviceInfo(dev LocalDeviceInfo) {
	g_deviceInfo_lock.Lock()
	defer g_deviceInfo_lock.Unlock()
	g_deviceInfo = dev
}

func DiscCoapRegistService(serviceData string, capabilityBitmap []uint32) {
	g_deviceInfo_lock.RLock()
	defer g_deviceInfo_lock.RUnlock()
	g_deviceInfo.ServiceData = serviceData
	g_deviceInfo.CapabilityBitmap = capabilityBitmap
}

func registerProviders() {
	deviceInfoProvider := func() *coap.DeviceInfo {
		g_deviceInfo_lock.RLock()
		defer g_deviceInfo_lock.RUnlock()
		return &coap.DeviceInfo{
			DeviceId:         g_deviceInfo.DeviceId,
			DeviceName:       g_deviceInfo.Name,
			DeviceType:       g_deviceInfo.Type,
			Version:          g_deviceInfo.Version,
			Mode:             DEVICE_DEFAULT_DISCOVER_MODE,
			DeviceHash:       DEVICE_DEFAULT_HASH,
			ServiceData:      g_deviceInfo.ServiceData,
			CapabilityBitmap: g_deviceInfo.CapabilityBitmap,
		}
	}
	ipProvider := func() (string, error) {
		localIp, _, err := getLocalNetworkInfo()
		if err != nil {
			return "", err
		}
		return localIp, nil
	}
	discoverHandler := func(dev *coap.DeviceInfo) {
		fmt.Printf("发现新设备：%+v\n", dev)
	}

	coap.RegisterProviders(coap.Providers{
		LocalDeviceInfo: deviceInfoProvider,
		LocalIPString:   ipProvider,
		Discover:        discoverHandler,
	})
}

func getLocalNetworkInfo() (net.IP, net.IPMask, error) {
	if g_net_mgr == nil {
		return "", errors.New("网络管理器未初始化")
	}
	defaultInterface, _ := g_net_mgr.GetDefaultInterface()
	localIp := defaultInterface.Addresses[0]
	localIpMask := defaultInterface.Masks[0]
	ifaceInfo, err := g_net_mgr.GetInterface(g_deviceInfo.NetworkName)
	if err != nil {
		log.Errorf("获取网络接口失败: %v", err)
	}
	if len(ifaceInfo.Addresses) > 0 {
		localIp = ifaceInfo.Addresses[0]
		localIpMask = ifaceInfo.Masks[0]
	}
	return localIp, localIpMask, nil
}
