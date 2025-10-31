package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/junbin-yang/dsoftbus-go/pkg/authmanager"
)

func main() {
	deviceID := flag.String("id", "device001", "Device ID")
	deviceName := flag.String("name", "GoDevice", "Device name")
	deviceIP := flag.String("ip", "127.0.0.1", "Device IP address")
	version := flag.String("version", "1.0.0", "Device version")
	flag.Parse()

	log.Printf("Starting Auth Manager Server\n")
	log.Printf("  Device ID: %s\n", *deviceID)
	log.Printf("  Device Name: %s\n", *deviceName)
	log.Printf("  Device IP: %s\n", *deviceIP)
	log.Printf("  Version: %s\n", *version)

	// 构建设备信息
	devInfo := &authmanager.DeviceInfo{
		DeviceID:   *deviceID,
		DeviceName: *deviceName,
		DeviceIP:   *deviceIP,
		Version:    *version,
		DevicePort: -1,
	}

	// 创建并启动认证管理服务
	busMgr := authmanager.NewBusManager(devInfo)
	if err := busMgr.Start(); err != nil {
		log.Fatalf("Failed to start bus manager: %v\n", err)
	}

	log.Printf("Auth Manager Server started successfully\n")
	log.Printf("Auth Port: %d\n", devInfo.DevicePort)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("Shutting down...\n")
	if err := busMgr.Stop(); err != nil {
		log.Printf("Error stopping bus manager: %v\n", err)
	}
	log.Printf("Server stopped\n")
}
