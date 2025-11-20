package tcp_connection

import (
	"log"
	"net"
	"sync"
	"testing"
	"time"
)

// 启用测试日志，方便定位问题
func init() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
}

// 测试服务器：等待客户端确认处理完数据后再关闭
func startTestServer(t *testing.T, dataChan chan []byte, doneChan chan string, serverDoneChan chan struct{}) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("服务器启动失败: %v", err)
	}
	defer listener.Close()

	// 发送服务器地址
	doneChan <- listener.Addr().String()

	conn, err := listener.Accept()
	if err != nil {
		select {
		case <-serverDoneChan:
			return
		default:
			t.Errorf("接受连接失败: %v", err)
			return
		}
	}
	defer conn.Close()

	// 读取客户端数据
	buf := make([]byte, DefaultBufSize)
	n, err := conn.Read(buf)
	if err != nil {
		t.Errorf("服务器读数据失败: %v", err)
		return
	}
	dataChan <- buf[:n]

	// 发送响应
	_, err = conn.Write([]byte("server response"))
	if err != nil {
		t.Errorf("服务器发响应失败: %v", err)
		return
	}

	// 等待客户端处理完毕的信号（阻塞直到收到）
	<-serverDoneChan
	log.Println("服务器收到关闭信号，关闭连接")
}

// 测试客户端完整流程
func TestBaseClient_CompleteFlow(t *testing.T) {
	dataChan := make(chan []byte, 1)
	doneChan := make(chan string, 1)
	serverDoneChan := make(chan struct{}) // 控制服务器关闭的信号
	var wg sync.WaitGroup

	// 启动测试服务器
	go startTestServer(t, dataChan, doneChan, serverDoneChan)
	serverAddr := <-doneChan
	tcpAddr, err := net.ResolveTCPAddr("tcp", serverAddr)
	if err != nil {
		t.Fatalf("解析服务器地址失败: %v", err)
	}
	log.Printf("测试服务器地址: %s", tcpAddr.String())

	var (
		connected     bool
		disconnected  bool
		receivedData  []byte
		dataProcessed = make(chan struct{}) // 标记数据已处理的信号
	)

	// 使用新的统一回调
	callback := &BaseListenerCallback{
		OnConnected: func(fd int, connType ConnectionType, connOpt *ConnectOption) {
			log.Printf("客户端触发 OnConnected (fd=%d, type=%v)", fd, connType)
			connected = true
			wg.Done()
		},
		OnDisconnected: func(fd int, connType ConnectionType) {
			log.Printf("客户端触发 OnDisconnected (fd=%d, type=%v)", fd, connType)
			disconnected = true
			wg.Done()
		},
		OnDataReceived: func(fd int, connType ConnectionType, buf []byte, used int) int {
			log.Printf("客户端触发 OnDataReceived (fd=%d, type=%v)，数据: %s", fd, connType, string(buf[:used]))
			receivedData = make([]byte, used)
			copy(receivedData, buf[:used])
			close(dataProcessed) // 通知测试：数据已处理
			return used
		},
	}

	// 使用新的API创建客户端
	client := NewBaseClient(nil, callback)

	// 测试连接
	wg.Add(1)
	fd, err := client.ConnectSimple(tcpAddr.IP.String(), tcpAddr.Port)
	if err != nil {
		t.Fatalf("客户端连接失败: %v", err)
	}
	log.Printf("客户端连接成功，分配的虚拟fd=%d", fd)

	// 验证虚拟fd从2000开始
	if fd < ClientFdStart {
		t.Fatalf("客户端虚拟fd应该从%d开始，实际为%d", ClientFdStart, fd)
	}

	wg.Wait() // 等待连接成功
	if !connected {
		t.Fatal("OnConnected 未触发")
	}

	// 提前添加断开等待（确保在OnDisconnected可能触发前执行）
	wg.Add(1) // 用于等待OnDisconnected

	// 测试发送数据
	err = client.SendBytes([]byte("client request"))
	if err != nil {
		t.Fatalf("客户端发送数据失败: %v", err)
	}
	log.Println("客户端已发送数据: client request")

	// 验证服务器收到数据
	select {
	case data := <-dataChan:
		if string(data) != "client request" {
			t.Fatalf("服务器收到数据不匹配: 预期 %q, 实际 %q", "client request", data)
		}
		log.Println("服务器已收到客户端数据")
	case <-time.After(2 * time.Second):
		t.Fatal("服务器超时未收到客户端数据")
	}

	// 等待客户端处理响应（必须等OnDataReceived完成）
	select {
	case <-dataProcessed:
		log.Println("客户端已处理响应数据")
	case <-time.After(2 * time.Second):
		t.Fatal("客户端超时未处理响应数据")
	}

	// 确认数据处理正确
	if string(receivedData) != "server response" {
		t.Fatalf("客户端收到数据不匹配: 预期 %q, 实际 %q", "server response", receivedData)
	}

	// 通知服务器可以关闭连接（此时客户端已处理完数据）
	close(serverDoneChan)
	time.Sleep(200 * time.Millisecond) // 确保服务器收到信号

	// 测试断开连接
	client.Close()
	wg.Wait() // 等待OnDisconnected触发
	if !disconnected {
		t.Fatal("OnDisconnected 未触发")
	}

	log.Println("测试完成，所有步骤执行正常")
}

