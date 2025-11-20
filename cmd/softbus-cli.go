package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/junbin-yang/dsoftbus-go/pkg/authentication"
	"github.com/junbin-yang/dsoftbus-go/pkg/bus_center"
	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
	"github.com/junbin-yang/dsoftbus-go/pkg/frame"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/network"
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

// CLI 命令行工具结构
type CLI struct {
	devices      map[string]*coap.DeviceInfo // 发现的设备列表
	authManagers map[int64]*AuthSession      // 认证会话管理 (authId -> session)
	authPort     int                         // 认证服务监听端口
}

// AuthSession 认证会话信息
type AuthSession struct {
	AuthId        int64
	DeviceId      string
	DeviceName    string
	IP            string
	Port          int
	Authenticated bool
}

// NewCLI 创建CLI实例
func NewCLI() *CLI {
	return &CLI{
		devices:      make(map[string]*coap.DeviceInfo),
		authManagers: make(map[int64]*AuthSession),
	}
}

// Initialize 初始化CLI
func (c *CLI) Initialize() error {
	logger.Info("[CLI] 正在初始化...")

	// 使用frame统一初始化接口
	if err := frame.InitSoftBusServer(); err != nil {
		return fmt.Errorf("初始化软总线框架失败: %v", err)
	}
	logger.Info("[CLI] 软总线框架已启动")

	// 获取本地设备信息
	localDevInfo := service.DiscCoapGetDeviceInfo()
	logger.Infof("[CLI] 设备信息：ID=%s, Name=%s, Type=%d", localDevInfo.DeviceId, localDevInfo.Name, localDevInfo.DeviceType)

	// 设置本地设备信息到Bus Center
	busCenter := bus_center.GetInstance()
	hash := sha256.Sum256([]byte(localDevInfo.DeviceId))
	hashStr := hex.EncodeToString(hash[:])
	busCenter.SetLocalDeviceInfo(&bus_center.LocalDeviceInfo{
		UDID:       localDevInfo.DeviceId,
		UUID:       hashStr,
		DeviceID:   localDevInfo.DeviceId,
		DeviceName: localDevInfo.Name,
		DeviceType: "PC",
	})

	// 注册设备信息提供者（从Bus Center获取）
	bcDevInfo := busCenter.GetLocalDeviceInfo()
	provider := &deviceInfoProvider{
		UDID:       bcDevInfo.UDID,
		UUID:       bcDevInfo.UUID,
		DeviceName: bcDevInfo.DeviceName,
		DeviceType: bcDevInfo.DeviceType,
	}
	authentication.RegisterDeviceInfoProvider(provider)

	// 注册Bus Center回调
	busCenter.RegisterEventCallback(c.onBusCenterEvent)
	busCenter.RegisterAuthCallback(bus_center.AuthCallback{
		OnAuthSuccess: c.onAuthSuccess,
		OnAuthFailed:  c.onAuthFailed,
	})

	// 注册认证回调（转发到Bus Center）
	authCallback := &authentication.AuthConnCallback{
		OnConnOpened: func(requestId uint32, authId int64) {
			c.onAuthOpened(requestId, authId)
		},
		OnConnOpenFailed: func(requestId uint32, reason int32) {
			busCenter.NotifyAuthFailed(requestId, reason)
		},
		OnDataReceived: func(authId int64, head *authentication.AuthDataHead, data []byte) {
			c.onAuthDataReceived(authId, head, data)
		},
	}

	if err := authentication.AuthDeviceInit(authCallback); err != nil {
		return fmt.Errorf("初始化认证回调失败: %v", err)
	}

	// 设置设备发现回调
	service.SetDiscoverCallback(c.onDeviceFound)

	// 启动认证TCP服务器监听
	authPort, err := authentication.StartSocketListening(authentication.Auth, "0.0.0.0", 0)
	if err != nil {
		return fmt.Errorf("启动认证TCP监听失败: %v", err)
	}
	c.authPort = authPort

	// 更新认证端口到Bus Center和Discovery
	busCenter.UpdateAuthPort(authPort)
	service.UpdateAuthPortToCoapService(authPort)
	logger.Infof("[CLI] 认证端口: %d", authPort)

	logger.Info("[CLI] 初始化完成")

	return nil
}

