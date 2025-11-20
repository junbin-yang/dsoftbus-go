package authentication

import (
	"sync"
	"testing"
	"time"
)

// ============================================================================
// 测试回调收集器
// ============================================================================

type authCallbackCollector struct {
	connOpened      []uint32           // 记录OnConnOpened的requestId
	connOpenFailed  []uint32           // 记录OnConnOpenFailed的requestId
	dataReceived    []int64            // 记录OnDataReceived的authId
	authIdMap       map[uint32]int64   // requestId -> authId映射
	receivedData    map[int64][]byte   // authId -> 接收到的数据
	mu              sync.Mutex
}

func newAuthCallbackCollector() *authCallbackCollector {
	return &authCallbackCollector{
		connOpened:     make([]uint32, 0),
		connOpenFailed: make([]uint32, 0),
		dataReceived:   make([]int64, 0),
		authIdMap:      make(map[uint32]int64),
		receivedData:   make(map[int64][]byte),
	}
}

func (c *authCallbackCollector) onConnOpened(requestId uint32, authId int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connOpened = append(c.connOpened, requestId)
	c.authIdMap[requestId] = authId
}

func (c *authCallbackCollector) onConnOpenFailed(requestId uint32, reason int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connOpenFailed = append(c.connOpenFailed, requestId)
}

func (c *authCallbackCollector) onDataReceived(authId int64, head *AuthDataHead, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dataReceived = append(c.dataReceived, authId)
	c.receivedData[authId] = data
}

func (c *authCallbackCollector) getAuthId(requestId uint32) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	authId, exists := c.authIdMap[requestId]
	return authId, exists
}

func (c *authCallbackCollector) hasConnOpened(requestId uint32) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.connOpened {
		if id == requestId {
			return true
		}
	}
	return false
}

func (c *authCallbackCollector) hasConnOpenFailed(requestId uint32) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.connOpenFailed {
		if id == requestId {
			return true
		}
	}
	return false
}

func (c *authCallbackCollector) hasDataReceived(authId int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.dataReceived {
		if id == authId {
			return true
		}
	}
	return false
}

func (c *authCallbackCollector) getData(authId int64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, exists := c.receivedData[authId]
	return data, exists
}

// ============================================================================
// 测试用例
// ============================================================================

// TestAuthDeviceInit_Deinit 测试初始化和反初始化
func TestAuthDeviceInit_Deinit(t *testing.T) {
	// 测试nil回调
	err := AuthDeviceInit(nil)
	if err == nil {
		t.Error("Expected error for nil callback")
	}
	t.Logf("AuthDeviceInit with nil callback error (expected): %v", err)

	// 正常初始化
	collector := newAuthCallbackCollector()
	callback := &AuthConnCallback{
		OnConnOpened:     collector.onConnOpened,
		OnConnOpenFailed: collector.onConnOpenFailed,
		OnDataReceived:   collector.onDataReceived,
	}

	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}
	t.Log("AuthDeviceInit succeeded")

	// 验证初始化状态
	service := getAuthManagerService()
	if !service.initialized {
		t.Error("Service should be initialized")
	}

	// 重复初始化（应该成功，会重新初始化）
	err = AuthDeviceInit(callback)
	if err != nil {
		t.Errorf("Reinitialize failed: %v", err)
	}
	t.Log("Reinitialize succeeded")

	// 反初始化
	AuthDeviceDeinit()
	if service.initialized {
		t.Error("Service should be deinitialized")
	}
	t.Log("AuthDeviceDeinit succeeded")

	// 多次反初始化（应该安全）
	AuthDeviceDeinit()
	t.Log("Multiple AuthDeviceDeinit succeeded")
}

