package hichain

import (
	"sync"
)

// DeviceAuthInfo 设备认证信息（内存缓存）
type DeviceAuthInfo struct {
	DeviceID       string // 设备ID
	PublicKey      []byte // ED25519公钥（32字节）
	PrivateKey     []byte // 本地ED25519私钥（64字节，仅保存自己的）
	SessionKey     []byte // 最后一次会话密钥（可选，用于快速重连）
	LastAuthTime   int64  // 最后认证时间戳
}

var (
	// 内存中的设备认证信息缓存
	deviceAuthStore = make(map[string]*DeviceAuthInfo)
	authStoreMu     sync.RWMutex
)

// SaveDeviceAuthInfo 保存设备认证信息到内存
func SaveDeviceAuthInfo(deviceID string, publicKey []byte) {
	authStoreMu.Lock()
	defer authStoreMu.Unlock()

	if info, exists := deviceAuthStore[deviceID]; exists {
		info.PublicKey = publicKey
	} else {
		deviceAuthStore[deviceID] = &DeviceAuthInfo{
			DeviceID:  deviceID,
			PublicKey: publicKey,
		}
	}
}

// GetDeviceAuthInfo 获取设备认证信息
func GetDeviceAuthInfo(deviceID string) *DeviceAuthInfo {
	authStoreMu.RLock()
	defer authStoreMu.RUnlock()
	return deviceAuthStore[deviceID]
}

// SaveLocalPrivateKey 保存本地长期私钥
func SaveLocalPrivateKey(deviceID string, privateKey, publicKey []byte) {
	authStoreMu.Lock()
	defer authStoreMu.Unlock()

	if info, exists := deviceAuthStore[deviceID]; exists {
		info.PrivateKey = privateKey
		info.PublicKey = publicKey
	} else {
		deviceAuthStore[deviceID] = &DeviceAuthInfo{
			DeviceID:   deviceID,
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		}
	}
}

// GetLocalPrivateKey 获取本地长期私钥（如果存在）
func GetLocalPrivateKey(deviceID string) ([]byte, []byte) {
	authStoreMu.RLock()
	defer authStoreMu.RUnlock()

	if info, exists := deviceAuthStore[deviceID]; exists {
		return info.PrivateKey, info.PublicKey
	}
	return nil, nil
}

// ClearDeviceAuthInfo 清除设备认证信息
func ClearDeviceAuthInfo(deviceID string) {
	authStoreMu.Lock()
	defer authStoreMu.Unlock()
	delete(deviceAuthStore, deviceID)
}