// Shutdown 关闭CLI
func (c *CLI) Shutdown() {
	logger.Info("[CLI] 正在关闭...")

	// 停止认证TCP监听
	authentication.StopSocketListening()
	logger.Info("[CLI] 认证TCP监听已停止")

	// 使用frame统一反初始化接口
	frame.DeinitSoftBusServer()
	logger.Info("[CLI] 软总线框架已关闭")

	logger.Info("[CLI] 已关闭")
}

// onAuthOpened 认证连接成功回调
func (c *CLI) onAuthOpened(requestId uint32, authId int64) {
	logger.Infof("[CLI] 认证连接成功: requestId=%d, authId=%d", requestId, authId)

	// 查找对应的设备信息
	var deviceId, deviceName, ip string
	var port int

	for id, device := range c.devices {
		// 这里需要通过requestId匹配，实际应该在ConnectDevice时记录
		deviceId = id
		deviceName = device.DeviceName
		ip = device.NetChannelInfo.Network.IP.String()
		port, _ = parsePortFromServiceData(device.ServiceData)
		break
	}

	// 保存认证会话
	session := &AuthSession{
		AuthId:        authId,
		DeviceId:      deviceId,
		DeviceName:    deviceName,
		IP:            ip,
		Port:          port,
		Authenticated: true,
	}
	c.authManagers[authId] = session

	// 通知Bus Center认证成功
	busCenter := bus_center.GetInstance()
	node := &bus_center.NodeInfo{
		NetworkID:     deviceId,
		DeviceID:      deviceId,
		DeviceName:    deviceName,
		Status:        bus_center.StatusOnline,
		AuthSeq:       authId,
		DiscoveryType: "CoAP",
		ConnectAddr:   fmt.Sprintf("%s:%d", ip, port),
	}
	busCenter.NotifyAuthSuccess(requestId, authId, node)
}

// onAuthSuccess Bus Center认证成功回调
func (c *CLI) onAuthSuccess(requestId uint32, authId int64, node *bus_center.NodeInfo) {
	fmt.Printf("\n>>> 认证成功: %s (authId=%d) <<<\n", node.DeviceName, authId)
	fmt.Print("softbus-cli> ")
}

// onAuthFailed Bus Center认证失败回调
func (c *CLI) onAuthFailed(requestId uint32, reason int32) {
	fmt.Printf("\n>>> 认证失败: requestId=%d, reason=%d <<<\n", requestId, reason)
	fmt.Print("softbus-cli> ")
}


// onAuthDataReceived 认证数据接收回调
func (c *CLI) onAuthDataReceived(authId int64, head *authentication.AuthDataHead, data []byte) {
	logger.Infof("[CLI] 收到认证数据: authId=%d, module=%d, seq=%d, len=%d",
		authId, head.Module, head.Seq, len(data))

	session, exists := c.authManagers[authId]
	if !exists {
		logger.Warnf("[CLI] 未找到认证会话: authId=%d", authId)
		return
	}

	fmt.Printf("\n>>> 收到来自 %s 的数据: module=%d, len=%d <<<\n",
		session.DeviceName, head.Module, len(data))
	fmt.Print("softbus-cli> ")
}

// onDeviceFound 设备发现回调
func (c *CLI) onDeviceFound(device *coap.DeviceInfo) {
	// 保存到设备列表
	c.devices[device.DeviceId] = device

	// 打印通知
	fmt.Printf("\n>>> 发现新设备: %s (%s) at %s <<<\n",
		device.DeviceName, device.DeviceId, device.ServiceData)
	fmt.Print("softbus> ")
}

