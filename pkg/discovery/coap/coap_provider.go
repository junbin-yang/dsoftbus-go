package coap

type Providers struct {
	LocalDeviceInfo func() *DeviceInfo
	LocalIPString   func() (string, error)
	Discover        func(dev *DeviceInfo)
}

// 可由上层注册本地设备信息、本地IP和设备发现回调的提供者
var (
	localDeviceInfoProvider  func() *DeviceInfo
	localIPStringProvider    func() (string, error)
	discoverCallbackProvider func(dev *DeviceInfo)
)

func RegisterProviders(p Providers) {
	localDeviceInfoProvider = p.LocalDeviceInfo
	localIPStringProvider = p.LocalIPString
	discoverCallbackProvider = p.Discover
}