// TestAuthDeviceOpenConn 测试打开连接
func TestAuthDeviceOpenConn(t *testing.T) {
	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer StopSocketListening()
	t.Logf("Server started on port: %d", port)

	// 初始化Auth Manager
	collector := newAuthCallbackCollector()
	callback := &AuthConnCallback{
		OnConnOpened:     collector.onConnOpened,
		OnConnOpenFailed: collector.onConnOpenFailed,
		OnDataReceived:   collector.onDataReceived,
	}

	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}
	defer AuthDeviceDeinit()

	// 测试：未初始化时打开连接
	AuthDeviceDeinit()
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	}
	err = AuthDeviceOpenConn(connInfo, 1001, nil)
	if err == nil {
		t.Error("Expected error when not initialized")
	}
	t.Logf("OpenConn without init error (expected): %v", err)

	// 重新初始化
	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}

	// 测试：nil connInfo
	err = AuthDeviceOpenConn(nil, 1002, nil)
	if err == nil {
		t.Error("Expected error for nil connInfo")
	}
	t.Logf("OpenConn with nil connInfo error (expected): %v", err)

	// 正常打开连接
	requestId := uint32(1003)
	err = AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		t.Fatalf("AuthDeviceOpenConn failed: %v", err)
	}
	t.Logf("AuthDeviceOpenConn called: requestId=%d", requestId)

	// 等待连接建立
	time.Sleep(200 * time.Millisecond)

	// 验证OnConnOpened被调用
	if !collector.hasConnOpened(requestId) {
		t.Errorf("OnConnOpened not called for requestId=%d", requestId)
	}

	authId, exists := collector.getAuthId(requestId)
	if !exists {
		t.Fatalf("AuthId not found for requestId=%d", requestId)
	}
	t.Logf("Connection established: authId=%d, requestId=%d", authId, requestId)

	// 验证AuthManager存在
	manager, err := GetAuthManagerByAuthId(authId)
	if err != nil {
		t.Errorf("GetAuthManagerByAuthId failed: %v", err)
	}
	if manager.AuthId != authId {
		t.Errorf("AuthId mismatch: expected=%d, got=%d", authId, manager.AuthId)
	}

	t.Log("AuthDeviceOpenConn test passed")
}

// TestAuthDevicePostTransData 测试数据发送
func TestAuthDevicePostTransData(t *testing.T) {
	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer StopSocketListening()

	// 初始化Auth Manager
	collector := newAuthCallbackCollector()
	callback := &AuthConnCallback{
		OnConnOpened:     collector.onConnOpened,
		OnConnOpenFailed: collector.onConnOpenFailed,
		OnDataReceived:   collector.onDataReceived,
	}

	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}
	defer AuthDeviceDeinit()

	// 打开连接
	requestId := uint32(2001)
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	}

	err = AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		t.Fatalf("AuthDeviceOpenConn failed: %v", err)
	}

	// 等待连接建立
	time.Sleep(200 * time.Millisecond)

	authId, exists := collector.getAuthId(requestId)
	if !exists {
		t.Fatalf("AuthId not found for requestId=%d", requestId)
	}
	t.Logf("Connected: authId=%d", authId)

	// 测试：服务未初始化
	AuthDeviceDeinit()
	err = AuthDevicePostTransData(authId, ModuleAuthSdk, 0, []byte("test"))
	if err == nil {
		t.Error("Expected error when not initialized")
	}
	t.Logf("PostTransData without init error (expected): %v", err)

	// 重新初始化
	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}

	// 打开新连接
	requestId = 2002
	err = AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		t.Fatalf("AuthDeviceOpenConn failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	authId, _ = collector.getAuthId(requestId)

	// 测试：无效的authId
	err = AuthDevicePostTransData(99999, ModuleAuthSdk, 0, []byte("test"))
	if err == nil {
		t.Error("Expected error for invalid authId")
	}
	t.Logf("PostTransData with invalid authId error (expected): %v", err)

	// 正常发送数据
	testData := []byte("Hello from client!")
	err = AuthDevicePostTransData(authId, ModuleAuthSdk, 0, testData)
	if err != nil {
		t.Errorf("PostTransData failed: %v", err)
	}
	t.Log("PostTransData called")

	// 等待数据接收（服务端会收到数据）
	time.Sleep(100 * time.Millisecond)

	t.Log("PostTransData test passed")
}

// TestSessionKeyManagement 测试Session Key管理
func TestSessionKeyManagement(t *testing.T) {
	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer StopSocketListening()

	// 初始化Auth Manager
	collector := newAuthCallbackCollector()
	callback := &AuthConnCallback{
		OnConnOpened:     collector.onConnOpened,
		OnConnOpenFailed: collector.onConnOpenFailed,
		OnDataReceived:   collector.onDataReceived,
	}

	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}
	defer AuthDeviceDeinit()

	// 打开连接
	requestId := uint32(3001)
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	}

	err = AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		t.Fatalf("AuthDeviceOpenConn failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	authId, exists := collector.getAuthId(requestId)
	if !exists {
		t.Fatalf("AuthId not found")
	}

	// 设置Session Key
	sessionKey := make([]byte, 16)
	for i := 0; i < 16; i++ {
		sessionKey[i] = byte(i)
	}

	err = AuthManagerSetSessionKey(authId, sessionKey)
	if err != nil {
		t.Errorf("SetSessionKey failed: %v", err)
	}
	t.Log("Session key set successfully")

	// 获取Session Key
	key, err := AuthManagerGetSessionKey(authId, 0)
	if err != nil {
		t.Errorf("GetSessionKey failed: %v", err)
	}
	if key.Index != 0 {
		t.Errorf("Key index mismatch: expected=0, got=%d", key.Index)
	}
	t.Logf("Session key retrieved: index=%d", key.Index)

	// 获取最新Session Key
	latestKey, err := AuthManagerGetLatestSessionKey(authId)
	if err != nil {
		t.Errorf("GetLatestSessionKey failed: %v", err)
	}
	if latestKey.Index != 0 {
		t.Errorf("Latest key index mismatch: expected=0, got=%d", latestKey.Index)
	}
	t.Log("Latest session key retrieved successfully")

	// 测试无效authId
	err = AuthManagerSetSessionKey(99999, sessionKey)
	if err == nil {
		t.Error("Expected error for invalid authId")
	}
	t.Logf("SetSessionKey with invalid authId error (expected): %v", err)

	t.Log("SessionKeyManagement test passed")
}

