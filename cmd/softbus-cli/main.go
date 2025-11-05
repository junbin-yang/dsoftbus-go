package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/junbin-yang/dsoftbus-go/pkg/authmanager"
	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
	"github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/network"
)

// CLI 命令行工具结构
type CLI struct {
	busManager       *authmanager.BusManager
	devices          map[string]*coap.DeviceInfo      // 发现的设备列表
	authenticatedDev map[string]*authmanager.AuthConn // 已认证的设备信息（包含会话端口）
}

// NewCLI 创建CLI实例
func NewCLI() *CLI {
	return &CLI{
		devices:          make(map[string]*coap.DeviceInfo),
		authenticatedDev: make(map[string]*authmanager.AuthConn),
	}
}

// Initialize 初始化CLI
func (c *CLI) Initialize() error {
	logger.Info("[CLI] 正在初始化...")

	// 初始化发现服务（会自动加载配置并注册设备信息）
	if err := service.InitService(); err != nil {
		return fmt.Errorf("初始化发现服务失败: %v", err)
	}
	logger.Info("[CLI] 发现服务已启动")

	// 获取本地设备信息（从配置文件中已加载）
	localDevInfo := service.DiscCoapGetDeviceInfo()
	logger.Infof("[CLI] 设备信息：ID=%s, Name=%s, Type=%s", localDevInfo.DeviceId, localDevInfo.Name, localDevInfo.DeviceType)

	// 设置设备发现回调
	service.SetDiscoverCallback(c.onDeviceFound)
	logger.Info("[CLI] 设备发现回调已注册")

	// 创建authmanager的设备信息
	authDevInfo := &authmanager.DeviceInfo{
		DeviceID:   localDevInfo.DeviceId,
		DeviceIP:   "", // 将由网络管理器获取
		DeviceName: localDevInfo.Name,
		DevicePort: -1,
		Version:    localDevInfo.Version,
	}
	localIp, _, err := service.GetLocalNetworkInfo()
	if err != nil {
		logger.Warnf("[CLI] 获取本地IP失败: %v", err)
		localIp = net.IPv4(0, 0, 0, 0)
	}
	authDevInfo.DeviceIP = localIp.String()

	// 创建并启动BusManager
	c.busManager = authmanager.NewBusManager(authDevInfo)
	if err := c.busManager.Start(); err != nil {
		service.DiscCoapDeinit()
		return fmt.Errorf("启动BusManager失败: %v", err)
	}

	authPort := c.busManager.GetAuthManager().GetAuthPort()
	sessionPort := c.busManager.GetSessionManager().GetPort()
	logger.Infof("[CLI] BusManager已启动，认证端口: %d, 会话端口: %d",
		authPort, sessionPort)

	// 更新认证端口到发现服务
	service.UpdateAuthPortToCoapService(authPort)

	logger.Info("[CLI] 初始化完成")

	return nil
}

// Shutdown 关闭CLI
func (c *CLI) Shutdown() {
	logger.Info("[CLI] 正在关闭...")

	if c.busManager != nil {
		c.busManager.Stop()
	}

	service.DiscCoapDeinit()

	logger.Info("[CLI] 已关闭")
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

// performHiChainAuth 执行HiChain密钥协商流程（完整实现）
// 参数：
//   - client：认证客户端
//   - authConn：认证连接信息
// 返回：
//   - 错误（协商失败时）
//
// 说明：本方法实现了完整的HiChain协议，与真实的HarmonyOS设备兼容。
// 认证流程包括：
// 1. 客户端发送AUTH_START消息
// 2. 服务端响应AUTH_CHALLENGE消息
// 3. 客户端发送AUTH_RESPONSE消息
// 4. 服务端发送AUTH_CONFIRM消息
// 5. 双方通过PAKE算法协商会话密钥
// 6. 密钥通过SetSessionKey回调保存到SessionKeyManager
func (c *CLI) performHiChainAuth(client *authmanager.AuthClient, authConn *authmanager.AuthConn) error {
	logger.Info("[CLI] 开始真实HiChain密钥协商...")

	// 执行完整的HiChain认证流程
	sessionKey, err := client.PerformHiChainAuth()
	if err != nil {
		return fmt.Errorf("HiChain认证失败: %v", err)
	}

	// 将协商完成的会话密钥保存到AuthManager的SessionKeyManager
	// 使用设备ID作为标识，索引为0
	authMgr := c.busManager.GetAuthManager()
	deviceID := authConn.DeviceID
	if err := authMgr.GetSessionKeyManager().AddSessionKey(deviceID, 0, sessionKey); err != nil {
		return fmt.Errorf("添加会话密钥失败: %v", err)
	}

	logger.Infof("[CLI] HiChain认证完成，已保存会话密钥 (deviceID=%s, index=0, 长度=%d字节)",
		deviceID, len(sessionKey))

	return nil
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
		if authConn, ok := c.authenticatedDev[device.DeviceId]; ok {
			authStatus = fmt.Sprintf("✓ 已认证 (会话端口:%d)", authConn.SessionPort)
		}

		fmt.Printf("  [%s]\n", device.DeviceId)
		fmt.Printf("    名称:     %s\n", device.DeviceName)
		fmt.Printf("    IP:       %s\n", deviceIP)
		fmt.Printf("    认证端口: %s\n", portStr)
		fmt.Printf("    状态:     %s\n", authStatus)
		fmt.Printf("    类型:     %s\n", service.GetDeviceNameByType((service.DeviceType)(device.DeviceType)))
		fmt.Println()
	}
}

