package main

import (
	"flag"
	"log"
	"time"

	"github.com/junbin-yang/dsoftbus-go/pkg/authmanager"
)

func main() {
	deviceID := flag.String("id", "client001", "Client device ID")
	deviceName := flag.String("name", "GoClient", "Client device name")
	deviceIP := flag.String("ip", "127.0.0.1", "Client device IP address")
	version := flag.String("version", "1.0.0", "Client version")
	remoteIP := flag.String("remote-ip", "127.0.0.1", "Remote device IP")
	remotePort := flag.Int("remote-port", 0, "Remote device auth port")
	flag.Parse()

	if *remotePort == 0 {
		log.Fatalf("Please specify remote port with -remote-port flag\n")
	}

	log.Printf("Starting Auth Manager Client\n")
	log.Printf("  Device ID: %s\n", *deviceID)
	log.Printf("  Device Name: %s\n", *deviceName)
	log.Printf("  Connecting to: %s:%d\n", *remoteIP, *remotePort)

	// 构建设备信息
	devInfo := &authmanager.DeviceInfo{
		DeviceID:   *deviceID,
		DeviceName: *deviceName,
		DeviceIP:   *deviceIP,
		Version:    *version,
	}

	// 创建认证客户端
	client := authmanager.NewAuthClient(devInfo)
	defer client.Close()

	// 连接远端地址
	if err := client.Connect(*remoteIP, *remotePort); err != nil {
		log.Fatalf("Failed to connect: %v\n", err)
	}

	log.Printf("Connected successfully\n")

	// 步骤1: 发送获取认证信息请求
	log.Printf("\n=== Step 1: Sending GetAuthInfo ===\n")
	if err := client.SendGetAuthInfo(); err != nil {
		log.Fatalf("Failed to send GetAuthInfo: %v\n", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 接收响应
	if err := client.ReceiveResponse(); err != nil {
		log.Fatalf("Failed to receive GetAuthInfo response: %v\n", err)
	}

	// 步骤2: 发送IP验证请求
	log.Printf("\n=== Step 2: Sending VerifyIP ===\n")
	if err := client.SendVerifyIP(8000, 9000); err != nil {
		log.Fatalf("Failed to send VerifyIP: %v\n", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 接收响应
	if err := client.ReceiveResponse(); err != nil {
		log.Fatalf("Failed to receive VerifyIP response: %v\n", err)
	}

	// 步骤3: 发送设备ID验证请求
	log.Printf("\n=== Step 3: Sending VerifyDeviceID ===\n")
	if err := client.SendVerifyDeviceID(); err != nil {
		log.Fatalf("Failed to send VerifyDeviceID: %v\n", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 接收响应
	if err := client.ReceiveResponse(); err != nil {
		log.Fatalf("Failed to receive VerifyDeviceID response: %v\n", err)
	}

	// 打印最终的连接信息
	authConn := client.GetAuthConn()
	log.Printf("\n=== Authentication Complete ===\n")
	log.Printf("Remote Device ID: %s\n", authConn.DeviceID)
	log.Printf("Remote Auth ID: %s\n", authConn.AuthID)
	log.Printf("Remote Auth Port: %d\n", authConn.AuthPort)
	log.Printf("Remote Session Port: %d\n", authConn.SessionPort)
	log.Printf("Online State: %d\n", authConn.OnlineState)

	log.Printf("\nClient completed successfully\n")
}
