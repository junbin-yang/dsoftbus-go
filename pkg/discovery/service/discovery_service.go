package service

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
	DataLen        uint           // 能力数据的最大长度（2字节）
}

// PublishFailReason 服务发布失败原因
type PublishFailReason int

// 失败原因常量
const (
	PublishFailReasonNotSupportMedium PublishFailReason = 1    // 不支持的介质
	PublishFailReasonParameterInvalid PublishFailReason = 2    // 参数无效
	PublishFailReasonUnknown          PublishFailReason = 0xFF // 未知原因
)

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

// IPublishCallback 服务发布成功/失败的回调接口
type IPublishCallback struct {
	OnPublishSuccess func(publishId int)                           // 发布成功回调
	OnPublishFail    func(publishId int, reason PublishFailReason) // 发布失败回调
}

// CommonDeviceKey 枚举设备信息（如标识、类型、名称等）
type CommonDeviceKey int

const (
	CommonDeviceKeyDevID   CommonDeviceKey = 0 // 设备ID，最大值为64个字符
	CommonDeviceKeyDevType CommonDeviceKey = 1 // 设备类型，目前仅支持"ddmpCapability"
	CommonDeviceKeyDevName CommonDeviceKey = 2 // 设备名称，最大值为63个字符
	CommonDeviceKeyMax     CommonDeviceKey = 3 // 保留
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
	g_capabilityData []byte
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
// moduleName：上层服务的模块名指针，最大值为63字节
// info：要发布的服务指针（参考PublishInfo）
// cb：服务发布的回调指针（参考IPublishCallback）
// 返回值：成功返回0，失败返回-1
func PublishService(moduleName string, info *PublishInfo, cb *IPublishCallback) int {
	// 参数合法性检查
	if moduleName == "" || len(moduleName) > MAX_PACKAGE_NAME || info == nil || cb == nil {
		return -1
	}
	if info.PublishId <= 0 || info.Capability == "" {
		return -1
	}
	if info.DataLen > MAX_SERVICE_DATA_LEN {
		if cb.OnPublishFail != nil {
			cb.OnPublishFail(info.PublishId, PublishFailReasonParameterInvalid)
		}
		return -1
	}

	// 初始化服务
	if initService() != 0 {
		if cb.OnPublishFail != nil {
			cb.OnPublishFail(info.PublishId, PublishFailReasonUnknown)
		}
		return -1
	}

	g_discoveryMutex.Lock()
	defer g_discoveryMutex.Unlock()

	// 检查是否已存在相同发布ID
	if findExistModule(moduleName, info.PublishId) != nil {
		if cb.OnPublishFail != nil {
			cb.OnPublishFail(info.PublishId, PublishFailReasonParameterInvalid)
		}
		return -1
	}

	// 查找空闲模块
	freeModule := findFreeModule()
	if freeModule == nil {
		if cb.OnPublishFail != nil {
			cb.OnPublishFail(info.PublishId, PublishFailReasonUnknown)
		}
		return -1
	}

	// 解析能力bitmap
	bitmap, err := parseCapability(info.Capability)
	if err != 0 {
		if cb.OnPublishFail != nil {
			cb.OnPublishFail(info.PublishId, PublishFailReasonParameterInvalid)
		}
		return -1
	}

	// 复制能力数据
	capData := make([]byte, info.DataLen)
	copy(capData, info.CapabilityData)

	// 填充模块信息
	freeModule.used = 1
	freeModule.packageName = moduleName
	freeModule.publishId = info.PublishId
	freeModule.medium = uint16(info.Medium)
	freeModule.capabilityBitmap = bitmap
	freeModule.capabilityData = capData
	freeModule.dataLength = uint16(info.DataLen)

	// 注册CoAP服务
	capabilityBitmaps := []uint{uint(bitmap)}
	if DiscCoapRegistService(capabilityBitmaps, len(capabilityBitmaps), string(capData)) != 0 {
		freeModule.used = 0
		if cb.OnPublishFail != nil {
			cb.OnPublishFail(info.PublishId, PublishFailReasonUnknown)
		}
		return -1
	}

	// 发布成功回调
	if cb.OnPublishSuccess != nil {
		cb.OnPublishSuccess(info.PublishId)
	}
	return 0
}

// UnPublishService 根据publishId和moduleName取消发布服务
// moduleName：上层服务的模块名指针，最大值为63字节
// publishId：要取消发布的服务ID，值必须大于0
// 返回值：成功返回0，失败返回非0值
func UnPublishService(moduleName string, publishId int) int {
	if moduleName == "" || len(moduleName) > MAX_PACKAGE_NAME || publishId <= 0 {
		return -1
	}

	g_discoveryMutex.Lock()
	defer g_discoveryMutex.Unlock()

	if g_isServiceInit == 0 {
		return -1
	}

	// 查找并释放模块
	module := findExistModule(moduleName, publishId)
	if module == nil {
		return -1
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
		g_capabilityData = nil
	}

	return 0
}

// SetCommonDeviceInfo 设置通用设备信息（如标识、类型、名称等）
// devInfo：设备信息数组指针
// num：设备信息数组中的元素数量，若与数组长度不一致，程序会崩溃
// 返回值：成功返回0，失败返回非0值
func SetCommonDeviceInfo(devInfo []CommonDeviceInfo, num uint) int {
	// 参数合法性检查
	if len(devInfo) == 0 || num == 0 || num > uint(COMM_DEVICE_KEY_MAX) {
		return -1
	}
	if uint(len(devInfo)) != num {
		return -1
	}

	// 获取设备信息并备份
	localDev := GetCommonDeviceInfo()
	if localDev == nil {
		return -1
	}

	// 备份原始信息
	backupDevId := localDev.deviceId
	backupDevName := localDev.deviceName
	backupDevType := localDev.deviceType

	var ret int
	for i := 0; i < int(num); i++ {
		item := devInfo[i]
		switch item.Key {
		case CommonDeviceKeyDevID:
			if len(item.Value) > MAX_DEV_ID_LEN {
				ret = -1
			} else {
				localDev.deviceId = item.Value
				ret = 0
			}
		case CommonDeviceKeyDevType:
			// 查找设备类型
			found := false
			for _, devMap := range g_devMap {
				if devMap.Value == item.Value {
					localDev.deviceType = int(devMap.DevType)
					found = true
					break
				}
			}
			if !found {
				ret = -1
			} else {
				ret = 0
			}
		case CommonDeviceKeyDevName:
			if len(item.Value) > MAX_DEV_NAME_LEN {
				ret = -1
			} else {
				localDev.deviceName = item.Value
				ret = 0
			}
		default:
			ret = -1
		}
		if ret != 0 {
			break
		}
	}

	// 更新设备信息
	if ret == 0 {
		ret = UpdateCommonDeviceInfo()
	}

	// 失败时恢复备份
	if ret != 0 {
		localDev.deviceId = backupDevId
		localDev.deviceName = backupDevName
		localDev.deviceType = backupDevType
	}

	return ret
}

// 初始化服务（内部辅助函数）
func initService() error {
	g_discoveryMutex.Lock()
	defer g_discoveryMutex.Unlock()

	if g_isServiceInit != 0 {
		return errors.New("service has been initialized")
	}

	// 初始化发布模块数组
	g_publishModule = make([]PublishModule, MAX_MODULE_COUNT)
	g_capabilityData = make([]byte, MAX_SERVICE_DATA_LEN)

	// 调用外部初始化函数
	if err := DiscCoapInit(); err != nil {
		return err
	}

	// todo 设置正确的设备信息！！！！！！
	DiscCoapRegisterDeviceInfo(LocalDeviceInfo{DeviceId: "12345678901234567890123456789012",
		Name: "testDevice", Type: "ddmpCapability"})

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
