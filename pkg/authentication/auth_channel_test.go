package authentication

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// 测试监听器注册和注销
func TestAuthChannelListener_RegUnreg(t *testing.T) {
	// 创建测试监听器
	listener := &AuthChannelListener{
		OnDataReceived: func(channelId int, data *AuthChannelData) {
			t.Logf("OnDataReceived: channelId=%d, module=%d", channelId, data.Module)
		},
		OnDisconnected: func(channelId int) {
			t.Logf("OnDisconnected: channelId=%d", channelId)
		},
	}

	// 测试注册
	err := RegAuthChannelListener(ModuleAuthChannel, listener)
	if err != nil {
		t.Fatalf("RegAuthChannelListener failed: %v", err)
	}

	// 测试重复注册（应该覆盖）
	err = RegAuthChannelListener(ModuleAuthChannel, listener)
	if err != nil {
		t.Fatalf("Re-register failed: %v", err)
	}

	// 测试注册另一个模块
	err = RegAuthChannelListener(ModuleAuthMsg, listener)
	if err != nil {
		t.Fatalf("RegAuthChannelListener for MODULE_AUTH_MSG failed: %v", err)
	}

	// 测试注销
	UnregAuthChannelListener(ModuleAuthChannel)
	UnregAuthChannelListener(ModuleAuthMsg)

	// 测试注册nil监听器（应该失败）
	err = RegAuthChannelListener(ModuleAuthChannel, nil)
	if err == nil {
		t.Fatal("Should fail when registering nil listener")
	}

	// 测试注册没有OnDataReceived的监听器（应该失败）
	badListener := &AuthChannelListener{
		OnDisconnected: func(channelId int) {},
	}
	err = RegAuthChannelListener(ModuleAuthChannel, badListener)
	if err == nil {
		t.Fatal("Should fail when OnDataReceived is nil")
	}

	t.Log("Listener registration test passed")
}

// 测试Auth Channel打开和关闭
func TestAuthChannel_OpenClose(t *testing.T) {
	// 启动测试服务器
	callback := &SocketCallback{
		OnConnected: func(module ListenerModule, fd int, isClient bool) {
			t.Logf("Server OnConnected: fd=%d", fd)
		},
		OnDisconnected: func(fd int) {
			t.Logf("Server OnDisconnected: fd=%d", fd)
		},
		OnDataReceived: func(module ListenerModule, fd int, head *AuthDataHead, data []byte) {
			t.Logf("Server OnDataReceived: fd=%d", fd)
		},
	}

	err := SetSocketCallback(callback)
	if err != nil {
		t.Fatalf("SetSocketCallback failed: %v", err)
	}

	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()
	t.Logf("Server started on port: %d", port)

	time.Sleep(100 * time.Millisecond)

	// 测试打开通道
	channelId := AuthOpenChannel("127.0.0.1", port)
	if channelId == InvalidChannelId {
		t.Fatal("AuthOpenChannel failed")
	}
	t.Logf("Channel opened: channelId=%d", channelId)

	time.Sleep(100 * time.Millisecond)

	// 测试关闭通道
	AuthCloseChannel(channelId)
	t.Log("Channel closed")

	time.Sleep(100 * time.Millisecond)

	// 测试打开无效地址的通道
	invalidChannelId := AuthOpenChannel("invalid.ip", port)
	if invalidChannelId != InvalidChannelId {
		t.Fatal("Should fail when connecting to invalid address")
	}

	// 测试关闭无效通道（不应该崩溃）
	AuthCloseChannel(InvalidChannelId)

	t.Log("AuthChannel open/close test passed")
}