// 测试客户端带选项的连接
func TestBaseClient_ConnectWithOptions(t *testing.T) {
	callback := &BaseListenerCallback{
		OnConnected: func(fd int, connType ConnectionType, connOpt *ConnectOption) {
			log.Printf("OnConnected (fd=%d, type=%v)", fd, connType)
		},
		OnDisconnected: func(fd int, connType ConnectionType) {
			log.Printf("OnDisconnected (fd=%d, type=%v)", fd, connType)
		},
		OnDataReceived: func(fd int, connType ConnectionType, buf []byte, used int) int {
			return used
		},
	}

	client := NewBaseClient(nil, callback)

	// 使用自定义选项连接（这里会连接失败，但能测试API）
	opt := &ClientOption{
		RemoteIP:        "127.0.0.1",
		RemotePort:      9999, // 不存在的端口
		Timeout:         1 * time.Second,
		KeepAlive:       true,
		KeepAlivePeriod: 30 * time.Second,
	}

	fd, err := client.Connect(opt)
	if err == nil {
		t.Error("预期连接失败，但实际成功了")
		client.Close()
	} else {
		log.Printf("预期的连接失败: %v", err)
	}

	if fd != -1 {
		t.Errorf("连接失败时应返回fd=-1，实际为%d", fd)
	}
}

// 测试无回调
func TestBaseClient_NoCallback(t *testing.T) {
	client := NewBaseClient(nil, nil)
	fd, err := client.ConnectSimple("127.0.0.1", 12345)
	if err == nil {
		t.Error("预期连接失败，但未返回错误")
		client.Close()
	}
	if fd != -1 {
		t.Errorf("连接失败时应返回fd=-1，实际为%d", fd)
	}
}

// 测试未连接时发送
func TestBaseClient_SendWithoutConnect(t *testing.T) {
	callback := &BaseListenerCallback{}
	client := NewBaseClient(nil, callback)
	err := client.SendBytes([]byte("test"))
	if err == nil || err.Error() != "未建立连接" {
		t.Errorf("预期错误 '未建立连接'，实际: %v", err)
	}
}

// 测试虚拟fd管理
func TestConnectionManager_FdAllocation(t *testing.T) {
	mgr := NewConnectionManager()

	// 测试服务端fd从1000开始
	serverFd1 := mgr.AllocateFd(ConnectionTypeServer)
	serverFd2 := mgr.AllocateFd(ConnectionTypeServer)

	if serverFd1 != ServerFdStart {
		t.Errorf("服务端第一个fd应该是%d，实际为%d", ServerFdStart, serverFd1)
	}
	if serverFd2 != ServerFdStart+1 {
		t.Errorf("服务端第二个fd应该是%d，实际为%d", ServerFdStart+1, serverFd2)
	}

	// 测试客户端fd从2000开始
	clientFd1 := mgr.AllocateFd(ConnectionTypeClient)
	clientFd2 := mgr.AllocateFd(ConnectionTypeClient)

	if clientFd1 != ClientFdStart {
		t.Errorf("客户端第一个fd应该是%d，实际为%d", ClientFdStart, clientFd1)
	}
	if clientFd2 != ClientFdStart+1 {
		t.Errorf("客户端第二个fd应该是%d，实际为%d", ClientFdStart+1, clientFd2)
	}

	log.Println("虚拟fd分配测试通过")
}

