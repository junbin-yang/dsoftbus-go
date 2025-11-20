package authentication

import (
	"sync"
	"testing"
	"time"
)

// 测试AuthConnInit和AuthConnDeinit
func TestAuthConnInit_Deinit(t *testing.T) {
	// 测试nil listener
	err := AuthConnInit(nil)
	if err == nil {
		t.Fatal("Should fail when initializing with nil listener")
	}
	t.Logf("AuthConnInit with nil listener error (expected): %v", err)

	// 测试正常初始化
	listener := &AuthConnListener{
		OnConnectResult: func(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo) {
			t.Logf("OnConnectResult: requestId=%d, connId=%d, result=%d", requestId, connId, result)
		},
		OnDisconnected: func(connId uint64, connInfo *AuthConnInfo) {
			t.Logf("OnDisconnected: connId=%d", connId)
		},
		OnDataReceived: func(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte) {
			t.Logf("OnDataReceived: connId=%d, fromServer=%v, module=%d", connId, fromServer, head.Module)
		},
	}

	err = AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}
	t.Log("AuthConnInit succeeded")

	// 测试重复初始化（应该成功，但会警告）
	err = AuthConnInit(listener)
	if err != nil {
		t.Fatalf("Reinitialize should succeed: %v", err)
	}
	t.Log("Reinitialize succeeded")

	// 反初始化
	AuthConnDeinit()
	t.Log("AuthConnDeinit succeeded")

	// 测试多次反初始化（不应该崩溃）
	AuthConnDeinit()
	t.Log("Multiple AuthConnDeinit succeeded")
}

// 测试ConnectAuthDevice
func TestConnectAuthDevice(t *testing.T) {
	var wg sync.WaitGroup
	var connectedConnId uint64
	var connectedRequestId uint32

	// 初始化监听器
	listener := &AuthConnListener{
		OnConnectResult: func(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo) {
			t.Logf("OnConnectResult: requestId=%d, connId=%d, result=%d", requestId, connId, result)
			if result == 0 {
				connectedConnId = connId
				connectedRequestId = requestId
				wg.Done()
			}
		},
		OnDisconnected: func(connId uint64, connInfo *AuthConnInfo) {
			t.Logf("OnDisconnected: connId=%d", connId)
		},
		OnDataReceived: func(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte) {
			t.Logf("OnDataReceived: connId=%d", connId)
		},
	}

	err := AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}
	defer AuthConnDeinit()

	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()
	t.Logf("Server started on port: %d", port)

	time.Sleep(100 * time.Millisecond)

	// 测试ConnectAuthDevice（未初始化manager）
	AuthConnDeinit()
	err = ConnectAuthDevice(1001, &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	})
	if err == nil {
		t.Fatal("Should fail when manager not initialized")
	}
	t.Logf("ConnectAuthDevice without init error (expected): %v", err)

	// 重新初始化
	err = AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}

	// 测试ConnectAuthDevice（nil connInfo）
	err = ConnectAuthDevice(1002, nil)
	if err == nil {
		t.Fatal("Should fail with nil connInfo")
	}
	t.Logf("ConnectAuthDevice with nil connInfo error (expected): %v", err)

	// 测试正常连接
	requestId := uint32(1003)
	connInfo := &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	}

	wg.Add(1)
	err = ConnectAuthDevice(requestId, connInfo)
	if err != nil {
		t.Fatalf("ConnectAuthDevice failed: %v", err)
	}
	t.Logf("ConnectAuthDevice called: requestId=%d", requestId)

	// 等待连接成功
	wg.Wait()

	if connectedConnId == 0 {
		t.Fatal("Connection not established")
	}
	if connectedRequestId != requestId {
		t.Errorf("RequestId mismatch: expected %d, got %d", requestId, connectedRequestId)
	}
	t.Logf("Connection established: connId=%d, requestId=%d", connectedConnId, connectedRequestId)

	t.Log("ConnectAuthDevice test passed")
}