// onBusCenterEvent Bus Center事件回调
func (c *CLI) onBusCenterEvent(event bus_center.LNNEvent, node *bus_center.NodeInfo) {
	switch event {
	case bus_center.EventNodeOnline:
		logger.Infof("[BusCenter] 节点上线: %s (%s)", node.DeviceName, node.NetworkID)
	case bus_center.EventNodeOffline:
		logger.Infof("[BusCenter] 节点下线: %s (%s)", node.DeviceName, node.NetworkID)
	case bus_center.EventNodeInfoChanged:
		logger.Infof("[BusCenter] 节点信息变更: %s (%s)", node.DeviceName, node.NetworkID)
	}
}

// PublishService 发布服务
func (c *CLI) PublishService(serviceName, capability string) error {
	logger.Infof("[CLI] 发布服务: %s, 能力: %s", serviceName, capability)

	publishInfo := &service.PublishInfo{
		PublishId:  1,
		Mode:       service.DiscoverModeActive,
		Medium:     service.ExchangeMediumCOAP,
		Capability: capability,
	}

	publishId, err := service.PublishService("cli", publishInfo)
	if err != nil {
		return fmt.Errorf("发布服务失败: %v", err)
	}

	logger.Infof("[CLI] 服务发布成功，PublishId=%d", publishId)
	fmt.Printf("服务 '%s' 已发布 (能力: %s)\n", serviceName, capability)
	return nil
}

// DiscoverDevices 主动发现设备
func (c *CLI) DiscoverDevices() error {
	logger.Info("[CLI] 开始主动发现设备...")

	// 获取本地网络信息
	localIP, localMask, err := service.GetLocalNetworkInfo()
	if err != nil {
		return fmt.Errorf("获取本地网络信息失败: %v", err)
	}

	// 计算广播地址
	broadcast, ok := network.CalculateIPv4Broadcast(localIP, localMask)
	if !ok {
		return fmt.Errorf("计算广播地址失败: %v", err)
	}
	logger.Infof("[CLI] 向广播地址 %s 发送发现请求", broadcast.String())

	// 构建发现数据包
	packet, err := coap.BuildDiscoverPacket(broadcast.String())
	if err != nil {
		return fmt.Errorf("构建发现数据包失败: %v", err)
	}

	// 创建UDP客户端
	dst := &net.UDPAddr{
		IP:   broadcast,
		Port: coap.COAP_DEFAULT_PORT,
	}
	client, err := coap.CoapCreateUDPClient(dst)
	if err != nil {
		return fmt.Errorf("创建UDP客户端失败: %v", err)
	}
	defer coap.CoapCloseSocket(client)

	// 发送发现数据包
	if _, err := coap.CoapSocketSend(client, packet); err != nil {
		return fmt.Errorf("发送发现数据包失败: %v", err)
	}

	logger.Info("[CLI] 发现请求已发送")
	fmt.Println("✓ 设备发现请求已发送，等待响应...")
	return nil
}

// parsePortFromServiceData 从ServiceData字符串中解析端口号
// ServiceData格式示例: "port:12345"
func parsePortFromServiceData(serviceData string) (int, error) {
	// 查找 "port:" 前缀
	const portPrefix = "port:"
	idx := strings.Index(serviceData, portPrefix)
	if idx == -1 {
		return 0, fmt.Errorf("serviceData中未找到端口信息")
	}

	// 提取端口号字符串
	portStr := serviceData[idx+len(portPrefix):]
	// 可能有其他信息跟在端口号后面，用逗号或空格分隔
	if sepIdx := strings.IndexAny(portStr, ", \t"); sepIdx != -1 {
		portStr = portStr[:sepIdx]
	}

	// 解析端口号
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("解析端口号失败: %v", err)
	}

	if port < 0 || port > 65535 {
		return 0, fmt.Errorf("端口号超出范围: %d", port)
	}

	return port, nil
}