// 测试服务端和客户端共用ConnectionManager
func TestSharedConnectionManager(t *testing.T) {
	// 创建共享的连接管理器
	sharedMgr := NewConnectionManager()

	// 创建服务端
	serverCallback := &BaseListenerCallback{
		OnConnected: func(fd int, connType ConnectionType, connOpt *ConnectOption) {
			log.Printf("[SERVER] OnConnected (fd=%d, type=%v)", fd, connType)
		},
		OnDisconnected: func(fd int, connType ConnectionType) {
			log.Printf("[SERVER] OnDisconnected (fd=%d, type=%v)", fd, connType)
		},
		OnDataReceived: func(fd int, connType ConnectionType, buf []byte, used int) int {
			log.Printf("[SERVER] OnDataReceived (fd=%d, type=%v): %s", fd, connType, string(buf[:used]))
			// 回显数据
			sharedMgr.SendBytes(fd, buf[:used])
			return used
		},
	}

	server := NewBaseServer(sharedMgr)
	err := server.StartBaseListener(&SocketOption{Addr: "127.0.0.1", Port: 0}, serverCallback)
	if err != nil {
		t.Fatalf("启动服务器失败: %v", err)
	}
	defer server.StopBaseListener()

	serverPort := server.GetPort()
	log.Printf("服务器启动在端口: %d", serverPort)

	// 创建客户端（共用同一个管理器）
	var clientFd int
	clientCallback := &BaseListenerCallback{
		OnConnected: func(fd int, connType ConnectionType, connOpt *ConnectOption) {
			log.Printf("[CLIENT] OnConnected (fd=%d, type=%v)", fd, connType)
			clientFd = fd
		},
		OnDisconnected: func(fd int, connType ConnectionType) {
			log.Printf("[CLIENT] OnDisconnected (fd=%d, type=%v)", fd, connType)
		},
		OnDataReceived: func(fd int, connType ConnectionType, buf []byte, used int) int {
			log.Printf("[CLIENT] OnDataReceived (fd=%d, type=%v): %s", fd, connType, string(buf[:used]))
			return used
		},
	}

	client := NewBaseClient(sharedMgr, clientCallback)
	fd, err := client.ConnectSimple("127.0.0.1", serverPort)
	if err != nil {
		t.Fatalf("客户端连接失败: %v", err)
	}
	defer client.Close()

	log.Printf("客户端连接成功 (fd=%d)", fd)

	// 等待连接建立
	time.Sleep(100 * time.Millisecond)

	// 验证服务端和客户端的fd在不同范围
	serverFds := []int{}
	clientFds := []int{fd}

	for _, testFd := range sharedMgr.GetAllFds() {
		connType, ok := sharedMgr.GetConnType(testFd)
		if !ok {
			continue
		}
		if connType == ConnectionTypeServer {
			serverFds = append(serverFds, testFd)
		}
	}

	if len(serverFds) == 0 {
		t.Fatal("未找到服务端连接")
	}

	for _, sfd := range serverFds {
		if sfd < ServerFdStart {
			t.Errorf("服务端fd=%d 应该 >= %d", sfd, ServerFdStart)
		}
	}

	for _, cfd := range clientFds {
		if cfd < ClientFdStart {
			t.Errorf("客户端fd=%d 应该 >= %d", cfd, ClientFdStart)
		}
	}

	// 测试通过共享管理器发送数据
	testData := []byte("test message")
	err = sharedMgr.SendBytes(clientFd, testData)
	if err != nil {
		t.Fatalf("发送数据失败: %v", err)
	}

	// 等待数据处理
	time.Sleep(100 * time.Millisecond)

	log.Printf("共享连接管理器测试通过，总连接数: %d", sharedMgr.GetConnCount())
}
