package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/junbin-yang/dsoftbus-go/pkg/authentication"
	"github.com/junbin-yang/dsoftbus-go/pkg/bus_center"
	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
	"github.com/junbin-yang/dsoftbus-go/pkg/frame"
	"github.com/junbin-yang/dsoftbus-go/pkg/transmission"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/network"
)

type CmdFunc func()

type CmdNode struct {
	Desc string
	Func CmdFunc
}

var cmdList = []CmdNode{
	{"PublishService", publishService},
	{"UnPublishService", unPublishService},
	{"StartDiscovery", startDiscovery},
	{"StopDiscovery", stopDiscovery},
	{"JoinLNN", joinLNN},
	{"LeaveLNN", leaveLNN},
	{"GetLocalDeviceInfo", getLocalDeviceInfo},
	{"GetOnlineDeviceInfo", getOnlineDeviceInfo},
	{"CreateSessionServer", createSessionServer},
	{"OpenSession", openSession},
	{"SendBytes", sendBytes},
	{"Exit", exitTool},
}

var (
	currentSessionID int32 = -1
)

type deviceInfoProvider struct {
	UDID       string
	UUID       string
	DeviceName string
	DeviceType string
}

func (p *deviceInfoProvider) GetDeviceInfo() (*authentication.DeviceInfo, error) {
	return &authentication.DeviceInfo{
		UDID:       p.UDID,
		UUID:       p.UUID,
		DeviceName: p.DeviceName,
		DeviceType: p.DeviceType,
		Version:    authentication.SoftBusVersion{Major: 1, Minor: 0},
	}, nil
}

func (p *deviceInfoProvider) GetUDID() (string, error) {
	return p.UDID, nil
}

func (p *deviceInfoProvider) GetUUID() (string, error) {
	return p.UUID, nil
}

func main() {
	// 初始化软总线框架
	if err := frame.InitSoftBusServer(); err != nil {
		fmt.Printf("初始化软总线框架失败: %v\n", err)
		os.Exit(1)
	}
	defer frame.DeinitSoftBusServer()

	// 注册设备信息提供者（从Bus Center获取）
	bc := bus_center.GetInstance()
	bcDevInfo := bc.GetLocalDeviceInfo()
	provider := &deviceInfoProvider{
		UDID:       bcDevInfo.UDID,
		UUID:       bcDevInfo.UUID,
		DeviceName: bcDevInfo.DeviceName,
		DeviceType: bcDevInfo.DeviceType,
	}
	authentication.RegisterDeviceInfoProvider(provider)

	logger.Info("[Tool] 软总线工具已启动")

	// 主循环
	for {
		helper()
		index := getInputNumber("Please input cmd index:")
		if index < 0 || index >= len(cmdList) {
			fmt.Printf("invalid cmd:%d.\n", index)
			continue
		}

		fmt.Printf("\nExecute: %s\n", cmdList[index].Desc)
		cmdList[index].Func()

		// Exit 是最后一个命令
		if index == len(cmdList)-1 {
			break
		}
	}
}

func helper() {
	fmt.Println("******Softbus Tool Command List******")
	for i, cmd := range cmdList {
		fmt.Printf("*     %02d - %-20s     *\n", i, cmd.Desc)
	}
	fmt.Println("*************************************")
}

func getInputNumber(prompt string) int {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	num, err := strconv.Atoi(input)
	if err != nil {
		return -1
	}
	return num
}