// ListDevices 列出发现的设备
func (c *CLI) ListDevices() {
	fmt.Println("\n=== 已发现的设备 ===")
	if len(c.devices) == 0 {
		fmt.Println("（无）")
		return
	}

	for _, device := range c.devices {
		deviceIP := device.NetChannelInfo.Network.IP.String()
		authPort, err := parsePortFromServiceData(device.ServiceData)
		portStr := "N/A"
		if err == nil {
			portStr = fmt.Sprintf("%d", authPort)
		}

		// 检查是否已认证
		authStatus := "未认证"
		var authId int64 = -1
		for id, session := range c.authManagers {
			if session.DeviceId == device.DeviceId && session.Authenticated {
				authStatus = fmt.Sprintf("已认证 (authId=%d)", id)
				authId = id
				break
			}
		}

		fmt.Printf("  [%s]\n", device.DeviceId)
		fmt.Printf("    名称:     %s\n", device.DeviceName)
		fmt.Printf("    IP:       %s\n", deviceIP)
		fmt.Printf("    认证端口: %s\n", portStr)
		fmt.Printf("    状态:     %s\n", authStatus)
		fmt.Printf("    类型:     %s\n", service.GetDeviceNameByType((service.DeviceType)(device.DeviceType)))
		if authId >= 0 {
			fmt.Printf("    AuthId:   %d\n", authId)
		}
		fmt.Println()
	}
}

// ConnectDevice 连接到指定设备并进行认证
func (c *CLI) ConnectDevice(deviceId string) error {
	device, exists := c.devices[deviceId]
	if !exists {
		return fmt.Errorf("设备不存在: %s", deviceId)
	}

	// 解析认证端口
	authPort, err := parsePortFromServiceData(device.ServiceData)
	if err != nil {
		return fmt.Errorf("无法获取设备认证端口: %v", err)
	}

	deviceIP := device.NetChannelInfo.Network.IP.String()
	logger.Infof("[CLI] 正在连接设备: %s (%s:%d)", device.DeviceName, deviceIP, authPort)

	// 构建认证连接信息
	connInfo := &authentication.AuthConnInfo{
		Type: authentication.AuthLinkTypeWifi,
		Ip:   deviceIP,
		Port: authPort,
		Udid: device.DeviceId,
	}

	// 生成请求ID（使用时间戳）
	requestId := uint32(time.Now().Unix() & 0xFFFFFFFF)

	// 打开认证连接（客户端模式），使用nil表示使用全局回调
	err = authentication.AuthDeviceOpenConn(connInfo, requestId, nil)
	if err != nil {
		return fmt.Errorf("打开认证连接失败: %v", err)
	}

	logger.Infof("[CLI] 认证连接请求已发送: requestId=%d", requestId)
	fmt.Printf("正在连接设备 %s (%s:%d)...\n", device.DeviceName, deviceIP, authPort)
	fmt.Printf("请求ID: %d\n", requestId)

	return nil
}

// SendTestData 向已认证设备发送测试数据
func (c *CLI) SendTestData(authId int64, message string) error {
	session, exists := c.authManagers[authId]
	if !exists {
		return fmt.Errorf("认证会话不存在: authId=%d", authId)
	}

	if !session.Authenticated {
		return fmt.Errorf("设备未认证: %s", session.DeviceName)
	}

	// 发送测试数据（使用MODULE_AUTH_MSG模块）
	err := authentication.AuthDevicePostTransData(
		authId,
		authentication.ModuleAuthMsg, // 使用AUTH_MSG模块
		0,                            // flag
		[]byte(message),
	)

	if err != nil {
		return fmt.Errorf("发送数据失败: %v", err)
	}

	logger.Infof("[CLI] 已发送测试数据到 authId=%d: %s", authId, message)
	fmt.Printf("✓ 已发送数据到 %s (authId=%d)\n", session.DeviceName, authId)

	return nil
}

