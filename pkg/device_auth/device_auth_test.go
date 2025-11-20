package device_auth

import (
	"testing"
)

// TestInitDestroyDeviceAuthService 测试初始化和销毁服务
func TestInitDestroyDeviceAuthService(t *testing.T) {
	// 初始化服务
	err := InitDeviceAuthService()
	if err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	t.Log("Device auth service initialized")

	// 重复初始化（应该成功）
	err = InitDeviceAuthService()
	if err != nil {
		t.Errorf("Reinitialize failed: %v", err)
	}

	// 获取实例
	ga, err := GetGaInstance()
	if err != nil {
		t.Errorf("GetGaInstance failed: %v", err)
	}
	if ga == nil {
		t.Error("GroupAuthManager should not be nil")
	}

	gm, err := GetGmInstance()
	if err != nil {
		t.Errorf("GetGmInstance failed: %v", err)
	}
	if gm == nil {
		t.Error("DeviceGroupManager should not be nil")
	}

	// 销毁服务
	DestroyDeviceAuthService()
	t.Log("Device auth service destroyed")

	// 销毁后获取实例应该失败
	_, err = GetGaInstance()
	if err == nil {
		t.Error("Expected error after service destroyed")
	}
	t.Logf("GetGaInstance after destroy error (expected): %v", err)
}

// TestGroupAuthManager 测试GroupAuthManager接口
func TestGroupAuthManager(t *testing.T) {
	err := InitDeviceAuthService()
	if err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	ga, _ := GetGaInstance()

	// 测试ProcessData
	callback := &DeviceAuthCallback{
		OnFinish: func(requestId int64, operationCode int32, returnData string) {
			t.Logf("OnFinish called: requestId=%d, operationCode=%d", requestId, operationCode)
		},
		OnError: func(requestId int64, operationCode int32, errorCode int32, errorReturn string) {
			t.Logf("OnError called: requestId=%d, errorCode=%d", requestId, errorCode)
		},
	}

	// 测试ProcessData with invalid data (should fail - not JSON)
	err = ga.ProcessData(1001, []byte("test data"), callback)
	if err == nil {
		t.Error("Expected error with invalid JSON data")
	}
	t.Logf("ProcessData with invalid data failed as expected: %v", err)

	// 测试AuthDevice
	err = ga.AuthDevice(AnyOsAccount, 1002, "{}", callback)
	if err != nil {
		t.Errorf("AuthDevice failed: %v", err)
	}
	t.Log("AuthDevice succeeded")

	// 测试CancelRequest
	ga.CancelRequest(1003, AUTH_APPID)
	t.Log("CancelRequest succeeded")

	// 测试GetRealInfo（应该返回not implemented）
	_, err = ga.GetRealInfo(AnyOsAccount, "pseudo-123")
	if err == nil {
		t.Error("Expected not implemented error")
	}
	t.Logf("GetRealInfo error (expected): %v", err)
}

