package authentication

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// 测试数据包编解码
func TestPackUnpackSocketPkt(t *testing.T) {
	// 准备测试数据
	testData := []byte("Hello, Authentication!")
	pktHead := &SocketPktHead{
		Magic:  MagicNumber,
		Module: ModuleAuthSdk,
		Seq:    12345,
		Flag:   0,
		Len:    uint32(len(testData)),
	}

	// 测试打包
	packed, err := PackSocketPkt(pktHead, testData)
	if err != nil {
		t.Fatalf("Pack failed: %v", err)
	}

	expectedSize := AuthPktHeadLen + len(testData)
	if len(packed) != expectedSize {
		t.Fatalf("Packed size mismatch: expected %d, got %d", expectedSize, len(packed))
	}

	// 测试解包
	unpacked, err := UnpackSocketPkt(packed)
	if err != nil {
		t.Fatalf("Unpack failed: %v", err)
	}

	// 验证头部
	if unpacked.Magic != pktHead.Magic {
		t.Errorf("Magic mismatch: expected 0x%X, got 0x%X", pktHead.Magic, unpacked.Magic)
	}
	if unpacked.Module != pktHead.Module {
		t.Errorf("Module mismatch: expected %d, got %d", pktHead.Module, unpacked.Module)
	}
	if unpacked.Seq != pktHead.Seq {
		t.Errorf("Seq mismatch: expected %d, got %d", pktHead.Seq, unpacked.Seq)
	}
	if unpacked.Flag != pktHead.Flag {
		t.Errorf("Flag mismatch: expected %d, got %d", pktHead.Flag, unpacked.Flag)
	}
	if unpacked.Len != pktHead.Len {
		t.Errorf("Len mismatch: expected %d, got %d", pktHead.Len, unpacked.Len)
	}

	// 验证数据
	dataStart := AuthPktHeadLen
	dataEnd := dataStart + int(unpacked.Len)
	unpackedData := packed[dataStart:dataEnd]
	if !bytes.Equal(unpackedData, testData) {
		t.Errorf("Data mismatch: expected %s, got %s", testData, unpackedData)
	}

	t.Log("Pack/Unpack test passed")
}

// 测试ConnId工具函数
func TestConnIdUtils(t *testing.T) {
	connType := AuthLinkTypeWifi
	fd := int32(1234)

	// 生成ConnId
	connId := GenConnId(connType, fd)

	// 提取连接类型
	extractedType := GetConnType(connId)
	if extractedType != connType {
		t.Errorf("ConnType mismatch: expected %d, got %d", connType, extractedType)
	}

	// 提取fd
	extractedFd := GetFd(connId)
	if extractedFd != fd {
		t.Errorf("Fd mismatch: expected %d, got %d", fd, extractedFd)
	}

	// 测试字符串转换
	typeStr := GetConnTypeStr(connId)
	if typeStr != "wifi/eth" {
		t.Errorf("ConnTypeStr mismatch: expected 'wifi/eth', got '%s'", typeStr)
	}

	t.Logf("ConnId test passed: connId=0x%X, type=%d, fd=%d, str=%s",
		connId, extractedType, extractedFd, typeStr)
}