// InteractiveMode 交互式模式
func (c *CLI) InteractiveMode() {
	localDevInfo := service.DiscCoapGetDeviceInfo()
	fmt.Println("\n===========================================")
	fmt.Println("    鸿蒙软总线命令行工具 (交互模式)")
	fmt.Printf("    设备ID: %s\n", localDevInfo.DeviceId)
	fmt.Printf("    设备名: %s\n", localDevInfo.Name)
	fmt.Println("===========================================")
	fmt.Println("\n输入 'help' 查看可用命令")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\nsoftbus-cli> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmd := parts[0]

		switch cmd {
		case "help", "h":
			c.printHelp()

		case "discover":
			if err := c.DiscoverDevices(); err != nil {
				fmt.Printf("错误: %v\n", err)
			}

		case "publish":
			if len(parts) < 2 {
				fmt.Println("用法: publish <能力>")
				fmt.Println("示例: publish hicall")
				continue
			}
			if err := c.PublishService("service", parts[1]); err != nil {
				fmt.Printf("错误: %v\n", err)
			}

		case "list":
			c.ListDevices()

		case "nodes":
			c.ListNodes()

		case "connect":
			if len(parts) < 2 {
				fmt.Println("用法: connect <设备ID>")
				fmt.Println("示例: connect 1234567890abcdef")
				continue
			}
			if err := c.ConnectDevice(parts[1]); err != nil {
				fmt.Printf("错误: %v\n", err)
			}

		case "send":
			if len(parts) < 3 {
				fmt.Println("用法: send <AuthId> <消息>")
				fmt.Println("示例: send 1001 Hello")
				continue
			}
			authId, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				fmt.Printf("错误: 无效的AuthId: %v\n", err)
				continue
			}
			message := strings.Join(parts[2:], " ")
			if err := c.SendTestData(authId, message); err != nil {
				fmt.Printf("错误: %v\n", err)
			}

		case "exit", "quit", "q":
			fmt.Println("再见！")
			return

		default:
			fmt.Printf("未知命令: %s (输入 'help' 查看帮助)\n", cmd)
		}
	}
}

// ListNodes 列出Bus Center中的节点
func (c *CLI) ListNodes() {
	busCenter := bus_center.GetInstance()
	nodes := busCenter.GetAllNodes()

	if len(nodes) == 0 {
		fmt.Println("暂无节点")
		return
	}

	fmt.Printf("\n%-20s %-20s %-10s %-20s %-20s\n", "NetworkID", "DeviceName", "Status", "ConnectAddr", "JoinTime")
	fmt.Println(strings.Repeat("-", 100))

	for _, node := range nodes {
		status := "离线"
		if node.Status == bus_center.StatusOnline {
			status = "在线"
		}
		fmt.Printf("%-20s %-20s %-10s %-20s %-20s\n",
			node.NetworkID[:16]+"...",
			node.DeviceName,
			status,
			node.ConnectAddr,
			node.JoinTime.Format("2006-01-02 15:04:05"))
	}
	fmt.Println()
}

// printHelp 打印帮助信息
func (c *CLI) printHelp() {
	fmt.Println("\n可用命令:")
	fmt.Println("  help, h                     - 显示此帮助")
	fmt.Println("  discover                    - 主动发现网络中的设备")
	fmt.Println("  publish <能力>              - 发布服务能力")
	fmt.Println("                                可用能力: hicall, profile, castPlus, dvKit, ddmpCapability")
	fmt.Println("  list                        - 列出已发现的设备及认证状态")
	fmt.Println("  nodes                       - 列出Bus Center中的节点")
	fmt.Println("  connect <设备ID>            - 连接到指定设备并进行认证")
	fmt.Println("  send <AuthId> <消息>        - 向已认证设备发送测试消息")
	fmt.Println("  exit, quit, q               - 退出程序")
	fmt.Println()
}

func main() {
	// 创建CLI
	cli := NewCLI()

	// 初始化
	if err := cli.Initialize(); err != nil {
		fmt.Printf("初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 设置信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 启动信号处理goroutine
	go func() {
		<-sigCh
		fmt.Println("\n收到中断信号，正在关闭...")
		cli.Shutdown()
		os.Exit(0)
	}()

	// 延迟关闭
	defer cli.Shutdown()

	// 启动交互式模式
	cli.InteractiveMode()
}