func getInputString(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// Discovery 命令
func publishService() {
	publishId := getInputNumber("Please input publish id:")
	capability := getInputNumber("Please input publish capability(0-hicall 1-profile 2-castPlus 3-dvKit 4-ddmpCapability):")

	capMap := []string{"hicall", "profile", "castPlus", "dvKit", "ddmpCapability"}
	if capability < 0 || capability >= len(capMap) {
		capability = 3 // default dvKit
	}

	publishInfo := &service.PublishInfo{
		PublishId:  publishId,
		Mode:       service.DiscoverModeActive,
		Medium:     service.ExchangeMediumCOAP,
		Capability: capMap[capability],
	}

	_, err := service.PublishService("softbus_tool", publishInfo)
	if err != nil {
		fmt.Printf("PublishService fail: %v\n", err)
		return
	}
	fmt.Println("PublishService success")
}

func unPublishService() {
	publishId := getInputNumber("Please input publish id:")
	_, err := service.UnPublishService("softbus_tool", publishId)
	if err != nil {
		fmt.Printf("UnPublishService fail: %v\n", err)
		return
	}
	fmt.Println("UnPublishService success")
}

func startDiscovery() {
	// 获取本地网络信息
	localIP, localMask, err := service.GetLocalNetworkInfo()
	if err != nil {
		fmt.Printf("获取本地网络信息失败: %v\n", err)
		return
	}

	// 计算广播地址
	broadcast, ok := network.CalculateIPv4Broadcast(localIP, localMask)
	if !ok {
		fmt.Println("计算广播地址失败")
		return
	}

	// 构建发现数据包
	packet, err := coap.BuildDiscoverPacket(broadcast.String())
	if err != nil {
		fmt.Printf("构建发现数据包失败: %v\n", err)
		return
	}

	// 创建UDP客户端
	dst := &net.UDPAddr{
		IP:   broadcast,
		Port: coap.COAP_DEFAULT_PORT,
	}
	client, err := coap.CoapCreateUDPClient(dst)
	if err != nil {
		fmt.Printf("创建UDP客户端失败: %v\n", err)
		return
	}
	defer coap.CoapCloseSocket(client)

	// 发送发现数据包
	if _, err := coap.CoapSocketSend(client, packet); err != nil {
		fmt.Printf("发送发现数据包失败: %v\n", err)
		return
	}

	fmt.Println("StartDiscovery success - 设备发现请求已发送")
}

func stopDiscovery() {
	fmt.Println("StopDiscovery - TODO: implement")
}

// Bus Center 命令
func joinLNN() {
	ip := getInputString("Please input ip:")
	port := getInputNumber("Please input port:")

	bc := bus_center.GetInstance()
	err := bc.JoinLNN("softbus_tool", ip, port, func(networkId string, retCode int32) {
		if retCode == 0 {
			fmt.Printf(">>>OnJoinLNNResult networkId = %s, retCode = %d (success)\n", networkId, retCode)
		} else {
			fmt.Printf(">>>OnJoinLNNResult networkId = %s, retCode = %d (failed)\n", networkId, retCode)
		}
	})

	if err != nil {
		fmt.Printf("JoinLNN fail: %v\n", err)
	}
}

func leaveLNN() {
	networkId := getInputString("Please input network Id:")

	bc := bus_center.GetInstance()
	err := bc.LeaveLNN("softbus_tool", networkId, func(networkId string, retCode int32) {
		fmt.Printf(">>>OnLeaveLNNDone networkId = %s, retCode = %d\n", networkId, retCode)
	})

	if err != nil {
		fmt.Printf("LeaveLNN fail: %v\n", err)
	} else {
		fmt.Println("LeaveLNN success")
	}
}

func getLocalDeviceInfo() {
	bc := bus_center.GetInstance()
	localInfo := bc.GetLocalDeviceInfo()
	if localInfo == nil {
		fmt.Println("Local device info not available")
		return
	}

	fmt.Println(">>>Local Device Info:")
	fmt.Printf("  DeviceName: %s\n", localInfo.DeviceName)
	fmt.Printf("  DeviceID: %s\n", localInfo.DeviceID)
	fmt.Printf("  UDID: %s\n", localInfo.UDID)
	fmt.Printf("  UUID: %s\n", localInfo.UUID)
	fmt.Printf("  DeviceType: %s\n", localInfo.DeviceType)
	fmt.Printf("  AuthPort: %d\n", localInfo.AuthPort)
}

func getOnlineDeviceInfo() {
	bc := bus_center.GetInstance()
	nodes := bc.GetOnlineNodes()

	fmt.Printf(">>>Online Device Count: %d\n", len(nodes))
	for i, node := range nodes {
		fmt.Printf("\n[Device %d]\n", i+1)
		fmt.Printf("  DeviceName: %s\n", node.DeviceName)
		fmt.Printf("  NetworkID: %s\n", node.NetworkID)
		fmt.Printf("  DeviceID: %s\n", node.DeviceID)
		fmt.Printf("  Status: %v\n", node.Status)
		fmt.Printf("  ConnectAddr: %s\n", node.ConnectAddr)
		fmt.Printf("  DiscoveryType: %s\n", node.DiscoveryType)
	}
}

// Session 命令
func createSessionServer() {
	sessionName := getInputString("Please input session name:")

	listener := &transmission.SessionServer{
		OnBind: func(sessionID int32) {
			fmt.Printf(">>>OnSessionOpened sessionID = %d\n", sessionID)
			currentSessionID = sessionID
		},
		OnShutdown: func(sessionID int32) {
			fmt.Printf(">>>OnSessionClosed sessionID = %d\n", sessionID)
		},
		OnBytes: func(sessionID int32, data []byte) {
			fmt.Printf(">>>OnBytesReceived sessionID = %d, len = %d, data = %s\n", sessionID, len(data), string(data))
		},
		OnMessage: func(sessionID int32, data []byte) {
			fmt.Printf(">>>OnMessageReceived sessionID = %d, len = %d, data = %s\n", sessionID, len(data), string(data))
		},
	}

	err := transmission.CreateSessionServer("softbus_tool", sessionName, listener)
	if err != nil {
		fmt.Printf("CreateSessionServer fail: %v\n", err)
		return
	}
	fmt.Println("CreateSessionServer success")
}

func openSession() {
	sessionName := getInputString("Please input session name:")
	peerName := getInputString("Please input peer session name:")
	networkID := getInputString("Please input peer network ID:")

	// 从 Bus Center 获取节点信息
	bc := bus_center.GetInstance()
	node := bc.GetNodeInfo(networkID)
	if node == nil {
		fmt.Printf("OpenSession fail: device not found (networkID=%s)\n", networkID)
		return
	}

	// 使用节点的 AuthSeq 作为 authID
	authID := node.AuthSeq

	sessionID, err := transmission.OpenSession(sessionName, peerName, networkID, authID)
	if err != nil {
		fmt.Printf("OpenSession fail: %v\n", err)
		return
	}

	currentSessionID = sessionID
	fmt.Printf("OpenSession success, sessionID = %d\n", sessionID)
}

func sendBytes() {
	if currentSessionID == -1 {
		fmt.Println("No active session. Please OpenSession first.")
		return
	}

	data := getInputString("Please input data to send:")

	err := transmission.SendBytes(currentSessionID, []byte(data))
	if err != nil {
		fmt.Printf("SendBytes fail: %v\n", err)
		return
	}
	fmt.Printf("SendBytes success, sent %d bytes\n", len(data))
}

func exitTool() {
	fmt.Println("BYE!")
}
