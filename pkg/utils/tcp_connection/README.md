# TCP Connection 库

一个简洁的 TCP 客户端/服务端通信库，提供统一的事件回调接口，简化网络通信开发。

## 特性

- 客户端与服务端接口风格统一，均采用事件回调模式
- 自动处理连接管理、数据收发和缓冲区管理
- 支持自定义数据包解析逻辑（通过 `processPackets` 回调）


## 快速开始

### 服务端使用示例

```go
package main

import (
	"fmt"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/tcp_connection"
)

func main() {
	// 创建服务器实例
	server := tcp_connection.NewBaseServer()

	// 定义事件处理器
	handler := &tcp_connection.BaseListenerHandler{
		onConnectEvent: func(fd int, connect *tcp_connection.ConnectOption) {
			fmt.Printf("新连接 (fd=%d)：%s:%d\n", 
				fd, 
				connect.socketOption.addr, 
				connect.socketOption.port)
		},
		onDisconnectEvent: func(fd int) {
			fmt.Printf("连接断开 (fd=%d)\n", fd)
		},
		processPackets: func(fd int, netConn *net.Conn, buf []byte, used int) int {
			data := make([]byte, used)
			copy(data, buf[:used])
			fmt.Printf("收到数据 (fd=%d)：%s\n", fd, string(data))

			// 示例：回复客户端
			(*netConn).Write([]byte("已收到: " + string(data)))

			return used // 标记所有数据已处理
		},
	}

	// 启动服务器（监听 0.0.0.0:8080）
	opt := &tcp_connection.SocketOption{addr: "0.0.0.0", port: 8080}
	if err := server.StartBaseListener(opt, handler); err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
		return
	}
	defer server.StopBaseListener()

	// 阻塞运行
	fmt.Println("服务器运行中，按 Ctrl+C 退出...")
	select {}
}
```

### 客户端使用示例

```go
package main

import (
	"fmt"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/tcp_connection"
)

func main() {
	// 定义事件处理器
	handler := &tcp_connection.BaseClientHandler{
		onConnect: func() {
			fmt.Println("已连接到服务器")
			// 连接成功后发送数据
			if err := client.SendBytes([]byte("hello server")); err != nil {
				fmt.Printf("发送失败: %v\n", err)
			}
		},
		onDisconnect: func() {
			fmt.Println("与服务器断开连接")
		},
		processPackets: func(buf []byte, used int) int {
			data := make([]byte, used)
			copy(data, buf[:used])
			fmt.Printf("收到服务器数据：%s\n", string(data))
			return used // 标记所有数据已处理
		},
	}

    // 创建客户端实例
	client := tcp_connection.GetBaseClient(handler)

	// 连接服务器
	if err := client.Connect("127.0.0.1", 8080); err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer client.Close()

	// 阻塞运行
	fmt.Println("客户端运行中，按 Ctrl+C 退出...")
	select {}
}
```

## API 说明

### 服务器（BaseServer）

- NewBaseServer(): 创建服务器实例
- StartBaseListener(opt *SocketOption, handler *BaseListenerHandler) error: 启动服务器监听
- StopBaseListener() error: 停止服务器并关闭所有连接
- SendBytes(fd int, data []byte) error: 向指定连接（通过 fd）发送数据
- GetPort() int: 获取服务器监听的端口

### 客户端（BaseClient）

- GetBaseClient(handler *BaseClientHandler): 使用事件处理器创建客户端实例
- Connect(remoteIP string, remotePort int) error: 连接远程服务器
- Close(): 关闭连接
- SendBytes(data []byte) error: 发送数据

### 事件处理器（BaseListenerHandler）

|  服务端回调  |  客户端回调  |  说明  |
|  ---------  |  ---------  |  ----  |
|  onConnectEvent  |  onConnect |  连接建立时触发  |
|  onDisconnectEvent  |  onDisconnect |  连接断开时触发  |
|  processPackets  |  processPackets |  收到数据时触发，返回已处理的字节数（-1 表示处理失败） |


## 注意事项

- processPackets 回调需正确返回已处理的字节数，未处理的数据会保留在缓冲区中
- 缓冲区默认大小为 1536 字节，可通过修改 DefaultBufSize 调整