package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
)

const (
	DEFAULT_MODULE_NAME = "com.huawei.demo"
	DEFAULT_PUBLISH_ID  = 10010
)

func main() {
	if err := service.InitService(); err != nil {
		fmt.Println(err)
		return
	}

	// 常驻：从标准输入读命令
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("commands: 'publishService' | 'unPublishService' | 'setCommonDeviceInfo' | 'help' | 'quit'")
	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case line == "help":
			fmt.Println("publishService          - publish service to discovered devices")
			fmt.Println("unPublishService        - un-publish service from discovered devices")
			fmt.Println("setCommonDeviceInfo     - set common device info")
			fmt.Println("quit                    - exit program")
		case line == "publishService":
			serInfo := service.PublishInfo{
				PublishId:      DEFAULT_PUBLISH_ID,
				Mode:           service.DiscoverModePassive,
				Medium:         service.ExchangeMediumCOAP,
				Freq:           service.ExchangeFreqLow,
				Capability:     "ddmpCapability",
				CapabilityData: []byte("CapabilityData111"),
			}
			_, err := service.PublishService(DEFAULT_MODULE_NAME, &serInfo)
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println("publishService success")
			}
		case line == "unPublishService":
			_, err := service.UnPublishService(DEFAULT_MODULE_NAME, DEFAULT_PUBLISH_ID)
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println("unPublishService success")
			}
		case line == "setCommonDeviceInfo":
			var devInfo []service.CommonDeviceInfo = []service.CommonDeviceInfo{
				{ // 修改设备标识
					Key:   service.CommonDeviceKeyDevID,
					Value: "88888888",
				},
				{ // 修改设备名称
					Key:   service.CommonDeviceKeyDevName,
					Value: "GODevice",
				},
			}
			_, err := service.SetCommonDeviceInfo(devInfo)
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println("setCommonDeviceInfo success")
			}
		case line == "quit":
			fmt.Println("exiting...")
			return
		default:
			fmt.Println("unknown command, type 'help'")
		}
	}
}
