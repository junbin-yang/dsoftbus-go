package authentication

import (
	"fmt"
	"sync"
	"testing"
)

// MockDeviceInfoProvider Mock设备信息提供者（用于测试）
type MockDeviceInfoProvider struct {
	deviceInfo *DeviceInfo
	udid       string
	uuid       string
	err        error
}

func (m *MockDeviceInfoProvider) GetDeviceInfo() (*DeviceInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.deviceInfo, nil
}

func (m *MockDeviceInfoProvider) GetUDID() (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.udid, nil
}

func (m *MockDeviceInfoProvider) GetUUID() (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.uuid, nil
}

// 测试默认提供者（应该返回错误）
func TestDefaultDeviceInfoProvider(t *testing.T) {
	// 确保使用默认提供者
	UnregisterDeviceInfoProvider()

	// 测试GetDeviceInfo
	info, err := GetLocalDeviceInfo()
	if err == nil {
		t.Error("Default provider should return error")
	}
	if info != nil {
		t.Error("Default provider should return nil device info")
	}
	t.Logf("GetLocalDeviceInfo error (expected): %v", err)

	// 测试GetUDID
	udid, err := GetLocalUDID()
	if err == nil {
		t.Error("Default provider should return error")
	}
	if udid != "" {
		t.Error("Default provider should return empty UDID")
	}
	t.Logf("GetLocalUDID error (expected): %v", err)

	// 测试GetUUID
	uuid, err := GetLocalUUID()
	if err == nil {
		t.Error("Default provider should return error")
	}
	if uuid != "" {
		t.Error("Default provider should return empty UUID")
	}
	t.Logf("GetLocalUUID error (expected): %v", err)

	// 测试IsDeviceInfoProviderRegistered
	if IsDeviceInfoProviderRegistered() {
		t.Error("Should return false for default provider")
	}

	t.Log("Default provider test passed")
}

// 测试注册和使用自定义提供者
func TestRegisterDeviceInfoProvider(t *testing.T) {
	// 创建Mock提供者
	mockProvider := &MockDeviceInfoProvider{
		deviceInfo: &DeviceInfo{
			UDID:       "test-udid-12345",
			UUID:       "test-uuid-67890",
			DeviceName: "TestDevice",
			DeviceType: "Phone",
			Version: SoftBusVersion{
				Major: 4,
				Minor: 1,
			},
			P2PMac: "00:11:22:33:44:55",
		},
		udid: "test-udid-12345",
		uuid: "test-uuid-67890",
		err:  nil,
	}

	// 测试注册nil提供者（应该失败）
	err := RegisterDeviceInfoProvider(nil)
	if err == nil {
		t.Fatal("Should fail when registering nil provider")
	}
	t.Logf("Register nil provider error (expected): %v", err)

	// 测试注册Mock提供者
	err = RegisterDeviceInfoProvider(mockProvider)
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// 测试IsDeviceInfoProviderRegistered
	if !IsDeviceInfoProviderRegistered() {
		t.Error("Should return true after registering provider")
	}

	// 测试GetLocalDeviceInfo
	info, err := GetLocalDeviceInfo()
	if err != nil {
		t.Fatalf("GetLocalDeviceInfo failed: %v", err)
	}
	if info == nil {
		t.Fatal("Device info should not be nil")
	}
	if info.UDID != mockProvider.deviceInfo.UDID {
		t.Errorf("UDID mismatch: expected %s, got %s", mockProvider.deviceInfo.UDID, info.UDID)
	}
	if info.UUID != mockProvider.deviceInfo.UUID {
		t.Errorf("UUID mismatch: expected %s, got %s", mockProvider.deviceInfo.UUID, info.UUID)
	}
	if info.DeviceName != mockProvider.deviceInfo.DeviceName {
		t.Errorf("DeviceName mismatch: expected %s, got %s", mockProvider.deviceInfo.DeviceName, info.DeviceName)
	}
	if info.DeviceType != mockProvider.deviceInfo.DeviceType {
		t.Errorf("DeviceType mismatch: expected %s, got %s", mockProvider.deviceInfo.DeviceType, info.DeviceType)
	}
	if info.Version.Major != mockProvider.deviceInfo.Version.Major {
		t.Errorf("Version.Major mismatch: expected %d, got %d", mockProvider.deviceInfo.Version.Major, info.Version.Major)
	}
	t.Logf("GetLocalDeviceInfo: UDID=%s, UUID=%s, Name=%s, Type=%s, Version=%d.%d",
		info.UDID, info.UUID, info.DeviceName, info.DeviceType, info.Version.Major, info.Version.Minor)

	// 测试GetLocalUDID
	udid, err := GetLocalUDID()
	if err != nil {
		t.Fatalf("GetLocalUDID failed: %v", err)
	}
	if udid != mockProvider.udid {
		t.Errorf("UDID mismatch: expected %s, got %s", mockProvider.udid, udid)
	}
	t.Logf("GetLocalUDID: %s", udid)

	// 测试GetLocalUUID
	uuid, err := GetLocalUUID()
	if err != nil {
		t.Fatalf("GetLocalUUID failed: %v", err)
	}
	if uuid != mockProvider.uuid {
		t.Errorf("UUID mismatch: expected %s, got %s", mockProvider.uuid, uuid)
	}
	t.Logf("GetLocalUUID: %s", uuid)

	t.Log("Register provider test passed")
}