// 测试Auth Channel消息路由
func TestAuthChannel_MessageRouting(t *testing.T) {
	var wg sync.WaitGroup

	var channelReceivedData []byte
	var msgReceivedData []byte

	// 设置Socket层回调
	socketCallback := &SocketCallback{
		OnConnected: func(module ListenerModule, fd int, isClient bool) {
			t.Logf("[SOCKET] OnConnected: fd=%d, isClient=%v", fd, isClient)
		},
		OnDisconnected: func(fd int) {
			t.Logf("[SOCKET] OnDisconnected: fd=%d", fd)
		},
		OnDataReceived: func(module ListenerModule, fd int, head *AuthDataHead, data []byte) {
			// 模块8和9的消息应该被路由到Auth Channel，不应该到这里
			// 但其他模块（如module=3）应该路由到这里
			if head.Module == ModuleAuthChannel || head.Module == ModuleAuthMsg {
				t.Errorf("[SOCKET] Should not receive MODULE_AUTH_CHANNEL/MSG data: module=%d", head.Module)
			} else {
				t.Logf("[SOCKET] OnDataReceived (expected for module=%d): fd=%d, len=%d, data=%s",
					head.Module, fd, head.Len, string(data))
			}
		},
	}

	err := SetSocketCallback(socketCallback)
	if err != nil {
		t.Fatalf("SetSocketCallback failed: %v", err)
	}

	// 注册Auth Channel监听器
	channelListener := &AuthChannelListener{
		OnDataReceived: func(channelId int, data *AuthChannelData) {
			t.Logf("[CHANNEL_LISTENER] OnDataReceived: channelId=%d, module=%d, len=%d, data=%s",
				channelId, data.Module, data.Len, string(data.Data))
			channelReceivedData = make([]byte, len(data.Data))
			copy(channelReceivedData, data.Data)
			wg.Done()
		},
		OnDisconnected: func(channelId int) {
			t.Logf("[CHANNEL_LISTENER] OnDisconnected: channelId=%d", channelId)
		},
	}

	msgListener := &AuthChannelListener{
		OnDataReceived: func(channelId int, data *AuthChannelData) {
			t.Logf("[MSG_LISTENER] OnDataReceived: channelId=%d, module=%d, len=%d, data=%s",
				channelId, data.Module, data.Len, string(data.Data))
			msgReceivedData = make([]byte, len(data.Data))
			copy(msgReceivedData, data.Data)
			wg.Done()
		},
		OnDisconnected: func(channelId int) {
			t.Logf("[MSG_LISTENER] OnDisconnected: channelId=%d", channelId)
		},
	}

	err = RegAuthChannelListener(ModuleAuthChannel, channelListener)
	if err != nil {
		t.Fatalf("RegAuthChannelListener(CHANNEL) failed: %v", err)
	}

	err = RegAuthChannelListener(ModuleAuthMsg, msgListener)
	if err != nil {
		t.Fatalf("RegAuthChannelListener(MSG) failed: %v", err)
	}

	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()
	t.Logf("Server started on port: %d", port)

	time.Sleep(100 * time.Millisecond)

	// 打开通道
	channelId := AuthOpenChannel("127.0.0.1", port)
	if channelId == InvalidChannelId {
		t.Fatal("AuthOpenChannel failed")
	}
	defer AuthCloseChannel(channelId)
	t.Logf("Channel opened: channelId=%d", channelId)

	time.Sleep(100 * time.Millisecond)

	// 测试1: 发送MODULE_AUTH_CHANNEL消息（应该路由到channelListener）
	channelData := &AuthChannelData{
		Module: ModuleAuthChannel,
		Seq:    1001,
		Flag:   0,
		Data:   []byte("Test CHANNEL message"),
		Len:    uint32(len("Test CHANNEL message")),
	}

	wg.Add(1)
	err = AuthPostChannelData(channelId, channelData)
	if err != nil {
		t.Fatalf("AuthPostChannelData(CHANNEL) failed: %v", err)
	}
	t.Log("Sent MODULE_AUTH_CHANNEL message")

	// 等待服务端接收
	wg.Wait()

	// 验证数据
	if !bytes.Equal(channelReceivedData, channelData.Data) {
		t.Errorf("CHANNEL data mismatch: expected %s, got %s", channelData.Data, channelReceivedData)
	}

	// 测试2: 发送MODULE_AUTH_MSG消息（应该路由到msgListener）
	msgData := &AuthChannelData{
		Module: ModuleAuthMsg,
		Seq:    2001,
		Flag:   0,
		Data:   []byte("Test MSG message"),
		Len:    uint32(len("Test MSG message")),
	}

	wg.Add(1)
	err = AuthPostChannelData(channelId, msgData)
	if err != nil {
		t.Fatalf("AuthPostChannelData(MSG) failed: %v", err)
	}
	t.Log("Sent MODULE_AUTH_MSG message")

	// 等待服务端接收
	wg.Wait()

	// 验证数据
	if !bytes.Equal(msgReceivedData, msgData.Data) {
		t.Errorf("MSG data mismatch: expected %s, got %s", msgData.Data, msgReceivedData)
	}

	// 测试3: 发送其他模块消息（应该路由到SocketCallback，会触发错误日志）
	otherData := &AuthChannelData{
		Module: ModuleAuthSdk, // module=3
		Seq:    3001,
		Flag:   0,
		Data:   []byte("Test OTHER message"),
		Len:    uint32(len("Test OTHER message")),
	}

	err = AuthPostChannelData(channelId, otherData)
	if err != nil {
		t.Fatalf("AuthPostChannelData(OTHER) failed: %v", err)
	}
	t.Log("Sent MODULE_AUTH_SDK message (should route to SocketCallback)")

	time.Sleep(100 * time.Millisecond)

	// 注销监听器
	UnregAuthChannelListener(ModuleAuthChannel)
	UnregAuthChannelListener(ModuleAuthMsg)

	t.Log("Auth Channel message routing test passed")
}

