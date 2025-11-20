package tcp_connection

import (
	"net"
	"time"
)

const (
	DefaultBufSize       = 1536 // 默认缓冲区大小（字节）
	ServerFdStart        = 1000 // 服务端虚拟fd起始值
	ClientFdStart        = 2000 // 客户端虚拟fd起始值
	DefaultConnectTimeout = 5 * time.Second // 默认连接超时时间
)

// ConnectionType 连接类型
type ConnectionType int

const (
	ConnectionTypeServer ConnectionType = iota // 服务端连接
	ConnectionTypeClient                       // 客户端连接
)

// SocketOption 套接字选项
type SocketOption struct {
	Addr string
	Port int
}

// ConnectOption 连接信息（包含本地和远程两端的完整信息）
type ConnectOption struct {
	LocalSocket  *SocketOption // 本地地址和端口
	RemoteSocket *SocketOption // 远程地址和端口
	NetConn      *net.Conn     // 网络连接
}

// ClientOption 客户端配置选项
type ClientOption struct {
	RemoteIP     string        // 远程IP地址
	RemotePort   int           // 远程端口
	Timeout      time.Duration // 连接超时时间（0表示使用默认值）
	KeepAlive    bool          // 是否启用长连接
	KeepAlivePeriod time.Duration // 长连接保活周期
}

// BaseListenerCallback 统一的事件回调处理器（服务端和客户端共用）
type BaseListenerCallback struct {
	OnConnected    func(fd int, connType ConnectionType, connOpt *ConnectOption) // 连接建立回调
	OnDisconnected func(fd int, connType ConnectionType)                         // 连接断开回调
	OnDataReceived func(fd int, connType ConnectionType, buf []byte, used int) int // 数据接收回调，返回已处理的字节数（-1表示解析失败）
}
