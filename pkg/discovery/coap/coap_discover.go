package coap

import (
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

const (
	COAP_DEVICE_DISCOVER_URI = "device_discover"

	NSTACKX_EOK     = 0
	NSTACKX_EFAILED = -1
)

var (
	gTerminalFlag int32
	onceInit      sync.Once
	listenWG      sync.WaitGroup
)

// ========= 设备发现核心逻辑 =========
// CoapResponseService: 根据请求包构造响应并发送
func coapResponseService(pkt *COAP_Packet, remoteUrl, remoteIp string) int {
	if pkt == nil || remoteUrl == "" || remoteIp == "" {
		return NSTACKX_EFAILED
	}

	// 1. 生成负载
	payloadStr, err := PrepareServiceDiscover(false)
	if err != nil {
		return NSTACKX_EFAILED
	}

	// 2. 构造发送缓冲
	snd := NewCOAPReadWriteBuffer(COAP_MAX_PDU_SIZE)

	// 3. 构建发送包
	ret := BuildSendPkt(pkt, remoteIp, payloadStr, snd)
	if ret != NSTACKX_EOK {
		return ret
	}

	// 4. 发送UDP
	dst := &net.UDPAddr{IP: net.ParseIP(remoteIp), Port: COAP_DEFAULT_PORT}
	cli, err := CoapCreateUDPClient(dst)
	if err != nil {
		log.Error("[DISCOVERY] create udp client failed:", log.GetError(err))
		return NSTACKX_EFAILED
	}
	defer CoapCloseSocket(cli)

	n, err := CoapSocketSend(cli, snd.Buffer[:snd.Len])
	if err != nil {
		log.Error("[DISCOVERY] send resp failed:", log.GetError(err))
		return NSTACKX_EFAILED
	}
	log.Debugf("[DISCOVERY] send resp ok, bytes: %d, payload: %s", n, payloadStr)
	return NSTACKX_EOK
}

// GetServiceDiscoverInfo: 使用 ParseServiceDiscover 解析负载
func getServiceDiscoverInfo(buf []byte) (ipAddr, remoteUrl string, dev *DeviceInfo, ret int) {
	dev = &DeviceInfo{}
	url, err := ParseServiceDiscover(buf, dev)
	if err != nil {
		log.Error("[DISCOVERY] parse service discover failed:", log.GetError(err))
		return "", "", nil, NSTACKX_EFAILED
	}
	if dev.NetChannelInfo.Network.IP == nil {
		log.Error("[DISCOVERY] ip is nil")
		return "", "", nil, NSTACKX_EFAILED
	}
	ipAddr = dev.NetChannelInfo.Network.IP.String()
	remoteUrl = url
	return ipAddr, remoteUrl, dev, NSTACKX_EOK
}

// PostServiceDiscover: 处理接收包，解析并回复
func postServiceDiscover(pkt *COAP_Packet) {
	if pkt == nil || pkt.Payload.Len == 0 || len(pkt.Payload.Buffer) == 0 {
		return
	}
	ipAddr, remoteUrl, dev, ret := getServiceDiscoverInfo(pkt.Payload.Buffer[:pkt.Payload.Len])
	if ret != NSTACKX_EOK {
		log.Error("[DISCOVERY] postServiceDiscover failed")
		return
	}
	if discoverCallbackProvider != nil {
		discoverCallbackProvider(dev)
	}
	if remoteUrl != "" {
		_ = coapResponseService(pkt, remoteUrl, ipAddr)
	}
}

// ========= I/O与线程（goroutine）逻辑 =========

func handleReadEvent(server *SocketInfo) {
	if server == nil || server.Conn == nil {
		return
	}
	buf := make([]byte, COAP_MAX_PDU_SIZE)
	n, src, err := CoapSocketRecv(server, buf)
	if err != nil || n <= 0 {
		return
	}

	// 过滤自身发出的广播/单播（比较来源IP与本地IP）
	if localIPStringProvider != nil && src != nil && src.IP != nil {
		if ipstr, _ := localIPStringProvider(); ipstr != "" && src.IP.String() == ipstr {
			return
		}
	}

	// 打印原始报文（十六进制）与来源地址
	/*
		fmt.Printf("[DISCOVERY] recv %d bytes from %v: ", n, src)
		for i := 0; i < n; i++ {
			fmt.Printf("%02X ", buf[i])
		}
		fmt.Println()
	*/

	// 解码
	var decodePkt COAP_Packet
	decodePkt.Protocol = COAP_UDP
	ret := COAP_SoftBusDecode(&decodePkt, buf[:n], n)
	if ret != DISCOVERY_ERR_SUCCESS {
		log.Errorf("[DISCOVERY] decode failed: %d", ret)
		return
	}

	// 打印解析后的关键信息
	log.Debugf("[DISCOVERY] ver=%d type=%d code=%d msgId=%d tokenLen=%d opts=%d payloadLen=%d",
		decodePkt.Header.Ver, decodePkt.Header.Type, decodePkt.Header.Code,
		decodePkt.Header.MsgId, decodePkt.Header.TokenLen, decodePkt.OptionsNum, decodePkt.Payload.Len,
	)

	// Payload内容仅在需要调试时打印
	// if decodePkt.Payload.Len > 0 && len(decodePkt.Payload.Buffer) >= int(decodePkt.Payload.Len) {
	// 	pl := decodePkt.Payload.Buffer[:decodePkt.Payload.Len]
	// 	log.Debugf("[DISCOVERY] payload: %s", string(pl))
	// }

	postServiceDiscover(&decodePkt)
}

func coapReadLoop() {
	defer listenWG.Done()
	server := GetCoapServerSocket()
	if server == nil {
		return
	}
	for {
		if gTerminalFlag == 0 {
			return
		}
		// 阻塞读取（由ReadFromUDP阻塞），无需select
		handleReadEvent(server)
	}
}

// ========= 对外初始化/反初始化 =========

func CoapInitDiscovery() int {
	var initErr error
	onceInit.Do(func() {
		// 1. 初始化服务器Socket
		if err := CoapInitServerSocket(); err != nil {
			initErr = err
			return
		}
		// 2. 启动监听协程
		gTerminalFlag = 1
		listenWG.Add(1)
		go coapReadLoop()
	})
	if initErr != nil {
		return NSTACKX_EFAILED
	}
	return NSTACKX_EOK
}

func CoapDeinitDiscovery() int {
	// 停监听
	gTerminalFlag = 0

	// 关闭socket
	if s := GetCoapServerSocket(); s != nil {
		_ = CoapCloseSocket(s)
	}

	// 等待goroutine退出
	done := make(chan struct{})
	go func() { listenWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	return NSTACKX_EOK
}

// BuildDiscoverPacket 将设备发现请求编码为CoAP字节流
func BuildDiscoverPacket(subnetIP string) ([]byte, error) {
	// 负载
	payloadStr, err := PrepareServiceDiscover(true)
	if err != nil {
		return nil, err
	}

	// 组帧
	snd := NewCOAPReadWriteBuffer(COAP_MAX_PDU_SIZE)
	var pkt COAP_Packet
	opts := []COAP_Option{
		{Num: DISCOVERY_MSG_URI_HOST, OptionBuf: []byte(subnetIP), Len: uint32(len(subnetIP))},
		{Num: DISCOVERY_MSG_URI_PATH, OptionBuf: []byte(COAP_DEVICE_DISCOVER_URI), Len: uint32(len(COAP_DEVICE_DISCOVER_URI))},
	}
	param := COAP_PacketParam{
		Protocol:   COAP_UDP,
		Type:       COAP_TYPE_CON,
		Code:       COAP_METHOD_POST,
		MsgId:      COAP_SoftBusMsgId(),
		Options:    opts,
		OptionsNum: uint8(len(opts)),
	}
	token := COAP_Buffer{Buffer: nil, Len: 0}
	payload := COAP_Buffer{Buffer: []byte(payloadStr), Len: uint32(len(payloadStr))}

	if ret := COAP_SoftBusEncode(&pkt, &param, &token, &payload, snd); ret != 0 {
		return nil, fmt.Errorf("encode failed: %d", ret)
	}
	return snd.Buffer[:snd.Len], nil
}