// AuthDevice 认证设备（完整的认证流程）
func (c *CLI) AuthDevice(deviceID string) error {
	device, exists := c.devices[deviceID]
	if !exists {
		return fmt.Errorf("设备不存在: %s", deviceID)
	}

	// 从NetChannelInfo获取IP地址
	deviceIP := device.NetChannelInfo.Network.IP.String()
	if deviceIP == "" || deviceIP == "<nil>" {
		return fmt.Errorf("设备IP地址无效")
	}

	// 从ServiceData解析认证端口
	authPort, err := parsePortFromServiceData(device.ServiceData)
	if err != nil {
		return fmt.Errorf("解析认证端口失败: %v", err)
	}

	logger.Infof("[CLI] 开始认证设备: %s at %s:%d", deviceID, deviceIP, authPort)
	fmt.Printf("正在认证设备 %s (%s:%d)...\n", deviceID, deviceIP, authPort)

	// 获取本地设备信息
	localDevInfo := service.DiscCoapGetDeviceInfo()
	localIP, _, _ := service.GetLocalNetworkInfo()

	// 创建认证客户端
	clientDevInfo := &authmanager.DeviceInfo{
		DeviceID:   localDevInfo.DeviceId,
		DeviceIP:   localIP.String(),
		DeviceName: localDevInfo.Name,
		DevicePort: c.busManager.GetAuthManager().GetAuthPort(),
		Version:    localDevInfo.Version,
	}
	client := authmanager.NewAuthClient(clientDevInfo)

	// 1. 连接到认证端口
	if err := client.Connect(deviceIP, authPort); err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	defer client.Close()

	// 2. 发送GetAuthInfo请求
	if err := client.SendGetAuthInfo(); err != nil {
		return fmt.Errorf("发送GetAuthInfo失败: %v", err)
	}

	// 3. 接收GetAuthInfo响应
	if err := client.ReceiveResponse(); err != nil {
		return fmt.Errorf("接收GetAuthInfo响应失败: %v", err)
	}

	// 4. 发送VerifyIP请求（交换端口信息）
	localAuthPort := c.busManager.GetAuthManager().GetAuthPort()
	localSessionPort := c.busManager.GetSessionManager().GetPort()
	if err := client.SendVerifyIP(localAuthPort, localSessionPort); err != nil {
		return fmt.Errorf("发送VerifyIP失败: %v", err)
	}

	// 5. 接收VerifyIP响应（获取对端会话端口）
	if err := client.ReceiveResponse(); err != nil {
		return fmt.Errorf("接收VerifyIP响应失败: %v", err)
	}

	// 保存认证信息
	authConn := client.GetAuthConn()
	c.authenticatedDev[deviceID] = authConn

	// 6. 重要：通过HiChain进行密钥协商（与真实鸿蒙设备兼容）
	// 在真实HarmonyOS中，这是通过AUTH_SDK模块完成的
	// HiChain完成4步握手：AUTH_START → AUTH_CHALLENGE → AUTH_RESPONSE → AUTH_CONFIRM
	logger.Info("[CLI] 开始HiChain密钥协商...")
	if err := c.performHiChainAuth(client, authConn); err != nil {
		return fmt.Errorf("HiChain密钥协商失败: %v", err)
	}
	logger.Info("[CLI] HiChain密钥协商完成")

	// 注意：在真实的HarmonyOS中，HiChain认证完成后通常会有一个VerifyDeviceID步骤
	// 但是该步骤主要用于确认设备ID，密钥已经通过HiChain协商完成
	// 为了简化流程和避免goroutine竞争，我们在HiChain完成后直接关闭认证连接
	// 后续的Session建立将使用会话端口进行新连接

	logger.Infof("[CLI] 设备 %s 认证成功，会话端口: %d", deviceID, authConn.SessionPort)
	fmt.Printf("✓ 设备 %s 认证成功！\n", deviceID)
	fmt.Printf("  认证端口: %d\n", authConn.AuthPort)
	fmt.Printf("  会话端口: %d\n", authConn.SessionPort)
	fmt.Printf("  设备ID:   %s\n", authConn.DeviceID)
	fmt.Println("提示: 使用 'connect <设备ID> <会话名>' 命令建立会话")

	return nil
}

