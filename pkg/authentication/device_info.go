package authentication

import (
	"fmt"
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// DeviceInfo 设备信息
// 对应C的设备信息字段，从LNN模块获取
type DeviceInfo struct {
	UDID       string         // 设备唯一标识 (Device Unique ID)
	UUID       string         // 通用唯一标识 (Universal Unique ID)
	DeviceName string         // 设备名称
	DeviceType string         // 设备类型
	Version    SoftBusVersion // 软总线版本
	P2PMac     string         // P2P MAC地址（预留，暂不使用）
}

// DeviceInfoProvider 设备信息提供者接口
// 外部实现此接口并注册，以提供设备信息
type DeviceInfoProvider interface {
	// GetDeviceInfo 获取完整设备信息
	GetDeviceInfo() (*DeviceInfo, error)

	// GetUDID 获取设备唯一标识
	GetUDID() (string, error)

	// GetUUID 获取通用唯一标识
	GetUUID() (string, error)
}

// DefaultDeviceInfoProvider 默认设备信息提供者
// 未注册外部提供者时使用，返回错误提示需要注入
type DefaultDeviceInfoProvider struct{}

func (d *DefaultDeviceInfoProvider) GetDeviceInfo() (*DeviceInfo, error) {
	return nil, fmt.Errorf("device info provider not registered, please call RegisterDeviceInfoProvider")
}

func (d *DefaultDeviceInfoProvider) GetUDID() (string, error) {
	return "", fmt.Errorf("device info provider not registered, please call RegisterDeviceInfoProvider")
}

func (d *DefaultDeviceInfoProvider) GetUUID() (string, error) {
	return "", fmt.Errorf("device info provider not registered, please call RegisterDeviceInfoProvider")
}

// 全局设备信息提供者
var (
	g_deviceInfoProvider   DeviceInfoProvider = &DefaultDeviceInfoProvider{}
	g_deviceInfoProviderMu sync.RWMutex
)

// RegisterDeviceInfoProvider 注册设备信息提供者
// 外部模块通过此函数注入设备信息提供者
//
// 参数:
//   - provider: 设备信息提供者实现
//
// 返回:
//   - error: 错误信息（provider为nil时返回错误）
//
// 示例:
//
//	type MyDeviceInfoProvider struct {
//	    udid string
//	    uuid string
//	}
//
//	func (p *MyDeviceInfoProvider) GetDeviceInfo() (*DeviceInfo, error) {
//	    return &DeviceInfo{
//	        UDID:       p.udid,
//	        UUID:       p.uuid,
//	        DeviceName: "MyDevice",
//	        DeviceType: "Phone",
//	        Version:    SoftBusVersion{Major: 1, Minor: 0},
//	    }, nil
//	}
//
//	provider := &MyDeviceInfoProvider{udid: "device-123", uuid: "uuid-456"}
//	RegisterDeviceInfoProvider(provider)
func RegisterDeviceInfoProvider(provider DeviceInfoProvider) error {
	if provider == nil {
		return fmt.Errorf("device info provider cannot be nil")
	}

	g_deviceInfoProviderMu.Lock()
	defer g_deviceInfoProviderMu.Unlock()

	g_deviceInfoProvider = provider
	log.Infof("[DEVICE_INFO] Device info provider registered successfully")

	return nil
}

// UnregisterDeviceInfoProvider 注销设备信息提供者
// 恢复为默认提供者（返回错误）
func UnregisterDeviceInfoProvider() {
	g_deviceInfoProviderMu.Lock()
	defer g_deviceInfoProviderMu.Unlock()

	g_deviceInfoProvider = &DefaultDeviceInfoProvider{}
	log.Infof("[DEVICE_INFO] Device info provider unregistered")
}

// GetLocalDeviceInfo 获取本地设备信息
// 对应C代码中的LnnGetLocalStrInfo等函数
//
// 返回:
//   - *DeviceInfo: 设备信息
//   - error: 错误信息
//
// C代码参考:
//
//	LnnGetLocalStrInfo(STRING_KEY_DEV_UDID, localUdid, UDID_BUF_LEN);
//	LnnGetLocalStrInfo(STRING_KEY_NETWORKID, networkId, sizeof(networkId));
func GetLocalDeviceInfo() (*DeviceInfo, error) {
	g_deviceInfoProviderMu.RLock()
	provider := g_deviceInfoProvider
	g_deviceInfoProviderMu.RUnlock()

	return provider.GetDeviceInfo()
}

// GetLocalUDID 获取本地设备UDID
// 对应C代码中的LnnGetLocalStrInfo(STRING_KEY_DEV_UDID, ...)
//
// 返回:
//   - string: 设备UDID
//   - error: 错误信息
func GetLocalUDID() (string, error) {
	g_deviceInfoProviderMu.RLock()
	provider := g_deviceInfoProvider
	g_deviceInfoProviderMu.RUnlock()

	return provider.GetUDID()
}

// GetLocalUUID 获取本地设备UUID
// 对应C代码中的LnnGetLocalStrInfo(STRING_KEY_UUID, ...)
//
// 返回:
//   - string: 设备UUID
//   - error: 错误信息
func GetLocalUUID() (string, error) {
	g_deviceInfoProviderMu.RLock()
	provider := g_deviceInfoProvider
	g_deviceInfoProviderMu.RUnlock()

	return provider.GetUUID()
}

// IsDeviceInfoProviderRegistered 检查是否已注册外部设备信息提供者
// 返回:
//   - bool: true表示已注册外部提供者，false表示使用默认提供者
func IsDeviceInfoProviderRegistered() bool {
	g_deviceInfoProviderMu.RLock()
	defer g_deviceInfoProviderMu.RUnlock()

	_, isDefault := g_deviceInfoProvider.(*DefaultDeviceInfoProvider)
	return !isDefault
}
