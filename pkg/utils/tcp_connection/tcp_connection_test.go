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
	handler := &BaseClientHandler{
		onConnect: func() {
			log.Println("客户端触发 onConnect")
			connected = true
			wg.Done()
		},
		onDisconnect: func() {
			log.Println("客户端触发 onDisconnect")
			disconnected = true
			wg.Done()
		},
		processPackets: func(buf []byte, used int) int {
			log.Printf("客户端触发 processPackets，数据: %s", string(buf[:used]))
			receivedData = make([]byte, used)
			copy(receivedData, buf[:used])
			close(dataProcessed) // 通知测试：数据已处理
			return used
		},
	}
	client := GetBaseClient(handler)

	// 测试连接
	wg.Add(1)
	err = client.Connect(tcpAddr.IP.String(), tcpAddr.Port)
	if err != nil {
		t.Fatalf("客户端连接失败: %v", err)
	}
	wg.Wait() // 等待连接成功
	if !connected {
		t.Fatal("onConnect 未触发")
	}

	// 提前添加断开等待（确保在onDisconnect可能触发前执行）
	wg.Add(1) // 用于等待onDisconnect

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

	// 等待客户端处理响应（必须等processPackets完成）
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
	wg.Wait() // 等待onDisconnect触发
	if !disconnected {
		t.Fatal("onDisconnect 未触发")
	}

	log.Println("测试完成，所有步骤执行正常")
}

// 其他测试用例保持不变...
func TestBaseClient_NoHandler(t *testing.T) {
	client := GetBaseClient(nil)
	err := client.Connect("127.0.0.1", 12345)
	if err == nil {
		t.Error("预期连接失败，但未返回错误")
	}
}

func TestBaseClient_SendWithoutConnect(t *testing.T) {
	client := GetBaseClient(nil)
	err := client.SendBytes([]byte("test"))
	if err == nil || err.Error() != "未建立连接" {
		t.Errorf("预期错误 '未建立连接'，实际: %v", err)
	}
}