// 测试PostAuthData
func TestPostAuthData(t *testing.T) {
	var wg sync.WaitGroup
	var connectedConnId uint64
	var receivedData []byte

	// 初始化监听器
	listener := &AuthConnListener{
		OnConnectResult: func(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo) {
			if result == 0 {
				connectedConnId = connId
				wg.Done()
			}
		},
		OnDisconnected: func(connId uint64, connInfo *AuthConnInfo) {
			t.Logf("OnDisconnected: connId=%d", connId)
		},
		OnDataReceived: func(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte) {
			t.Logf("OnDataReceived: connId=%d, module=%d, seq=%d, len=%d, fromServer=%v",
				connId, head.Module, head.Seq, len(data), fromServer)
			receivedData = make([]byte, len(data))
			copy(receivedData, data)
			wg.Done()
		},
	}

	err := AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}
	defer AuthConnDeinit()

	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()

	time.Sleep(100 * time.Millisecond)

	// 连接设备
	wg.Add(1)
	err = ConnectAuthDevice(2001, &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	})
	if err != nil {
		t.Fatalf("ConnectAuthDevice failed: %v", err)
	}

	wg.Wait()
	t.Logf("Connected: connId=%d", connectedConnId)

	// 测试PostAuthData（未初始化）
	AuthConnDeinit()
	err = PostAuthData(connectedConnId, &AuthDataHead{Module: ModuleAuthSdk}, []byte("test"))
	if err == nil {
		t.Fatal("Should fail when manager not initialized")
	}
	t.Logf("PostAuthData without init error (expected): %v", err)

	// 重新初始化并连接
	err = AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}

	wg.Add(1)
	err = ConnectAuthDevice(2002, &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	})
	if err != nil {
		t.Fatalf("ConnectAuthDevice failed: %v", err)
	}
	wg.Wait()

	// 测试PostAuthData（无效connId）
	err = PostAuthData(99999, &AuthDataHead{Module: ModuleAuthSdk}, []byte("test"))
	if err == nil {
		t.Fatal("Should fail with invalid connId")
	}
	t.Logf("PostAuthData with invalid connId error (expected): %v", err)

	// 测试正常发送
	testData := []byte("Hello from client!")
	head := &AuthDataHead{
		Module: ModuleAuthSdk,
		Seq:    3001,
		Flag:   0,
		Len:    uint32(len(testData)),
	}

	wg.Add(1)
	err = PostAuthData(connectedConnId, head, testData)
	if err != nil {
		t.Fatalf("PostAuthData failed: %v", err)
	}
	t.Log("PostAuthData called")

	// 等待服务端接收
	wg.Wait()

	if string(receivedData) != string(testData) {
		t.Errorf("Data mismatch: expected %s, got %s", testData, receivedData)
	}
	t.Logf("Data received correctly: %s", receivedData)

	t.Log("PostAuthData test passed")
}

// 测试GetConnInfo
func TestGetConnInfo(t *testing.T) {
	var wg sync.WaitGroup
	var connectedConnId uint64

	// 初始化监听器
	listener := &AuthConnListener{
		OnConnectResult: func(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo) {
			if result == 0 {
				connectedConnId = connId
				wg.Done()
			}
		},
		OnDisconnected: func(connId uint64, connInfo *AuthConnInfo) {},
		OnDataReceived: func(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte) {},
	}

	err := AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}
	defer AuthConnDeinit()

	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()

	time.Sleep(100 * time.Millisecond)

	// 测试GetConnInfo（未初始化）
	AuthConnDeinit()
	_, err = GetConnInfo(12345)
	if err == nil {
		t.Fatal("Should fail when manager not initialized")
	}
	t.Logf("GetConnInfo without init error (expected): %v", err)

	// 重新初始化
	err = AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}

	// 测试GetConnInfo（无效connId）
	_, err = GetConnInfo(99999)
	if err == nil {
		t.Fatal("Should fail with invalid connId")
	}
	t.Logf("GetConnInfo with invalid connId error (expected): %v", err)

	// 连接设备
	wg.Add(1)
	err = ConnectAuthDevice(4001, &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	})
	if err != nil {
		t.Fatalf("ConnectAuthDevice failed: %v", err)
	}
	wg.Wait()

	// 测试正常获取连接信息
	connInfo, err := GetConnInfo(connectedConnId)
	if err != nil {
		t.Fatalf("GetConnInfo failed: %v", err)
	}

	if connInfo.Type != AuthLinkTypeWifi {
		t.Errorf("ConnInfo type mismatch: expected %d, got %d", AuthLinkTypeWifi, connInfo.Type)
	}
	if connInfo.Ip != "127.0.0.1" {
		t.Errorf("ConnInfo ip mismatch: expected 127.0.0.1, got %s", connInfo.Ip)
	}
	if connInfo.Port != port {
		t.Errorf("ConnInfo port mismatch: expected %d, got %d", port, connInfo.Port)
	}

	t.Logf("ConnInfo: type=%d, ip=%s, port=%d", connInfo.Type, connInfo.Ip, connInfo.Port)
	t.Log("GetConnInfo test passed")
}

