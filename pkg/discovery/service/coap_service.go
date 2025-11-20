package service

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

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
		DeviceType:       DEVICE_DEFAULT_TYPE,         // 设备类型
		Version:          DEVICE_DEFAULT_VERSION,      // 设备版本
		ServiceData:      DEVICE_DEFAULT_SERVICE_DATA, // 设备服务数据
		CapabilityBitmap: []uint16{1},                 // 设备能力位图
	}
	g_deviceInfo_lock        sync.RWMutex
	g_net_mgr                *network.Manager
	g_discover_callback      func(dev *coap.DeviceInfo)
	g_discover_callback_lock sync.RWMutex
)

func DiscCoapInit() error {
	var err error
	g_net_mgr, err = network.NewManager()
	if err != nil {
		log.Fatalf("网络管理器初始化失败: %v", err)
	}
	err = g_net_mgr.Start()
	if err != nil {
		log.Fatalf("启动网络监控失败: %v", err)
		return err
	}

	registerProviders()

	if coap.CoapInitDiscovery() != 0 {
		return errors.New("初始化发现监听服务失败")
	}
	log.Infof("CoAP discovery listener started on UDP port %d", coap.COAP_DEFAULT_PORT)
	return nil
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

func DiscCoapGetDeviceInfo() *LocalDeviceInfo {
	g_deviceInfo_lock.Lock()
	defer g_deviceInfo_lock.Unlock()
	return &g_deviceInfo
}

func DiscCoapRegistService(serviceData string, capabilityBitmap []uint16) {
	g_deviceInfo_lock.Lock()
	defer g_deviceInfo_lock.Unlock()
	g_deviceInfo.ServiceData = serviceData
	g_deviceInfo.CapabilityBitmap = capabilityBitmap
}

// SetDiscoverCallback 设置设备发现回调函数
// callback: 当发现新设备时调用的回调函数
func SetDiscoverCallback(callback func(dev *coap.DeviceInfo)) {
	g_discover_callback_lock.Lock()
	defer g_discover_callback_lock.Unlock()
	g_discover_callback = callback
}

// getDiscoverCallback 获取设备发现回调函数
func getDiscoverCallback() func(dev *coap.DeviceInfo) {
	g_discover_callback_lock.RLock()
	defer g_discover_callback_lock.RUnlock()
	return g_discover_callback
}

func UpdateAuthPortToCoapService(new_port int) {
	g_deviceInfo_lock.Lock()
	defer g_deviceInfo_lock.Unlock()
	serviceData := g_deviceInfo.ServiceData  // 获取设备的服务数据
	if !strings.Contains(serviceData, ":") { // 若服务数据中不包含冒号，则表示没有设置端口
		return
	}
	portIndex := strings.Index(serviceData, ":") + 1                                              // 获取冒号的下标
	port := serviceData[portIndex:]                                                               // 获取端口部分
	g_deviceInfo.ServiceData = strings.ReplaceAll(serviceData, port, fmt.Sprintf("%d", new_port)) // 将端口替换为新端口
}

func registerProviders() {
	deviceInfoProvider := func() *coap.DeviceInfo {
		g_deviceInfo_lock.RLock()
		defer g_deviceInfo_lock.RUnlock()
		return &coap.DeviceInfo{
			DeviceId:         g_deviceInfo.DeviceId,
			DeviceName:       g_deviceInfo.Name,
			DeviceType:       uint8(g_deviceInfo.DeviceType),
			Version:          g_deviceInfo.Version,
			Mode:             DEVICE_DEFAULT_DISCOVER_MODE,
			DeviceHash:       DEVICE_DEFAULT_HASH,
			ServiceData:      g_deviceInfo.ServiceData,
			CapabilityBitmap: g_deviceInfo.CapabilityBitmap,
		}
	}
	ipProvider := func() (string, error) {
		localIp, _, err := GetLocalNetworkInfo()
		if err != nil {
			return "", err
		}
		return localIp.String(), nil
	}
	discoverHandler := func(dev *coap.DeviceInfo) {
		// 提取端口信息
		port := ""
		if strings.Contains(dev.ServiceData, "port:") {
			parts := strings.Split(dev.ServiceData, "port:")
			if len(parts) > 1 {
				port = strings.TrimRight(strings.Split(parts[1], ",")[0], " ")
			}
		}

		// 精简输出
		if port != "" {
			log.Infof("[DISCOVERY] 发现设备: %s (%s:%s)", dev.DeviceName, dev.NetChannelInfo.Network.IP, port)
		} else {
			log.Infof("[DISCOVERY] 发现设备: %s (%s)", dev.DeviceName, dev.NetChannelInfo.Network.IP)
		}

		// 调用用户设置的回调
		callback := getDiscoverCallback()
		if callback != nil {
			callback(dev)
		}
	}

	coap.RegisterProviders(coap.Providers{
		LocalDeviceInfo: deviceInfoProvider,
		LocalIPString:   ipProvider,
		Discover:        discoverHandler,
	})
}

func GetLocalNetworkInfo() (net.IP, net.IPMask, error) {
	if g_net_mgr == nil {
		return net.IP{}, net.IPMask{}, errors.New("网络管理器未初始化")
	}

	// 优先使用指定接口，若不存在则用默认接口
	var ifaceInfo *network.InterfaceInfo
	var err error
	if g_deviceInfo.NetworkName != "" {
		ifaceInfo, err = g_net_mgr.GetInterface(g_deviceInfo.NetworkName)
		if err != nil {
			log.Errorf("获取指定网络接口失败: %v，尝试使用默认接口", err)
		}
	}
	if ifaceInfo == nil {
		ifaceInfo, err = g_net_mgr.GetDefaultInterface()
		if err != nil {
			return net.IP{}, net.IPMask{}, fmt.Errorf("获取默认接口失败: %w", err)
		}
	}

	// 遍历接口地址，找到第一个有效的 IPv4 地址和对应的掩码
	for i, addr := range ifaceInfo.Addresses {
		ipv4 := addr.To4()
		if ipv4 == nil {
			continue // 跳过 IPv6 地址
		}
		mask := ifaceInfo.Masks[i]
		if mask == nil || len(mask) != 4 {
			continue // 跳过 IPv6 掩码
		}
		// 验证掩码是否为有效的 IPv4 掩码（每个字节 0-255）
		validMask := true
		for _, b := range mask {
			if b < 0 || b > 0xff {
				validMask = false
				break
			}
		}
		if validMask {
			return ipv4, mask, nil
		}
	}

	return net.IP{}, net.IPMask{}, errors.New("未找到有效的 IPv4 地址和掩码")
}

// updateCoapService 更新CoAP服务，收集所有模块的能力和数据
func updateCoapService() error {
	g_deviceInfo_lock.Lock()
	defer g_deviceInfo_lock.Unlock()

	// 构建完整的serviceData: "port:<authPort>,<module1_data>,<module2_data>,..."
	var serviceDataBuilder strings.Builder

	// 1. 添加认证端口（始终在开头）
	if g_deviceInfo.ServiceData != "" && strings.HasPrefix(g_deviceInfo.ServiceData, "port:") {
		// 如果已有认证端口，使用它
		serviceDataBuilder.WriteString(g_deviceInfo.ServiceData)
	} else {
		// 否则使用默认值
		serviceDataBuilder.WriteString(DEVICE_DEFAULT_SERVICE_DATA)
	}

	// 2. 收集所有模块的能力位图和数据
	var capabilityBitmap uint16 = 0

	for i := 0; i < MAX_MODULE_COUNT; i++ {
		if g_publishModule[i].used == 0 {
			continue
		}

		// 合并能力位图
		capabilityBitmap |= g_publishModule[i].capabilityBitmap

		// 拼接能力数据
		if len(g_publishModule[i].capabilityData) > 0 {
			serviceDataBuilder.WriteString(",")
			serviceDataBuilder.Write(g_publishModule[i].capabilityData)
		}
	}

	// 3. 更新全局设备信息
	g_deviceInfo.ServiceData = serviceDataBuilder.String()
	if capabilityBitmap > 0 {
		g_deviceInfo.CapabilityBitmap = []uint16{capabilityBitmap}
	}

	log.Debugf("[DISCOVERY] 更新serviceData: %s, capabilityBitmap: %v",
		g_deviceInfo.ServiceData, g_deviceInfo.CapabilityBitmap)

	return nil
}