// ConnectDevice 连接到设备并建立会话
func (c *CLI) ConnectDevice(deviceID, sessionName string) (int, error) {
	// 检查设备是否已认证
	authConn, authenticated := c.authenticatedDev[deviceID]
	if !authenticated {
		return -1, fmt.Errorf("设备 %s 未认证，请先执行: auth %s", deviceID, deviceID)
	}

	// 从已认证信息中获取IP和会话端口
	device, exists := c.devices[deviceID]
	if !exists {
		return -1, fmt.Errorf("设备不存在: %s", deviceID)
	}

	deviceIP := device.NetChannelInfo.Network.IP.String()
	sessionPort := authConn.SessionPort

	logger.Infof("[CLI] 连接到设备 %s (%s:%d) 会话名: %s", deviceID, deviceIP, sessionPort, sessionName)
	fmt.Printf("正在连接到设备 %s 的会话端口 %d...\n", deviceID, sessionPort)

	// 获取本地设备ID
	localDevInfo := service.DiscCoapGetDeviceInfo()

	// 重要：在打开Session之前，先创建SessionServer用于接收消息
	// 检查是否已创建该SessionServer
	moduleName := "cli"
	fullSessionName := moduleName + "/" + sessionName

	// 查找或创建SessionServer
	if c.busManager.GetSessionServer(fullSessionName) == nil {
		logger.Infof("[CLI] 为客户端会话创建SessionServer: %s", fullSessionName)
		listener := &SessionListener{sessionName: sessionName}
		if err := c.busManager.CreateSessionServer(moduleName, sessionName, listener); err != nil {
			return -1, fmt.Errorf("创建SessionServer失败: %v", err)
		}
	}

	// 使用SessionManager打开会话
	sessionID, err := c.busManager.OpenSession(deviceIP, sessionPort, sessionName, localDevInfo.DeviceId)
	if err != nil {
		return -1, fmt.Errorf("打开会话失败: %v", err)
	}

	logger.Infof("[CLI] 会话 #%d 已建立", sessionID)
	fmt.Printf("✓ 已成功连接到设备 %s，会话ID: %d\n", deviceID, sessionID)

	return sessionID, nil
}

// SessionListener 会话监听器实现
type SessionListener struct {
	sessionName string
}

func (sl *SessionListener) OnSessionOpened(sessionID int) int {
	logger.Infof("[SESSION] 会话已打开: ID=%d, Name=%s", sessionID, sl.sessionName)
	fmt.Printf("\n>>> 会话 #%d 已建立 <<<\n", sessionID)
	fmt.Print("softbus> ")
	return 0
}

func (sl *SessionListener) OnSessionClosed(sessionID int) {
	logger.Infof("[SESSION] 会话已关闭: ID=%d", sessionID)
	fmt.Printf("\n>>> 会话 #%d 已关闭 <<<\n", sessionID)
	fmt.Print("softbus> ")
}

func (sl *SessionListener) OnBytesReceived(sessionID int, data []byte) {
	logger.Infof("[SESSION] 收到数据: ID=%d, 长度=%d", sessionID, len(data))
	fmt.Printf("\n>>> 会话 #%d 收到消息: %s\n", sessionID, string(data))
	fmt.Print("softbus> ")
}

// CreateSession 创建会话
func (c *CLI) CreateSession(sessionName string) error {
	logger.Infof("[CLI] 创建会话服务器: %s", sessionName)

	// 创建监听器
	listener := &SessionListener{
		sessionName: sessionName,
	}

	// 创建会话服务器
	if err := c.busManager.CreateSessionServer("cli", sessionName, listener); err != nil {
		return fmt.Errorf("创建会话服务器失败: %v", err)
	}

	logger.Info("[CLI] 会话服务器创建成功")
	fmt.Printf("✓ 会话服务器 '%s' 已创建，等待连接...\n", sessionName)
	return nil
}