// TestDeviceGroupManager 测试DeviceGroupManager接口
func TestDeviceGroupManager(t *testing.T) {
	err := InitDeviceAuthService()
	if err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	// 测试RegCallback
	callback := &DeviceAuthCallback{
		OnTransmit: func(requestId int64, data []byte) bool {
			t.Logf("OnTransmit: requestId=%d, dataLen=%d", requestId, len(data))
			return true
		},
	}

	err = gm.RegCallback(AUTH_APPID, callback)
	if err != nil {
		t.Errorf("RegCallback failed: %v", err)
	}
	t.Log("RegCallback succeeded")

	// 测试RegDataChangeListener
	listener := &DataChangeListener{
		OnGroupCreated: func(groupInfo string) {
			t.Logf("OnGroupCreated: %s", groupInfo)
		},
		OnDeviceBound: func(peerUdid string, groupInfo string) {
			t.Logf("OnDeviceBound: %s", peerUdid)
		},
	}

	err = gm.RegDataChangeListener(AUTH_APPID, listener)
	if err != nil {
		t.Errorf("RegDataChangeListener failed: %v", err)
	}
	t.Log("RegDataChangeListener succeeded")

	// 测试GetJoinedGroups
	groups, err := gm.GetJoinedGroups(AnyOsAccount, AUTH_APPID, PeerToPeerGroup)
	if err != nil {
		t.Errorf("GetJoinedGroups failed: %v", err)
	}
	t.Logf("GetJoinedGroups returned %d groups", len(groups))

	// 测试GetTrustedDevices
	devices, err := gm.GetTrustedDevices(AnyOsAccount, AUTH_APPID, "test-group-id")
	if err != nil {
		t.Errorf("GetTrustedDevices failed: %v", err)
	}
	t.Logf("GetTrustedDevices returned %d devices", len(devices))

	// 测试IsDeviceInGroup
	exists := gm.IsDeviceInGroup(AnyOsAccount, AUTH_APPID, "test-group-id", "test-device-id")
	t.Logf("IsDeviceInGroup: %v", exists)

	// 测试UnRegCallback
	err = gm.UnRegCallback(AUTH_APPID)
	if err != nil {
		t.Errorf("UnRegCallback failed: %v", err)
	}
	t.Log("UnRegCallback succeeded")

	// 测试UnRegDataChangeListener
	err = gm.UnRegDataChangeListener(AUTH_APPID)
	if err != nil {
		t.Errorf("UnRegDataChangeListener failed: %v", err)
	}
	t.Log("UnRegDataChangeListener succeeded")
}

// TestHelperFunctions 测试辅助函数
func TestHelperFunctions(t *testing.T) {
	err := InitDeviceAuthService()
	if err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	// 测试ProcessCredential
	result, err := ProcessCredential(CredOpQuery, "{}")
	if err != nil {
		t.Errorf("ProcessCredential failed: %v", err)
	}
	t.Logf("ProcessCredential result: %s", result)

	// 测试StartAuthDevice
	callback := &DeviceAuthCallback{
		OnFinish: func(requestId int64, operationCode int32, returnData string) {
			t.Logf("OnFinish: requestId=%d", requestId)
		},
	}

	err = StartAuthDevice(2001, "{}", callback)
	if err != nil {
		t.Errorf("StartAuthDevice failed: %v", err)
	}
	t.Log("StartAuthDevice succeeded")

	// 测试CancelAuthRequest
	err = CancelAuthRequest(2002, "{}")
	if err != nil {
		t.Errorf("CancelAuthRequest failed: %v", err)
	}
	t.Log("CancelAuthRequest succeeded")
}

// TestGetInstanceBeforeInit 测试未初始化时获取实例
func TestGetInstanceBeforeInit(t *testing.T) {
	// 确保服务未初始化
	DestroyDeviceAuthService()

	_, err := GetGaInstance()
	if err == nil {
		t.Error("Expected error when service not initialized")
	}
	t.Logf("GetGaInstance before init error (expected): %v", err)

	_, err = GetGmInstance()
	if err == nil {
		t.Error("Expected error when service not initialized")
	}
	t.Logf("GetGmInstance before init error (expected): %v", err)
}

// ============================================================================
// DeviceGroupManager 完整功能测试
// ============================================================================

func TestCreateAndGetGroup(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	createParams := `{"groupId":"TEST_001","groupName":"TestGroup","groupType":256,"groupVisibility":0}`
	if err := gm.CreateGroup(AnyOsAccount, 1001, "test_app", createParams); err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	groupInfo, err := gm.GetGroupInfoById(AnyOsAccount, "test_app", "TEST_001")
	if err != nil {
		t.Fatalf("GetGroupInfoById failed: %v", err)
	}
	t.Logf("Group info: %s", groupInfo)
}

