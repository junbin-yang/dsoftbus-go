package service

import (
	"errors"
	"sync"

	"github.com/junbin-yang/dsoftbus-go/pkg/utils/config"
)

// ExchangeMedium 用于发布服务的介质（如蓝牙、Wi-Fi、USB等）
type ExchangeMedium int

// 介质类型常量（目前只支持CoAP协议）
const (
	ExchangeMediumAuto ExchangeMedium = 0 // 自动选择介质
	ExchangeMediumBLE  ExchangeMedium = 1 // 蓝牙
	ExchangeMediumCOAP ExchangeMedium = 2 // Wi-Fi（CoAP协议）
	ExchangeMediumUSB  ExchangeMedium = 3 // USB
)

// ExchangeFreq 服务发布频率（仅用于蓝牙，目前未支持）
type ExchangeFreq int

// 频率类型常量
const (
	ExchangeFreqLow       ExchangeFreq = 0 // 低频率
	ExchangeFreqMid       ExchangeFreq = 1 // 中频率
	ExchangeFreqHigh      ExchangeFreq = 2 // 高频率
	ExchangeFreqSuperHigh ExchangeFreq = 3 // 超高频率
)

// DiscoverMode 服务发布模式
type DiscoverMode int

// 发布模式常量
const (
	DiscoverModePassive DiscoverMode = 0x55 // 被动模式
	DiscoverModeActive  DiscoverMode = 0xAA // 主动模式
)

// PublishInfo 定义发送给发现设备的服务提供信息
type PublishInfo struct {
	PublishId      int            // 服务发布ID
	Mode           DiscoverMode   // 服务发布模式
	Medium         ExchangeMedium // 服务发布介质
	Freq           ExchangeFreq   // 服务发布频率
	Capability     string         // 服务发布能力（参考g_capabilityMap）
	CapabilityData []byte         // 服务发布的能力数据
}

// DataBitMap 枚举设备发布的支持能力
type DataBitMap int

const (
	DataBitMapHICALL        DataBitMap = iota // MeeTime
	DataBitMapPROFILE                         // 智能域中的视频反向连接
	DataBitMapHOMEVISIONPIC                   // Vision中的图库
	DataBitMapCASTPLUS                        // cast+
	DataBitMapAA                              // Vision中的输入法
	DataBitMapDVKIT                           // 设备虚拟化工具包
	DataBitMapDDMP                            // 分布式中间件
)

// CapabilityMap 定义支持的能力与位图之间的映射
type CapabilityMap struct {
	Bitmap     DataBitMap // 位图（参考DataBitMap）
	Capability string     // 能力（参考g_capabilityMap）
}

// g_capabilityMap 能力与位图的映射表
var g_capabilityMap = []CapabilityMap{
	{DataBitMapHICALL, "hicall"},
	{DataBitMapPROFILE, "profile"},
	{DataBitMapCASTPLUS, "castPlus"},
	{DataBitMapHOMEVISIONPIC, "homevisionPic"},
	{DataBitMapAA, "aaCapability"},
	{DataBitMapDVKIT, "dvKit"},
	{DataBitMapDDMP, "ddmpCapability"},
}

// CommonDeviceKey 枚举设备信息（如标识、类型、名称等）
type CommonDeviceKey int

const (
	CommonDeviceKeyDevID   CommonDeviceKey = 0 // 设备ID，最大值为64个字符
	CommonDeviceKeyDevType CommonDeviceKey = 1 // 设备类型，如"ddmpCapability"
	CommonDeviceKeyDevName CommonDeviceKey = 2 // 设备名称，最大值为63个字符
)

// CommonDeviceInfo 定义要设置的设备的类型和内容
type CommonDeviceInfo struct {
	Key   CommonDeviceKey // 设备信息类型（参考CommonDeviceKey）
	Value string          // 要设置的内容
}

/////////////////////////////////////////////////////////////////////

const (
	MAX_PACKAGE_NAME     = 64
	MAX_MODULE_COUNT     = 3
	MAX_SERVICE_DATA_LEN = 64
)

var (
	g_isServiceInit  int
	g_publishModule  []PublishModule
	g_discoveryMutex sync.Mutex
)

// PublishModule 用于存储发布服务的模块信息
type PublishModule struct {
	packageName      string
	publishId        int
	medium           uint16
	capabilityBitmap uint16
	capabilityData   []byte
	dataLength       uint16
	used             int
}

// PublishService 在局域网内向发现设备发布服务
// moduleName：上层服务的模块名
// info：要发布的服务指针（参考PublishInfo）
// 返回值：成功返回publishId
func PublishService(moduleName string, info *PublishInfo) (int, error) {
	// 参数合法性检查
	if moduleName == "" || len(moduleName) > MAX_PACKAGE_NAME || info == nil {
		return 0, errors.New("参数错误")
	}
	if info.PublishId <= 0 || info.Capability == "" {
		return 0, errors.New("参数错误")
	}
	if len(info.CapabilityData) > MAX_SERVICE_DATA_LEN {
		return 0, errors.New("参数错误")
	}

	g_discoveryMutex.Lock()
	defer g_discoveryMutex.Unlock()

	// 检查是否已存在相同发布ID
	if findExistModule(moduleName, info.PublishId) != nil {
		return info.PublishId, errors.New("重复发布的服务")
	}

	// 查找空闲模块
	freeModule := findFreeModule()
	if freeModule == nil {
		return 0, errors.New("无法发布更多服务")
	}

	// 解析能力bitmap
	bitmap, err := parseCapability(info.Capability)
	if err != 0 {
		return 0, errors.New("解析服务能力失败")
	}

	// 复制能力数据
	capData := make([]byte, len(info.CapabilityData))
	copy(capData, info.CapabilityData)

	// 填充模块信息
	freeModule.used = 1
	freeModule.packageName = moduleName
	freeModule.publishId = info.PublishId
	freeModule.medium = uint16(info.Medium)
	freeModule.capabilityBitmap = bitmap
	freeModule.capabilityData = capData
	freeModule.dataLength = uint16(len(info.CapabilityData))

	// 重新收集所有模块的能力和数据，并注册到CoAP
	if err := updateCoapService(); err != nil {
		return 0, err
	}

	return info.PublishId, nil
}

