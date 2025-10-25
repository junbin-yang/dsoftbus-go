package coap

import (
	"errors"
	"net"
	"sync"

	"golang.org/x/net/ipv4"
)

const (
	COAP_DEFAULT_PORT = 5684 // CoAP默认端口（UDP）
	COAP_MAX_PDU_SIZE = 1024 // 最大PDU长度
	COAP_TTL_VALUE    = 64   // 默认TTL值
)

// 错误定义
var (
	ErrInvalidParam       = errors.New("invalid parameter")
	ErrSocketCreateFailed = errors.New("socket create failed")
	ErrAddressInvalid     = errors.New("invalid address")
	ErrBindFailed         = errors.New("bind failed")
	ErrConnectFailed      = errors.New("connect failed")
)

// socket信息结构体
type SocketInfo struct {
	Conn    *net.UDPConn // UDP连接实例
	DstAddr *net.UDPAddr // 目标地址（客户端用）
}

var (
	gServerSocket *SocketInfo // 全局服务器Socket
	socketMu      sync.Mutex  // 线程安全锁
)

// 获取服务器Socket
func GetCoapServerSocket() *SocketInfo {
	socketMu.Lock()
	defer socketMu.Unlock()
	return gServerSocket
}

// 创建并绑定CoAP UDP服务器
// 功能：创建UDP socket，绑定到指定地址和端口，用于接收客户端请求
func CoapCreateUDPServer(addr *net.UDPAddr) (*SocketInfo, error) {
	if addr == nil {
		return nil, ErrAddressInvalid
	}

	// 创建UDP监听（服务器）
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, ErrBindFailed
	}

	packetConn := ipv4.NewPacketConn(conn)
	// 设置组播TTL
	if err := packetConn.SetMulticastTTL(COAP_TTL_VALUE); err != nil {
		//h.log.Warn("设置IPv4组播TTL失败", zap.Error(err))
		conn.Close()
		return nil, ErrSocketCreateFailed
	}
	// 禁用IPv4组播回环（本机不接收自己发送的组播包）
	if err := packetConn.SetMulticastLoopback(false); err != nil {
		//h.log.Warn("禁用IPv4组播回环失败", zap.Error(err))
		conn.Close()
		return nil, ErrSocketCreateFailed
	}

	// 构造并返回SocketInfo
	return &SocketInfo{
		Conn:    conn,
		DstAddr: nil, // 服务器无需预设目标地址
	}, nil
}

// 创建CoAP UDP客户端
// 功能：创建UDP socket，可指定目标服务器地址（可选），用于向服务器发送请求
func CoapCreateUDPClient(dstAddr *net.UDPAddr) (*SocketInfo, error) {
	if dstAddr == nil {
		// 若未指定目标地址，客户端绑定到本地任意地址
		localAddr := &net.UDPAddr{
			IP:   net.IPv4zero,
			Port: 0,
		}
		conn, err := net.ListenUDP("udp", localAddr)
		if err != nil {
			return nil, ErrSocketCreateFailed
		}
		return &SocketInfo{
			Conn:    conn,
			DstAddr: nil, // 后续发送时需指定目标地址
		}, nil
	}

	// 若指定目标地址，直接"连接"到该地址（便于后续Write直接发送）
	conn, err := net.DialUDP("udp", nil, dstAddr)
	if err != nil {
		return nil, ErrConnectFailed
	}

	return &SocketInfo{
		Conn:    conn,
		DstAddr: dstAddr, // 记录目标地址
	}, nil
}

// 初始化服务器Socket（封装CoapCreateUDPServer）
func CoapInitServerSocket() error {
	socketMu.Lock()
	defer socketMu.Unlock()

	if gServerSocket != nil {
		return nil // 已初始化
	}

	// 绑定到默认端口和所有网卡
	defaultAddr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: COAP_DEFAULT_PORT,
	}

	serverSock, err := CoapCreateUDPServer(defaultAddr)
	if err != nil {
		return err
	}
	gServerSocket = serverSock
	COAP_SoftBusInitMsgId() // 初始化消息ID生成器
	return nil
}

// 通过Socket发送数据
func CoapSocketSend(socket *SocketInfo, data []byte) (int, error) {
	if socket == nil || socket.Conn == nil || data == nil {
		return 0, ErrInvalidParam
	}

	// 如果 CoapCreateUDPClient 已经指定目标地址，可直接发送
	ret, err := socket.Conn.Write(data)
	if err != nil {
		if socket.DstAddr != nil {
			ret, err = socket.Conn.WriteToUDP(data, socket.DstAddr)
		}
	}

	return ret, err
}

// 从Socket接收数据
func CoapSocketRecv(socket *SocketInfo, buf []byte) (int, *net.UDPAddr, error) {
	if socket == nil || socket.Conn == nil || buf == nil {
		return 0, nil, ErrInvalidParam
	}

	// 接收数据并返回发送方地址
	n, srcAddr, err := socket.Conn.ReadFromUDP(buf)
	return n, srcAddr, err
}

// 关闭Socket
func CoapCloseSocket(socket *SocketInfo) error {
	if socket == nil || socket.Conn == nil {
		return nil
	}
	return socket.Conn.Close()
}

