package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/network"
)

func main() {
	var (
		// 设备信息
		devName     = flag.String("name", "GoDevice", "local device name")
		devID       = flag.String("id", "{\"UDID\":\"1234567890\"}", "local device id")
		version     = flag.String("ver", "1.0.0", "local device version")
		devType     = flag.Uint("type", 1, "local device type (0-255)")
		mode        = flag.Uint("mode", 1, "request mode")
		devHash     = flag.String("hash", "0", "device hash")
		serviceData = flag.String("svc", "", "service data")
		capsStr     = flag.String("caps", "192", "capability bitmap, comma-separated, e.g. 1,2,3")
		iface       = flag.String("iface", "eth0", "eth0、wlan0")
	)
	flag.Parse()

	log.SetLevel(log.DebugLevel)

	mgr, err := network.NewManager()
	if err != nil {
		log.Fatalf("网络管理器初始化失败: %v", err)
	}
	defer mgr.Stop()
	err = mgr.Start()
	if err != nil {
		log.Errorf("启动网络监控失败: %v", err)
		return
	}
	ifaceInfo, err := mgr.GetInterface(*iface)
	if err != nil {
		log.Errorf("获取网络接口失败: %v", err)
		return
	}

	// 注册提供者
	caps := parseCaps(*capsStr)
	defaultInterface, _ := mgr.GetDefaultInterface()
	localIp := defaultInterface.Addresses[0].String()
	localIpMask := defaultInterface.Masks[0]
	if len(ifaceInfo.Addresses) > 0 {
		localIp = ifaceInfo.Addresses[0].String()
		localIpMask = ifaceInfo.Masks[0]
	}
	registerProviders(localIp, *devName, *devID, *version, *devType, *mode, *devHash, *serviceData, caps)

	// 启动监听
	if coap.CoapInitDiscovery() != 0 {
		log.Error("failed to init discovery listener")
		os.Exit(1)
	}
	log.Infof("CoAP discovery listener started on UDP port %d", coap.COAP_DEFAULT_PORT)

	// 常驻：从标准输入读命令
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("commands: 'discover' | 'help' | 'quit'")
	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case line == "help":
			fmt.Println("discover                - send discovery to all broadcast addresses")
			fmt.Println("quit                    - exit program")
		case line == "discover":
			broadcast, ok := network.CalculateIPv4Broadcast(net.ParseIP(localIp), localIpMask)
			if !ok {
				fmt.Fprintln(os.Stderr, "calculate broadcast addr failed")
			}
			if err := sendDiscover(broadcast.String()); err != nil {
				fmt.Fprintln(os.Stderr, "broadcast discover failed:", err)
			} else {
				fmt.Println("broadcast discover sent")
			}
		case line == "quit":
			fmt.Println("exiting...")
			return
		default:
			fmt.Println("unknown command, type 'help'")
		}
	}
}

func sendDiscover(subnetIP string) error {
	buf, err := coap.BuildDiscoverPacket(subnetIP)
	if err != nil {
		return err
	}
	dst := &net.UDPAddr{IP: net.ParseIP(subnetIP), Port: coap.COAP_DEFAULT_PORT}
	cli, err := coap.CoapCreateUDPClient(dst)
	if err != nil {
		return err
	}
	defer coap.CoapCloseSocket(cli)
	if _, err = coap.CoapSocketSend(cli, buf); err != nil {
		return err
	}
	return nil
}

func parseCaps(s string) []uint {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]uint, 0, len(parts))
	for _, p := range parts {
		var v uint
		_, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &v)
		if err == nil {
			out = append(out, v)
		}
	}
	return out
}

func registerProviders(localIp, devName, devID, version string, devType uint, mode uint, devHash, serviceData string, caps []uint) {
	deviceInfoProvider := func() *coap.DeviceInfo {
		// 将[]uint转[]uint16
		c := make([]uint16, len(caps))
		for i, v := range caps {
			c[i] = uint16(v)
		}
		return &coap.DeviceInfo{
			DeviceId:         devID,
			DeviceName:       devName,
			DeviceType:       uint8(devType),
			Version:          version,
			Mode:             uint8(mode),
			DeviceHash:       devHash,
			ServiceData:      serviceData,
			CapabilityBitmap: c,
		}
	}
	ipProvider := func() (string, error) {
		return localIp, nil
	}
	discoverHandler := func(dev *coap.DeviceInfo) {
		fmt.Printf("发现新设备：%+v\n", dev)
	}

	coap.RegisterProviders(coap.Providers{
		LocalDeviceInfo: deviceInfoProvider,
		LocalIPString:   ipProvider,
		Discover:        discoverHandler,
	})
}