// TestDeviceInfoQuery 测试设备信息查询
func TestDeviceInfoQuery(t *testing.T) {
	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer StopSocketListening()

	// 初始化Auth Manager
	collector := newAuthCallbackCollector()
	callback := &AuthConnCallback{
		OnConnOpened:     collector.onConnOpened,
		OnConnOpenFailed: collector.onConnOpenFailed,
		OnDataReceived:   collector.onDataReceived,
	}

	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}
	defer AuthDeviceDeinit()

	// 打开连接
	requestId := uint32(4001)
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
		Udid: "test-udid-12345",
	}

	err = AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		t.Fatalf("AuthDeviceOpenConn failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	authId, exists := collector.getAuthId(requestId)
	if !exists {
		t.Fatalf("AuthId not found")
	}

	// 获取连接信息
	info, err := AuthDeviceGetConnInfo(authId)
	if err != nil {
		t.Errorf("GetConnInfo failed: %v", err)
	}
	if info.Ip != "127.0.0.1" {
		t.Errorf("IP mismatch: expected=127.0.0.1, got=%s", info.Ip)
	}
	if info.Port != port {
		t.Errorf("Port mismatch: expected=%d, got=%d", port, info.Port)
	}
	t.Logf("ConnInfo: type=%d, ip=%s, port=%d", info.Type, info.Ip, info.Port)

	// 获取ServerSide
	isServer, err := AuthDeviceGetServerSide(authId)
	if err != nil {
		t.Errorf("GetServerSide failed: %v", err)
	}
	if isServer {
		t.Error("Should be client side")
	}
	t.Logf("ServerSide: %v", isServer)

	// 测试无效authId
	_, err = AuthDeviceGetConnInfo(99999)
	if err == nil {
		t.Error("Expected error for invalid authId")
	}
	t.Logf("GetConnInfo with invalid authId error (expected): %v", err)

	t.Log("DeviceInfoQuery test passed")
}

// TestAuthDeviceCloseConn 测试关闭连接
func TestAuthDeviceCloseConn(t *testing.T) {
	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer StopSocketListening()

	// 初始化Auth Manager
	collector := newAuthCallbackCollector()
	callback := &AuthConnCallback{
		OnConnOpened:     collector.onConnOpened,
		OnConnOpenFailed: collector.onConnOpenFailed,
		OnDataReceived:   collector.onDataReceived,
	}

	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}
	defer AuthDeviceDeinit()

	// 打开连接
	requestId := uint32(5001)
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	}

	err = AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		t.Fatalf("AuthDeviceOpenConn failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	authId, exists := collector.getAuthId(requestId)
	if !exists {
		t.Fatalf("AuthId not found")
	}
	t.Logf("Connected: authId=%d", authId)

	// 测试关闭无效的authId（应该安全）
	AuthDeviceCloseConn(99999)
	t.Log("CloseConn with invalid authId succeeded")

	// 正常关闭连接
	AuthDeviceCloseConn(authId)
	t.Logf("CloseConn called: authId=%d", authId)

	// 等待断开（增加等待时间以确保异步清理完成）
	time.Sleep(300 * time.Millisecond)

	// 验证AuthManager已被移除
	_, err = GetAuthManagerByAuthId(authId)
	if err == nil {
		t.Error("AuthManager should be removed")
	}
	t.Log("AuthManager removed successfully")

	t.Log("CloseConn test passed")
}