// UnPublishService 根据publishId和moduleName取消发布服务
// moduleName：上层服务的模块名
// publishId：要取消发布的服务ID
// 返回值：成功返回true
func UnPublishService(moduleName string, publishId int) (bool, error) {
	if moduleName == "" || len(moduleName) > MAX_PACKAGE_NAME || publishId <= 0 {
		return false, errors.New("参数错误")
	}

	g_discoveryMutex.Lock()
	defer g_discoveryMutex.Unlock()

	if g_isServiceInit == 0 {
		return false, errors.New("服务未发布")
	}

	// 查找并释放模块
	module := findExistModule(moduleName, publishId)
	if module == nil {
		return false, errors.New("服务未发布")
	}

	// 清理模块数据
	module.used = 0
	module.capabilityData = nil

	// 检查是否所有模块都已释放，如果是则反初始化服务
	allFree := true
	for i := 0; i < MAX_MODULE_COUNT; i++ {
		if g_publishModule[i].used == 1 {
			allFree = false
			break
		}
	}
	if allFree {
		DiscCoapDeinit()
		g_isServiceInit = 0
		g_publishModule = nil
	}

	return true, nil
}

// 初始化服务
func InitService() error {
	g_discoveryMutex.Lock()
	defer g_discoveryMutex.Unlock()

	if g_isServiceInit != 0 {
		return errors.New("服务已初始化")
	}

	// 初始化发布模块数组
	g_publishModule = make([]PublishModule, MAX_MODULE_COUNT)

	// 调用外部初始化函数
	if err := DiscCoapInit(); err != nil {
		return err
	}

	// 设备设备信息
	conf := config.Parse()
	if conf.DeviceName == "" || conf.UDID == "" || conf.Interface == "" || conf.DeviceType == "" {
		return errors.New("配置文件错误")
	}
	DiscCoapRegisterDeviceInfo(LocalDeviceInfo{
		Name:             conf.DeviceName,
		DeviceId:         conf.UDID,
		NetworkName:      conf.Interface,
		DeviceType:       GetDeviceTypeByName(conf.DeviceType),
		Version:          DEVICE_DEFAULT_VERSION,
		ServiceData:      DEVICE_DEFAULT_SERVICE_DATA,
		CapabilityBitmap: []uint16{},
	})

	g_isServiceInit = 1
	return nil
}

// 查找已存在的发布模块
func findExistModule(moduleName string, publishId int) *PublishModule {
	for i := 0; i < MAX_MODULE_COUNT; i++ {
		module := &g_publishModule[i]
		if module.used == 1 && module.packageName == moduleName && module.publishId == publishId {
			return module
		}
	}
	return nil
}

// 查找空闲的发布模块
func findFreeModule() *PublishModule {
	for i := 0; i < MAX_MODULE_COUNT; i++ {
		module := &g_publishModule[i]
		if module.used == 0 {
			return module
		}
	}
	return nil
}

// 解析能力字符串到bitmap
func parseCapability(capability string) (uint16, int) {
	for _, item := range g_capabilityMap {
		if item.Capability == capability {
			return uint16(item.Bitmap), 0
		}
	}
	return 0, -1
}

// SetCommonDeviceInfo 设置通用设备信息（如标识、类型、名称等）
// devInfo：设备信息数组
func SetCommonDeviceInfo(devInfo []CommonDeviceInfo) (bool, error) {
	num := len(devInfo)
	// 参数合法性检查
	if num == 0 {
		return false, errors.New("没有要修改的设备信息")
	}

	// 获取设备信息并备份
	localDev := DiscCoapGetDeviceInfo()
	if localDev == nil {
		return false, errors.New("未注册设备信息")
	}

	// 备份原始信息
	backupDevId := localDev.DeviceId
	backupDevName := localDev.Name
	backupDevType := localDev.DeviceType

	var ret bool
	for i := 0; i < num; i++ {
		item := devInfo[i]
		switch item.Key {
		case CommonDeviceKeyDevID:
			localDev.DeviceId = item.Value
			ret = true
		case CommonDeviceKeyDevType:
			// 查找设备类型
			ret = false
			for _, devMap := range g_devMap {
				if devMap.Value == item.Value {
					localDev.DeviceType = devMap.DevType
					ret = true
					break
				}
			}
		case CommonDeviceKeyDevName:
			localDev.Name = item.Value
			ret = true
		default:
			ret = false
		}
		if !ret {
			break
		}
	}

	// 失败时恢复备份
	if !ret {
		localDev.DeviceId = backupDevId
		localDev.Name = backupDevName
		localDev.DeviceType = backupDevType
	}

	return ret, nil
}