func TestDeleteGroup(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	createParams := `{"groupId":"TEST_002","groupName":"TestGroup2","groupType":256}`
	gm.CreateGroup(AnyOsAccount, 1002, "test_app", createParams)

	deleteParams := `{"groupId":"TEST_002"}`
	if err := gm.DeleteGroup(AnyOsAccount, 1003, "test_app", deleteParams); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	_, err := gm.GetGroupInfoById(AnyOsAccount, "test_app", "TEST_002")
	if err == nil {
		t.Error("Expected error for deleted group")
	}
}

func TestAddAndDeleteMember(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	createParams := `{"groupId":"TEST_003","groupName":"TestGroup3","groupType":256}`
	gm.CreateGroup(AnyOsAccount, 1004, "test_app", createParams)

	addParams := `{"groupId":"TEST_003","deviceId":"device001","udid":"udid001"}`
	if err := gm.AddMemberToGroup(AnyOsAccount, 1005, "test_app", addParams); err != nil {
		t.Fatalf("AddMemberToGroup failed: %v", err)
	}

	if !gm.IsDeviceInGroup(AnyOsAccount, "test_app", "TEST_003", "device001") {
		t.Error("Device should be in group")
	}

	deleteParams := `{"groupId":"TEST_003","deviceId":"device001"}`
	if err := gm.DeleteMemberFromGroup(AnyOsAccount, 1006, "test_app", deleteParams); err != nil {
		t.Fatalf("DeleteMemberFromGroup failed: %v", err)
	}

	if gm.IsDeviceInGroup(AnyOsAccount, "test_app", "TEST_003", "device001") {
		t.Error("Device should not be in group")
	}
}

func TestGetJoinedGroupsByType(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	for i := 0; i < 3; i++ {
		createParams := `{"groupId":"TEST_00` + string(rune('4'+i)) + `","groupName":"TestGroup","groupType":256}`
		gm.CreateGroup(AnyOsAccount, int64(1007+i), "test_app", createParams)
	}

	groups, err := gm.GetJoinedGroups(AnyOsAccount, "test_app", PeerToPeerGroup)
	if err != nil {
		t.Fatalf("GetJoinedGroups failed: %v", err)
	}

	if len(groups) < 3 {
		t.Errorf("Expected at least 3 groups, got %d", len(groups))
	}
}

func TestGetRelatedGroupsForDevice(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	createParams := `{"groupId":"TEST_007","groupName":"TestGroup7","groupType":256}`
	gm.CreateGroup(AnyOsAccount, 1010, "test_app", createParams)

	addParams := `{"groupId":"TEST_007","deviceId":"device002"}`
	gm.AddMemberToGroup(AnyOsAccount, 1011, "test_app", addParams)

	groups, err := gm.GetRelatedGroups(AnyOsAccount, "test_app", "device002")
	if err != nil {
		t.Fatalf("GetRelatedGroups failed: %v", err)
	}

	if len(groups) == 0 {
		t.Error("Expected at least 1 related group")
	}
}

func TestGetTrustedDevicesInGroup(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	createParams := `{"groupId":"TEST_008","groupName":"TestGroup8","groupType":256}`
	gm.CreateGroup(AnyOsAccount, 1012, "test_app", createParams)

	for i := 0; i < 3; i++ {
		addParams := `{"groupId":"TEST_008","deviceId":"device00` + string(rune('3'+i)) + `"}`
		gm.AddMemberToGroup(AnyOsAccount, int64(1013+i), "test_app", addParams)
	}

	devices, err := gm.GetTrustedDevices(AnyOsAccount, "test_app", "TEST_008")
	if err != nil {
		t.Fatalf("GetTrustedDevices failed: %v", err)
	}

	if len(devices) != 3 {
		t.Errorf("Expected 3 devices, got %d", len(devices))
	}
}