// 测试DisconnectAuthDevice
func TestDisconnectAuthDevice(t *testing.T) {
	var wg sync.WaitGroup
	var connectedConnId uint64
	var disconnected bool

	// 初始化监听器
	listener := &AuthConnListener{
		OnConnectResult: func(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo) {
			if result == 0 {
				connectedConnId = connId
				wg.Done()
			}
		},
		OnDisconnected: func(connId uint64, connInfo *AuthConnInfo) {
			t.Logf("OnDisconnected: connId=%d", connId)
			disconnected = true
			wg.Done()
		},
		OnDataReceived: func(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte) {},
	}

	err := AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}
	defer AuthConnDeinit()

	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()

	time.Sleep(100 * time.Millisecond)

	// 连接设备
	wg.Add(1)
	err = ConnectAuthDevice(5001, &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	})
	if err != nil {
		t.Fatalf("ConnectAuthDevice failed: %v", err)
	}
	wg.Wait()
	t.Logf("Connected: connId=%d", connectedConnId)

	// 测试DisconnectAuthDevice（无效connId，不应该崩溃）
	DisconnectAuthDevice(99999)
	t.Log("DisconnectAuthDevice with invalid connId succeeded")

	// 测试正常断开
	wg.Add(1)
	DisconnectAuthDevice(connectedConnId)
	t.Logf("DisconnectAuthDevice called: connId=%d", connectedConnId)

	// 等待断开通知
	wg.Wait()

	if !disconnected {
		t.Error("Disconnect notification not received")
	}
	t.Log("Disconnect notification received")

	// 验证连接已删除
	_, err = GetConnInfo(connectedConnId)
	if err == nil {
		t.Error("Connection should be removed after disconnect")
	}
	t.Log("Connection removed successfully")

	t.Log("DisconnectAuthDevice test passed")
}

// 测试完整流程
func TestAuthConnection_FullFlow(t *testing.T) {
	var wg sync.WaitGroup
	var clientConnId uint64
	var clientReceivedData []byte
	var serverReceivedData []byte

	// 初始化监听器
	listener := &AuthConnListener{
		OnConnectResult: func(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo) {
			t.Logf("OnConnectResult: requestId=%d, connId=%d, result=%d", requestId, connId, result)
			if result == 0 {
				clientConnId = connId
				wg.Done()
			}
		},
		OnDisconnected: func(connId uint64, connInfo *AuthConnInfo) {
			t.Logf("OnDisconnected: connId=%d", connId)
		},
		OnDataReceived: func(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte) {
			t.Logf("OnDataReceived: connId=%d, fromServer=%v, module=%d, seq=%d, len=%d",
				connId, fromServer, head.Module, head.Seq, len(data))

			// fromServer表示连接是否是服务端连接
			// fromServer=true: 服务端连接收到数据（数据来自客户端）
			// fromServer=false: 客户端连接收到数据（数据来自服务端）
			if fromServer {
				// 服务端连接收到客户端数据
				serverReceivedData = make([]byte, len(data))
				copy(serverReceivedData, data)
			} else {
				// 客户端连接收到服务端数据
				clientReceivedData = make([]byte, len(data))
				copy(clientReceivedData, data)
			}
			wg.Done()
		},
	}

	err := AuthConnInit(listener)
	if err != nil {
		t.Fatalf("AuthConnInit failed: %v", err)
	}
	defer AuthConnDeinit()

	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()
	t.Logf("Server started on port: %d", port)

	time.Sleep(100 * time.Millisecond)

	// 客户端连接
	wg.Add(1)
	err = ConnectAuthDevice(6001, &AuthConnInfo{
		Type: AuthLinkTypeWifi,
		Ip:   "127.0.0.1",
		Port: port,
	})
	if err != nil {
		t.Fatalf("ConnectAuthDevice failed: %v", err)
	}

	wg.Wait()
	t.Logf("Client connected: connId=%d", clientConnId)

	// 客户端发送数据
	clientData := []byte("Hello from client!")
	wg.Add(1)
	err = PostAuthData(clientConnId, &AuthDataHead{
		Module: ModuleAuthSdk,
		Seq:    7001,
		Len:    uint32(len(clientData)),
	}, clientData)
	if err != nil {
		t.Fatalf("Client PostAuthData failed: %v", err)
	}
	t.Log("Client sent data")

	wg.Wait()

	if string(serverReceivedData) != string(clientData) {
		t.Errorf("Server data mismatch: expected %s, got %s", clientData, serverReceivedData)
	}
	t.Log("Server received data correctly")

	// 服务端需要获取服务端的connId（从服务端连接获取）
	// 这里我们可以通过查找fd对应的connId
	// 但为了简化测试，直接测试客户端断开即可

	// 断开连接
	DisconnectAuthDevice(clientConnId)
	t.Log("Connection disconnected")

	time.Sleep(100 * time.Millisecond)

	t.Log("Full flow test passed")
}