// 测试断开连接通知
func TestAuthChannel_DisconnectNotification(t *testing.T) {
	var wg sync.WaitGroup
	var channelDisconnected bool
	var msgDisconnected bool

	// 设置Socket层回调
	socketCallback := &SocketCallback{
		OnConnected: func(module ListenerModule, fd int, isClient bool) {
			t.Logf("OnConnected: fd=%d, isClient=%v", fd, isClient)
		},
		OnDisconnected: func(fd int) {
			t.Logf("[SOCKET] OnDisconnected: fd=%d", fd)
		},
		OnDataReceived: func(module ListenerModule, fd int, head *AuthDataHead, data []byte) {},
	}

	err := SetSocketCallback(socketCallback)
	if err != nil {
		t.Fatalf("SetSocketCallback failed: %v", err)
	}

	// 注册Auth Channel监听器
	channelListener := &AuthChannelListener{
		OnDataReceived: func(channelId int, data *AuthChannelData) {},
		OnDisconnected: func(channelId int) {
			t.Logf("[CHANNEL_LISTENER] OnDisconnected: channelId=%d", channelId)
			channelDisconnected = true
			wg.Done()
		},
	}

	msgListener := &AuthChannelListener{
		OnDataReceived: func(channelId int, data *AuthChannelData) {},
		OnDisconnected: func(channelId int) {
			t.Logf("[MSG_LISTENER] OnDisconnected: channelId=%d", channelId)
			msgDisconnected = true
			wg.Done()
		},
	}

	err = RegAuthChannelListener(ModuleAuthChannel, channelListener)
	if err != nil {
		t.Fatalf("RegAuthChannelListener(CHANNEL) failed: %v", err)
	}

	err = RegAuthChannelListener(ModuleAuthMsg, msgListener)
	if err != nil {
		t.Fatalf("RegAuthChannelListener(MSG) failed: %v", err)
	}

	// 启动服务器
	port, err := StartSocketListening(Auth, "127.0.0.1", 0)
	if err != nil {
		t.Fatalf("StartSocketListening failed: %v", err)
	}
	defer StopSocketListening()

	time.Sleep(100 * time.Millisecond)

	// 打开通道
	channelId := AuthOpenChannel("127.0.0.1", port)
	if channelId == InvalidChannelId {
		t.Fatal("AuthOpenChannel failed")
	}

	time.Sleep(100 * time.Millisecond)

	// 等待两个监听器的断开通知
	wg.Add(2)

	// 关闭通道
	AuthCloseChannel(channelId)
	t.Log("Channel closed")

	// 等待断开通知
	wg.Wait()

	// 验证所有监听器都收到了断开通知
	if !channelDisconnected {
		t.Error("channelListener did not receive disconnect notification")
	}
	if !msgDisconnected {
		t.Error("msgListener did not receive disconnect notification")
	}

	// 注销监听器
	UnregAuthChannelListener(ModuleAuthChannel)
	UnregAuthChannelListener(ModuleAuthMsg)

	t.Log("Disconnect notification test passed")
}
