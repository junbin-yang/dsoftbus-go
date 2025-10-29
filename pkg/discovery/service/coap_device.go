package service

// DeviceType 设备类型
type DeviceType uint8

// 设备类型常量定义
const (
	DeviceTypeUnknown DeviceType = 0x00 // 未知设备
	DeviceTypePhone   DeviceType = 0x0E // 智能手机
	DeviceTypePad     DeviceType = 0x11 // 平板
	DeviceTypeTV      DeviceType = 0x9C // 智能电视
	DeviceTypePC      DeviceType = 0x0C // 电脑
	DeviceTypeAudio   DeviceType = 0x0A // 音频设备
	DeviceTypeCar     DeviceType = 0x83 // 车载设备
	DeviceTypeL0      DeviceType = 0xF1 // 小型设备L0
	DeviceTypeL1      DeviceType = 0xF2 // 小型设备L1
)

// DeviceMap 设备类型名称与枚举值的映射
type DeviceMap struct {
	Value   string     // 设备类型名称
	DevType DeviceType // 对应的设备类型枚举值
}

// g_devMap 设备类型映射表
var g_devMap = []DeviceMap{
	{Value: "PHONE", DevType: DeviceTypePhone},
	{Value: "PAD", DevType: DeviceTypePad},
	{Value: "TV", DevType: DeviceTypeTV},
	{Value: "PC", DevType: DeviceTypePC},
	{Value: "AUDIO", DevType: DeviceTypeAudio},
	{Value: "CAR", DevType: DeviceTypeCar},
	{Value: "L0", DevType: DeviceTypeL0},
	{Value: "L1", DevType: DeviceTypeL1},
}

// 根据设备名称获取枚举值
func GetDeviceTypeByName(name string) (DeviceType, bool) {
	for _, m := range g_devMap {
		if m.Value == name {
			return m.DevType, true
		}
	}
	return DeviceTypeUnknown, false
}

// 根据枚举值获取设备名称
func GetDeviceNameByType(devType DeviceType) (string, bool) {
	for _, m := range g_devMap {
		if m.DevType == devType {
			return m.Value, true
		}
	}
	return "", false
}

type LocalDeviceInfo struct {
	Name             string
	DeviceId         string
	NetworkName      string
	DeviceType       DeviceType
	Version          string
	ServiceData      string
	CapabilityBitmap []uint16
}