// 测试注销提供者
func TestUnregisterDeviceInfoProvider(t *testing.T) {
	// 先注册一个提供者
	mockProvider := &MockDeviceInfoProvider{
		deviceInfo: &DeviceInfo{
			UDID: "test-udid",
			UUID: "test-uuid",
		},
		udid: "test-udid",
		uuid: "test-uuid",
	}

	err := RegisterDeviceInfoProvider(mockProvider)
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// 验证已注册
	if !IsDeviceInfoProviderRegistered() {
		t.Error("Provider should be registered")
	}

	// 注销提供者
	UnregisterDeviceInfoProvider()

	// 验证已注销（应该返回错误）
	if IsDeviceInfoProviderRegistered() {
		t.Error("Provider should be unregistered")
	}

	info, err := GetLocalDeviceInfo()
	if err == nil {
		t.Error("Should return error after unregistering")
	}
	if info != nil {
		t.Error("Should return nil after unregistering")
	}

	t.Log("Unregister provider test passed")
}

// 测试提供者返回错误的情况
func TestProviderError(t *testing.T) {
	// 创建返回错误的Mock提供者
	mockProvider := &MockDeviceInfoProvider{
		err: fmt.Errorf("mock error: failed to get device info"),
	}

	err := RegisterDeviceInfoProvider(mockProvider)
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// 测试GetLocalDeviceInfo返回错误
	info, err := GetLocalDeviceInfo()
	if err == nil {
		t.Error("Should return error from provider")
	}
	if info != nil {
		t.Error("Should return nil when error occurs")
	}
	t.Logf("GetLocalDeviceInfo error (expected): %v", err)

	// 测试GetLocalUDID返回错误
	udid, err := GetLocalUDID()
	if err == nil {
		t.Error("Should return error from provider")
	}
	if udid != "" {
		t.Error("Should return empty UDID when error occurs")
	}
	t.Logf("GetLocalUDID error (expected): %v", err)

	// 测试GetLocalUUID返回错误
	uuid, err := GetLocalUUID()
	if err == nil {
		t.Error("Should return error from provider")
	}
	if uuid != "" {
		t.Error("Should return empty UUID when error occurs")
	}
	t.Logf("GetLocalUUID error (expected): %v", err)

	t.Log("Provider error test passed")
}

// 测试并发安全
func TestDeviceInfoConcurrency(t *testing.T) {
	// 注册一个提供者
	mockProvider := &MockDeviceInfoProvider{
		deviceInfo: &DeviceInfo{
			UDID: "concurrent-test-udid",
			UUID: "concurrent-test-uuid",
		},
		udid: "concurrent-test-udid",
		uuid: "concurrent-test-uuid",
	}

	err := RegisterDeviceInfoProvider(mockProvider)
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// 并发读取
	var wg sync.WaitGroup
	concurrency := 100
	errors := make(chan error, concurrency*3)

	for i := 0; i < concurrency; i++ {
		wg.Add(3)

		// 并发GetLocalDeviceInfo
		go func() {
			defer wg.Done()
			info, err := GetLocalDeviceInfo()
			if err != nil {
				errors <- fmt.Errorf("GetLocalDeviceInfo failed: %v", err)
				return
			}
			if info.UDID != mockProvider.udid {
				errors <- fmt.Errorf("UDID mismatch")
			}
		}()

		// 并发GetLocalUDID
		go func() {
			defer wg.Done()
			udid, err := GetLocalUDID()
			if err != nil {
				errors <- fmt.Errorf("GetLocalUDID failed: %v", err)
				return
			}
			if udid != mockProvider.udid {
				errors <- fmt.Errorf("UDID mismatch")
			}
		}()

		// 并发GetLocalUUID
		go func() {
			defer wg.Done()
			uuid, err := GetLocalUUID()
			if err != nil {
				errors <- fmt.Errorf("GetLocalUUID failed: %v", err)
				return
			}
			if uuid != mockProvider.uuid {
				errors <- fmt.Errorf("UUID mismatch")
			}
		}()
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Errorf("Concurrency test error: %v", err)
	}

	t.Logf("Concurrency test completed: %d concurrent operations", concurrency*3)
	t.Log("Concurrency test passed")
}

// 测试提供者覆盖（重新注册）
func TestProviderOverride(t *testing.T) {
	// 注册第一个提供者
	provider1 := &MockDeviceInfoProvider{
		deviceInfo: &DeviceInfo{UDID: "provider1-udid"},
		udid:       "provider1-udid",
		uuid:       "provider1-uuid",
	}
	err := RegisterDeviceInfoProvider(provider1)
	if err != nil {
		t.Fatalf("Failed to register provider1: %v", err)
	}

	udid, err := GetLocalUDID()
	if err != nil {
		t.Fatalf("GetLocalUDID failed: %v", err)
	}
	if udid != "provider1-udid" {
		t.Errorf("Expected provider1-udid, got %s", udid)
	}
	t.Logf("Provider1 UDID: %s", udid)

	// 注册第二个提供者（覆盖第一个）
	provider2 := &MockDeviceInfoProvider{
		deviceInfo: &DeviceInfo{UDID: "provider2-udid"},
		udid:       "provider2-udid",
		uuid:       "provider2-uuid",
	}
	err = RegisterDeviceInfoProvider(provider2)
	if err != nil {
		t.Fatalf("Failed to register provider2: %v", err)
	}

	udid, err = GetLocalUDID()
	if err != nil {
		t.Fatalf("GetLocalUDID failed: %v", err)
	}
	if udid != "provider2-udid" {
		t.Errorf("Expected provider2-udid, got %s", udid)
	}
	t.Logf("Provider2 UDID: %s", udid)

	t.Log("Provider override test passed")
}