// TestAuthManager_FullFlow 测试完整流程
func TestAuthManager_FullFlow(t *testing.T) {
	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer StopSocketListening()
	t.Logf("Server started on port: %d", port)

	// 初始化Auth Manager
	collector := newAuthCallbackCollector()
	callback := &AuthConnCallback{
		OnConnOpened: func(requestId uint32, authId int64) {
			collector.onConnOpened(requestId, authId)
			t.Logf("OnConnOpened: requestId=%d, authId=%d", requestId, authId)
		},
		OnConnOpenFailed: func(requestId uint32, reason int32) {
			collector.onConnOpenFailed(requestId, reason)
			t.Logf("OnConnOpenFailed: requestId=%d, reason=%d", requestId, reason)
		},
		OnDataReceived: func(authId int64, head *AuthDataHead, data []byte) {
			collector.onDataReceived(authId, head, data)
			t.Logf("OnDataReceived: authId=%d, module=%d, len=%d", authId, head.Module, len(data))
		},
	}

	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}
	defer AuthDeviceDeinit()
	t.Log("Auth Manager initialized")

	// 1. 打开连接
	requestId := uint32(6001)
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
		Udid: "test-device-udid",
	}

	err = AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		t.Fatalf("AuthDeviceOpenConn failed: %v", err)
	}

	// 等待连接建立
	time.Sleep(200 * time.Millisecond)

	authId, exists := collector.getAuthId(requestId)
	if !exists {
		t.Fatalf("AuthId not found")
	}
	t.Logf("Connection established: authId=%d", authId)

	// 2. 设置Session Key
	sessionKey := make([]byte, 16)
	for i := 0; i < 16; i++ {
		sessionKey[i] = byte(i + 1)
	}

	err = AuthManagerSetSessionKey(authId, sessionKey)
	if err != nil {
		t.Errorf("SetSessionKey failed: %v", err)
	}
	t.Log("Session key set")

	// 3. 发送数据
	testData := []byte("Test auth message")
	err = AuthDevicePostTransData(authId, ModuleAuthSdk, 0, testData)
	if err != nil {
		t.Errorf("PostTransData failed: %v", err)
	}
	t.Log("Data sent")

	// 4. 查询信息
	info, err := AuthDeviceGetConnInfo(authId)
	if err != nil {
		t.Errorf("GetConnInfo failed: %v", err)
	}
	t.Logf("ConnInfo: ip=%s, port=%d", info.Ip, info.Port)

	isServer, err := AuthDeviceGetServerSide(authId)
	if err != nil {
		t.Errorf("GetServerSide failed: %v", err)
	}
	t.Logf("IsServer: %v", isServer)

	// 5. 获取Session Key
	key, err := AuthManagerGetLatestSessionKey(authId)
	if err != nil {
		t.Errorf("GetLatestSessionKey failed: %v", err)
	}
	t.Logf("Session key retrieved: index=%d", key.Index)

	// 6. 关闭连接
	AuthDeviceCloseConn(authId)
	t.Log("Connection closed")

	// 等待清理（增加等待时间以确保异步清理完成）
	time.Sleep(300 * time.Millisecond)

	// 验证清理
	_, err = GetAuthManagerByAuthId(authId)
	if err == nil {
		t.Error("AuthManager should be removed")
	}

	t.Log("Full flow test passed!")
}

// TestGetAuthManager 测试查找函数
func TestGetAuthManager(t *testing.T) {
	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer StopSocketListening()

	// 初始化Auth Manager
	collector := newAuthCallbackCollector()
	callback := &AuthConnCallback{
		OnConnOpened:     collector.onConnOpened,
		OnConnOpenFailed: collector.onConnOpenFailed,
		OnDataReceived:   collector.onDataReceived,
	}

	err = AuthDeviceInit(callback)
	if err != nil {
		t.Fatalf("AuthDeviceInit failed: %v", err)
	}
	defer AuthDeviceDeinit()

	// 打开连接
	requestId := uint32(7001)
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	}

	err = AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		t.Fatalf("AuthDeviceOpenConn failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	authId, _ := collector.getAuthId(requestId)

	// 通过authId查找
	manager, err := GetAuthManagerByAuthId(authId)
	if err != nil {
		t.Errorf("GetAuthManagerByAuthId failed: %v", err)
	}
	if manager.AuthId != authId {
		t.Errorf("AuthId mismatch")
	}
	t.Logf("Found by authId: %d", authId)

	// 通过connId查找
	connId := manager.ConnId
	manager2, err := GetAuthManagerByConnId(connId)
	if err != nil {
		t.Errorf("GetAuthManagerByConnId failed: %v", err)
	}
	if manager2.AuthId != authId {
		t.Errorf("AuthId mismatch")
	}
	t.Logf("Found by connId: %d", connId)

	// 测试无效查找
	_, err = GetAuthManagerByAuthId(99999)
	if err == nil {
		t.Error("Expected error for invalid authId")
	}

	_, err = GetAuthManagerByConnId(99999)
	if err == nil {
		t.Error("Expected error for invalid connId")
	}

	t.Log("GetAuthManager test passed")
}
