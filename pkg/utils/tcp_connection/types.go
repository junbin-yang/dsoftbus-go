package tcp_connection

import (
	"net"
)

const (
	DefaultBufSize = 1536 // 默认缓冲区大小（字节）
)

type SocketOption struct {
	addr string
	port int
}

// ConnectOption 连接选项
type ConnectOption struct {
	socketOption *SocketOption // 监听器套接字选项
	netConn      *net.Conn     // 网络连接
}

// BaseListenerHandler 基础监听器，定义连接事件和数据处理的回调函数
type BaseListenerHandler struct {
	// onConnectEvent 连接事件回调函数
	onConnectEvent func(fd int, connect *ConnectOption)

	// onDisconnectEvent 断开事件回调函数
	onDisconnectEvent func(fd int)

	// processPackets 数据处理回调函数
	// 参数：
	//   - fd：伪文件描述符
	//   - netConn：网络连接
	//   - buf：数据缓冲区
	//   - used：已使用的缓冲区大小
	// 返回：
	//   - 已处理的字节数（-1表示解析失败）
	processPackets func(fd int, netConn *net.Conn, buf []byte, used int) int
}

// BaseClientHandler 客户端事件回调处理器
type BaseClientHandler struct {
	// onConnect 客户端连接成功时触发
	onConnect func()

	// onDisconnect 客户端断开连接时触发
	onDisconnect func()

	// processPackets 客户端接收数据时触发（处理收到的数据包）
	// 参数：
	//   - buf：数据缓冲区
	//   - used：已使用的缓冲区大小
	// 返回：
	//   - 已处理的字节数（-1表示解析失败）
	processPackets func(buf []byte, used int) int
}