func TestGetDeviceInfo(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	createParams := `{"groupId":"TEST_009","groupName":"TestGroup9","groupType":256}`
	gm.CreateGroup(AnyOsAccount, 1016, "test_app", createParams)

	addParams := `{"groupId":"TEST_009","deviceId":"device006","udid":"udid006","authId":"auth006"}`
	gm.AddMemberToGroup(AnyOsAccount, 1017, "test_app", addParams)

	deviceInfo, err := gm.GetDeviceInfoById(AnyOsAccount, "test_app", "device006", "TEST_009")
	if err != nil {
		t.Fatalf("GetDeviceInfoById failed: %v", err)
	}
	t.Logf("Device info: %s", deviceInfo)
}

func TestDataChangeCallbacks(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	gm, _ := GetGmInstance()

	groupCreated := false
	groupDeleted := false
	deviceBound := false
	deviceUnbound := false

	listener := &DataChangeListener{
		OnGroupCreated: func(groupInfo string) {
			groupCreated = true
		},
		OnGroupDeleted: func(groupInfo string) {
			groupDeleted = true
		},
		OnDeviceBound: func(peerUdid string, groupInfo string) {
			deviceBound = true
		},
		OnDeviceUnBound: func(peerUdid string, groupInfo string) {
			deviceUnbound = true
		},
	}

	gm.RegDataChangeListener("test_app", listener)

	createParams := `{"groupId":"TEST_010","groupName":"TestGroup10","groupType":256}`
	gm.CreateGroup(AnyOsAccount, 1018, "test_app", createParams)

	if !groupCreated {
		t.Error("OnGroupCreated not called")
	}

	addParams := `{"groupId":"TEST_010","deviceId":"device007","udid":"udid007"}`
	gm.AddMemberToGroup(AnyOsAccount, 1019, "test_app", addParams)

	if !deviceBound {
		t.Error("OnDeviceBound not called")
	}

	deleteParams := `{"groupId":"TEST_010","deviceId":"device007"}`
	gm.DeleteMemberFromGroup(AnyOsAccount, 1020, "test_app", deleteParams)

	if !deviceUnbound {
		t.Error("OnDeviceUnBound not called")
	}

	disbandParams := `{"groupId":"TEST_010"}`
	gm.DeleteGroup(AnyOsAccount, 1021, "test_app", disbandParams)

	if !groupDeleted {
		t.Error("OnGroupDeleted not called")
	}
}

// ============================================================================
// GroupAuthManager 完整功能测试
// ============================================================================

func TestAuthDeviceWithCallback(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	ga, _ := GetGaInstance()

	transmitted := false
	callback := &DeviceAuthCallback{
		OnTransmit: func(requestId int64, data []byte) bool {
			transmitted = true
			t.Logf("Transmitted %d bytes", len(data))
			return true
		},
	}

	authParams := `{"peerUdid":"test-device","serviceType":"softbus_auth"}`
	if err := ga.AuthDevice(AnyOsAccount, 2001, authParams, callback); err != nil {
		t.Fatalf("AuthDevice failed: %v", err)
	}

	if !transmitted {
		t.Error("OnTransmit not called")
	}
}

func TestProcessDataWithInvalidMessage(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	ga, _ := GetGaInstance()

	callback := &DeviceAuthCallback{
		OnTransmit: func(requestId int64, data []byte) bool {
			return true
		},
	}

	invalidData := []byte("invalid json")
	err := ga.ProcessData(2002, invalidData, callback)
	if err == nil {
		t.Error("Expected error with invalid data")
	}
}

func TestCancelAuthRequest(t *testing.T) {
	if err := InitDeviceAuthService(); err != nil {
		t.Fatalf("InitDeviceAuthService failed: %v", err)
	}
	defer DestroyDeviceAuthService()

	ga, _ := GetGaInstance()

	callback := &DeviceAuthCallback{
		OnTransmit: func(requestId int64, data []byte) bool {
			return true
		},
	}

	authParams := `{"peerUdid":"test-device"}`
	ga.AuthDevice(AnyOsAccount, 2003, authParams, callback)

	ga.CancelRequest(2003, "test_app")
}