// 测试完整流程：服务端监听+客户端连接+数据收发
func TestAuthTcpConnection_FullFlow(t *testing.T) {
	var wg sync.WaitGroup
	var serverReceivedData []byte
	var clientReceivedData []byte
	var serverFd int

	// 统一回调（根据isClient判断）
	unifiedCallback := &SocketCallback{
		OnConnected: func(module ListenerModule, fd int, isClient bool) {
			if isClient {
				t.Logf("[CLIENT] OnConnected: module=%d, fd=%d", module, fd)
			} else {
				serverFd = fd
				t.Logf("[SERVER] OnConnected: module=%d, fd=%d", module, fd)
			}
		},
		OnDisconnected: func(fd int) {
			t.Logf("OnDisconnected: fd=%d", fd)
		},
		OnDataReceived: func(module ListenerModule, fd int, head *AuthDataHead, data []byte) {
			// 根据fd判断是服务端还是客户端接收
			if fd == serverFd {
				t.Logf("[SERVER] OnDataReceived: module=%d, fd=%d, dataType=0x%X, len=%d, data=%s",
					module, fd, head.DataType, head.Len, string(data))
				serverReceivedData = make([]byte, len(data))
				copy(serverReceivedData, data)
				wg.Done()
			} else {
				t.Logf("[CLIENT] OnDataReceived: module=%d, fd=%d, dataType=0x%X, len=%d, data=%s",
					module, fd, head.DataType, head.Len, string(data))
				clientReceivedData = make([]byte, len(data))
				copy(clientReceivedData, data)
				wg.Done()
			}
		},
	}

	// 设置统一回调
	err := SetSocketCallback(unifiedCallback)
	if err != nil {
		t.Fatalf("SetSocketCallback failed: %v", err)
	}

	// 启动服务端监听
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()
	t.Logf("Server started on port: %d", port)

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 客户端连接
	fd, err := SocketConnectDevice("127.0.0.1", port)
	if err != nil {
		t.Fatalf("SocketConnectDevice failed: %v", err)
	}
	defer SocketDisconnectDevice(Auth, fd)
	t.Logf("Client connected: fd=%d", fd)

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 客户端发送数据
	clientData := []byte("Hello from client!")
	clientHead := &AuthDataHead{
		DataType: DataTypeAuth,
		Module:   ModuleAuthSdk,
		Seq:      1001,
		Flag:     0,
		Len:      uint32(len(clientData)),
	}

	wg.Add(1) // 等待服务端接收数据
	err = SocketPostBytes(fd, clientHead, clientData)
	if err != nil {
		t.Fatalf("Client SocketPostBytes failed: %v", err)
	}
	t.Log("Client sent data")

	// 等待服务端接收数据
	wg.Wait()

	// 验证服务端收到的数据
	if !bytes.Equal(serverReceivedData, clientData) {
		t.Errorf("Server received data mismatch: expected %s, got %s",
			clientData, serverReceivedData)
	}

	// 服务端回复数据
	serverData := []byte("Hello from server!")
	serverHead := &AuthDataHead{
		DataType: DataTypeDeviceInfo,
		Module:   ModuleAuthConnection,
		Seq:      2001,
		Flag:     0,
		Len:      uint32(len(serverData)),
	}

	wg.Add(1) // 等待客户端接收数据
	err = SocketPostBytes(serverFd, serverHead, serverData)
	if err != nil {
		t.Fatalf("Server SocketPostBytes failed: %v", err)
	}
	t.Log("Server sent data")

	// 等待客户端接收数据
	wg.Wait()

	// 验证客户端收到的数据
	if !bytes.Equal(clientReceivedData, serverData) {
		t.Errorf("Client received data mismatch: expected %s, got %s",
			serverData, clientReceivedData)
	}

	t.Log("Full flow test passed!")
}

// 测试Socket连接信息查询
func TestSocketGetConnInfo(t *testing.T) {
	callback := &SocketCallback{
		OnConnected: func(module ListenerModule, fd int, isClient bool) {
			t.Logf("OnConnected: fd=%d", fd)
		},
		OnDisconnected: func(fd int) {},
		OnDataReceived: func(module ListenerModule, fd int, head *AuthDataHead, data []byte) {},
	}

	err := SetSocketCallback(callback)
	if err != nil {
		t.Fatalf("SetSocketCallback failed: %v", err)
	}

	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()

	time.Sleep(100 * time.Millisecond)

	// 客户端连接
	fd, err := SocketConnectDevice("127.0.0.1", port)
	if err != nil {
		t.Fatalf("SocketConnectDevice failed: %v", err)
	}
	defer SocketDisconnectDevice(Auth, fd)

	time.Sleep(100 * time.Millisecond)

	// 查询连接信息
	connInfo, isServer, err := SocketGetConnInfo(fd)
	if err != nil {
		t.Fatalf("SocketGetConnInfo failed: %v", err)
	}

	t.Logf("ConnInfo: type=%d, ip=%s, port=%d, isServer=%v",
		connInfo.Type, connInfo.Ip, connInfo.Port, isServer)

	if connInfo.Type != AuthLinkTypeWifi {
		t.Errorf("ConnInfo type mismatch: expected %d, got %d", AuthLinkTypeWifi, connInfo.Type)
	}

	if isServer {
		t.Error("Client connection should not be server")
	}
}