// SendMessage 发送消息
func (c *CLI) SendMessage(sessionID int, message string) error {
	logger.Infof("[CLI] 发送消息到会话 %d: %s", sessionID, message)

	if err := c.busManager.SendBytes(sessionID, []byte(message)); err != nil {
		return fmt.Errorf("发送消息失败: %v", err)
	}

	logger.Info("[CLI] 消息发送成功")
	fmt.Printf("消息已发送到会话 #%d\n", sessionID)
	return nil
}

// CloseSession 关闭会话
func (c *CLI) CloseSession(sessionID int) error {
	logger.Infof("[CLI] 关闭会话: %d", sessionID)

	if err := c.busManager.CloseSession(sessionID); err != nil {
		return fmt.Errorf("关闭会话失败: %v", err)
	}

	logger.Info("[CLI] 会话已关闭")
	fmt.Printf("会话 #%d 已关闭\n", sessionID)
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
		fmt.Print("\nsoftbus> ")
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

		case "auth":
			if len(parts) < 2 {
				fmt.Println("用法: auth <设备ID>")
				continue
			}
			if err := c.AuthDevice(parts[1]); err != nil {
				fmt.Printf("错误: %v\n", err)
			}

		case "connect":
			if len(parts) < 3 {
				fmt.Println("用法: connect <设备ID> <会话名>")
				continue
			}
			sessionID, err := c.ConnectDevice(parts[1], parts[2])
			if err != nil {
				fmt.Printf("错误: %v\n", err)
			} else {
				fmt.Printf("连接成功，会话ID: %d\n", sessionID)
			}

		case "session":
			if len(parts) < 2 {
				fmt.Println("用法: session create <会话名>")
				continue
			}
			subcmd := parts[1]
			switch subcmd {
			case "create":
				if len(parts) < 3 {
					fmt.Println("用法: session create <会话名>")
					continue
				}
				if err := c.CreateSession(parts[2]); err != nil {
					fmt.Printf("错误: %v\n", err)
				}
			default:
				fmt.Printf("未知子命令: %s\n", subcmd)
			}

		case "send":
			if len(parts) < 3 {
				fmt.Println("用法: send <会话ID> <消息>")
				continue
			}
			var sessionID int
			fmt.Sscanf(parts[1], "%d", &sessionID)
			message := strings.Join(parts[2:], " ")
			if err := c.SendMessage(sessionID, message); err != nil {
				fmt.Printf("错误: %v\n", err)
			}

		case "close":
			if len(parts) < 2 {
				fmt.Println("用法: close <会话ID>")
				continue
			}
			var sessionID int
			fmt.Sscanf(parts[1], "%d", &sessionID)
			if err := c.CloseSession(sessionID); err != nil {
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

// printHelp 打印帮助信息
func (c *CLI) printHelp() {
	fmt.Println("\n可用命令:")
	fmt.Println("  help, h                     - 显示此帮助")
	fmt.Println("  discover                    - 主动发现网络中的设备")
	fmt.Println("  publish <能力>              - 发布服务能力")
	fmt.Println("                                可用能力: hicall, profile, castPlus, dvKit, ddmpCapability")
	fmt.Println("  list                        - 列出已发现的设备")
	fmt.Println("  auth <设备ID>               - 认证设备（必须先认证才能连接）")
	fmt.Println("  connect <设备ID> <会话名>   - 连接到设备并建立会话（需先认证）")
	fmt.Println("  session create <名称>       - 创建会话服务器（等待连接）")
	fmt.Println("  send <会话ID> <消息>        - 发送消息到会话")
	fmt.Println("  close <会话ID>              - 关闭会话")
	fmt.Println("  exit, quit, q               - 退出程序")
	fmt.Println()
	fmt.Println("典型流程:")
	fmt.Println("  1. discover                 - 主动发现设备（或等待自动发现）")
	fmt.Println("  2. list                     - 查看已发现的设备")
	fmt.Println("  3. auth <设备ID>            - 认证设备")
	fmt.Println("  4. connect <设备ID> <会话名> - 建立会话")
	fmt.Println("  5. send <会话ID> <消息>      - 发送消息")
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
