# TCP Connection 库

一个强大的 TCP 客户端/服务端通信库，提供统一的事件回调接口和虚拟文件描述符管理，简化网络通信开发。

## 特性

-   ✅ **统一的回调接口**：客户端与服务端使用相同的 `BaseListenerCallback` 接口
-   ✅ **虚拟 fd 管理**：服务端从 `1000` 开始，客户端从 `2000` 开始，避免冲突
-   ✅ **统一的连接管理器**：`ConnectionManager` 集中管理所有连接（支持服务端和客户端共用）
-   ✅ **灵活的客户端配置**：支持连接超时、长连接（KeepAlive）等配置
-   ✅ **自动缓冲区管理**：自动处理数据包边界和缓冲区溢出
-   ✅ **线程安全**：内置锁机制保证并发安全

## 架构设计

### 虚拟文件描述符（fd）分配策略

```
服务端连接: 1000, 1001, 1002, ...
客户端连接: 2000, 2001, 2002, ...
```

### 统一的回调接口

```go
type BaseListenerCallback struct {
    OnConnected    func(fd int, connType ConnectionType, connOpt *ConnectOption)  // 连接建立
    OnDisconnected func(fd int, connType ConnectionType)                          // 连接断开
    OnDataReceived func(fd int, connType ConnectionType, buf []byte, used int) int // 数据接收
}
```

## 快速开始

### 服务端使用示例

```go
package main

import (
	"fmt"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/tcp_connection"
)

func main() {
	// 创建连接管理器（可选，如果不传则自动创建）
	connMgr := tcp_connection.NewConnectionManager()

	// 创建服务器实例
	server := tcp_connection.NewBaseServer(connMgr)

	// 定义统一的事件回调
	callback := &tcp_connection.BaseListenerCallback{
		OnConnected: func(fd int, connType tcp_connection.ConnectionType, connOpt *tcp_connection.ConnectOption) {
			fmt.Printf("新连接 (fd=%d, type=%v)\n", fd, connType)
			fmt.Printf("  本地: %s:%d\n",
				connOpt.LocalSocket.Addr, connOpt.LocalSocket.Port)
			fmt.Printf("  远程: %s:%d\n",
				connOpt.RemoteSocket.Addr, connOpt.RemoteSocket.Port)
		},
		OnDisconnected: func(fd int, connType tcp_connection.ConnectionType) {
			fmt.Printf("连接断开 (fd=%d, type=%v)\n", fd, connType)
		},
		OnDataReceived: func(fd int, connType tcp_connection.ConnectionType, buf []byte, used int) int {
			data := make([]byte, used)
			copy(data, buf[:used])
			fmt.Printf("收到数据 (fd=%d, type=%v): %s\n", fd, connType, string(data))

			// 回显数据
			server.SendBytes(fd, []byte("收到: "+string(data)))

			return used // 返回已处理的字节数
		},
	}

	// 启动服务器（监听 0.0.0.0:8080）
	opt := &tcp_connection.SocketOption{Addr: "0.0.0.0", Port: 8080}
	if err := server.StartBaseListener(opt, callback); err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
		return
	}
	defer server.StopBaseListener()

	fmt.Printf("服务器启动成功，监听 %s:%d\n", server.GetAddr(), server.GetPort())

	// 阻塞运行
	select {}
}
```

### 客户端使用示例（基础用法）

```go
package main

import (
	"fmt"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/tcp_connection"
)

func main() {
	// 定义统一的事件回调
	callback := &tcp_connection.BaseListenerCallback{
		OnConnected: func(fd int, connType tcp_connection.ConnectionType, connOpt *tcp_connection.ConnectOption) {
			fmt.Printf("连接成功 (fd=%d, type=%v)\n", fd, connType)
			fmt.Printf("  本地端口: %d (随机分配)\n", connOpt.LocalSocket.Port)
			fmt.Printf("  远程服务器: %s:%d\n",
				connOpt.RemoteSocket.Addr, connOpt.RemoteSocket.Port)
		},
		OnDisconnected: func(fd int, connType tcp_connection.ConnectionType) {
			fmt.Printf("连接断开 (fd=%d, type=%v)\n", fd, connType)
		},
		OnDataReceived: func(fd int, connType tcp_connection.ConnectionType, buf []byte, used int) int {
			data := make([]byte, used)
			copy(data, buf[:used])
			fmt.Printf("收到服务器数据 (fd=%d, type=%v): %s\n", fd, connType, string(data))
			return used
		},
	}

	// 创建客户端实例
	client := tcp_connection.NewBaseClient(nil, callback)

	// 连接服务器（简化方法）
	fd, err := client.ConnectSimple("127.0.0.1", 8080)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Printf("客户端连接成功，虚拟fd=%d\n", fd)

	// 发送数据
	if err := client.SendBytes([]byte("Hello Server")); err != nil {
		fmt.Printf("发送失败: %v\n", err)
	}

	// 阻塞运行
	select {}
}
```

### 客户端使用示例（高级配置）

```go
package main

import (
	"fmt"
	"time"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/tcp_connection"
)

func main() {
	callback := &tcp_connection.BaseListenerCallback{
		OnConnected: func(fd int, connType tcp_connection.ConnectionType, connOpt *tcp_connection.ConnectOption) {
			fmt.Printf("连接成功 (fd=%d, type=%v)\n", fd, connType)
		},
		OnDisconnected: func(fd int, connType tcp_connection.ConnectionType) {
			fmt.Printf("连接断开 (fd=%d, type=%v)\n", fd, connType)
		},
		OnDataReceived: func(fd int, connType tcp_connection.ConnectionType, buf []byte, used int) int {
			return used
		},
	}

	client := tcp_connection.NewBaseClient(nil, callback)

	// 使用高级配置连接
	opt := &tcp_connection.ClientOption{
		RemoteIP:        "127.0.0.1",
		RemotePort:      8080,
		Timeout:         10 * time.Second,  // 连接超时10秒
		KeepAlive:       true,              // 启用长连接
		KeepAlivePeriod: 30 * time.Second,  // 保活周期30秒
	}

	fd, err := client.Connect(opt)
	if err != nil {
		fmt.Printf("连接失败: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Printf("客户端连接成功 (fd=%d), KeepAlive已启用\n", fd)

	// 阻塞运行
	select {}
}
```

### 共享连接管理器示例

服务端和客户端可以共用同一个 `ConnectionManager`，实现统一的连接管理：

```go
package main

import (
	"fmt"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/tcp_connection"
)

func main() {
	// 创建共享的连接管理器
	sharedMgr := tcp_connection.NewConnectionManager()

	// 创建服务端
	serverCallback := &tcp_connection.BaseListenerCallback{
		OnConnected: func(fd int, connType tcp_connection.ConnectionType, connOpt *tcp_connection.ConnectOption) {
			fmt.Printf("[服务端] 新连接 (fd=%d, type=%v)\n", fd, connType)
		},
		OnDisconnected: func(fd int, connType tcp_connection.ConnectionType) {
			fmt.Printf("[服务端] 断开 (fd=%d, type=%v)\n", fd, connType)
		},
		OnDataReceived: func(fd int, connType tcp_connection.ConnectionType, buf []byte, used int) int {
			// 使用共享管理器发送数据
			sharedMgr.SendBytes(fd, buf[:used])
			return used
		},
	}

	server := tcp_connection.NewBaseServer(sharedMgr)
	server.StartBaseListener(&tcp_connection.SocketOption{Addr: "0.0.0.0", Port: 8080}, serverCallback)
	defer server.StopBaseListener()

	// 创建客户端（共用管理器）
	clientCallback := &tcp_connection.BaseListenerCallback{
		OnConnected: func(fd int, connType tcp_connection.ConnectionType, connOpt *tcp_connection.ConnectOption) {
			fmt.Printf("[客户端] 连接成功 (fd=%d, type=%v)\n", fd, connType)
		},
		OnDisconnected: func(fd int, connType tcp_connection.ConnectionType) {
			fmt.Printf("[客户端] 断开 (fd=%d, type=%v)\n", fd, connType)
		},
		OnDataReceived: func(fd int, connType tcp_connection.ConnectionType, buf []byte, used int) int {
			return used
		},
	}

	client := tcp_connection.NewBaseClient(sharedMgr, clientCallback)
	client.ConnectSimple("127.0.0.1", 8080)
	defer client.Close()

	// 通过共享管理器查看所有连接
	fmt.Printf("当前总连接数: %d\n", sharedMgr.GetConnCount())
	for _, fd := range sharedMgr.GetAllFds() {
		connType, _ := sharedMgr.GetConnType(fd)
		fmt.Printf("  fd=%d, type=%v\n", fd, connType)
	}

	select {}
}
```

### 共享回调函数示例

服务端和客户端可以共用同一个回调函数，通过 `ConnectionType` 参数区分连接类型：

```go
package main

import (
	"fmt"
	"github.com/junbin-yang/dsoftbus-go/pkg/utils/tcp_connection"
)

func main() {
	// 共享的回调函数，通过 connType 区分是服务端还是客户端连接
	sharedCallback := &tcp_connection.BaseListenerCallback{
		OnConnected: func(fd int, connType tcp_connection.ConnectionType, connOpt *tcp_connection.ConnectOption) {
			if connType == tcp_connection.ConnectionTypeServer {
				fmt.Printf("[服务端连接] fd=%d, 客户端地址: %s:%d\n",
					fd, connOpt.RemoteSocket.Addr, connOpt.RemoteSocket.Port)
			} else {
				fmt.Printf("[客户端连接] fd=%d, 服务器地址: %s:%d\n",
					fd, connOpt.RemoteSocket.Addr, connOpt.RemoteSocket.Port)
			}
		},
		OnDisconnected: func(fd int, connType tcp_connection.ConnectionType) {
			typeStr := "服务端"
			if connType == tcp_connection.ConnectionTypeClient {
				typeStr = "客户端"
			}
			fmt.Printf("[%s连接] fd=%d 断开\n", typeStr, fd)
		},
		OnDataReceived: func(fd int, connType tcp_connection.ConnectionType, buf []byte, used int) int {
			typeStr := "服务端"
			if connType == tcp_connection.ConnectionTypeClient {
				typeStr = "客户端"
			}
			fmt.Printf("[%s连接] fd=%d 收到数据: %s\n", typeStr, fd, string(buf[:used]))
			return used
		},
	}

	// 服务端和客户端都使用同一个回调
	server := tcp_connection.NewBaseServer(nil)
	server.StartBaseListener(&tcp_connection.SocketOption{Addr: "0.0.0.0", Port: 8080}, sharedCallback)
	defer server.StopBaseListener()

	client := tcp_connection.NewBaseClient(nil, sharedCallback)
	client.ConnectSimple("127.0.0.1", 8080)
	defer client.Close()

	select {}
}
```

## API 说明

### 核心类型

#### BaseListenerCallback（统一回调接口）

```go
type BaseListenerCallback struct {
    OnConnected    func(fd int, connType ConnectionType, connOpt *ConnectOption)
    OnDisconnected func(fd int, connType ConnectionType)
    OnDataReceived func(fd int, connType ConnectionType, buf []byte, used int) int
}
```

#### ConnectOption（连接信息）

```go
type ConnectOption struct {
    LocalSocket  *SocketOption // 本地地址和端口
    RemoteSocket *SocketOption // 远程地址和端口
    NetConn      *net.Conn     // 网络连接
}
```

**说明**：

-   服务端 `OnConnected` 回调：`LocalSocket` 是服务端监听地址，`RemoteSocket` 是客户端地址
-   客户端 `OnConnected` 回调：`LocalSocket` 是客户端本地地址（含随机端口），`RemoteSocket` 是服务端地址

#### ClientOption（客户端配置）

```go
type ClientOption struct {
    RemoteIP        string        // 远程IP地址
    RemotePort      int           // 远程端口
    Timeout         time.Duration // 连接超时（0=使用默认5秒）
    KeepAlive       bool          // 是否启用长连接
    KeepAlivePeriod time.Duration // 保活周期
}
```

### 服务器（BaseServer）

#### 创建与管理

-   `NewBaseServer(connMgr *ConnectionManager) *BaseServer`
    创建服务器实例。`connMgr` 为 `nil` 时自动创建新的管理器

-   `StartBaseListener(opt *SocketOption, callback *BaseListenerCallback) error`
    启动服务器监听

-   `StopBaseListener() error`
    停止服务器并关闭所有服务端连接

#### 数据操作

-   `SendBytes(fd int, data []byte) error`
    向指定虚拟 fd 发送数据

-   `GetConnInfo(fd int) *ConnectOption`
    获取连接的完整信息（包含本地和远程两端）

-   ~~`GetConnPeerInfo(fd int) *ConnectOption`~~（已废弃）
    获取连接的对端信息，建议使用 `GetConnInfo`

-   ~~`GetConnLocalInfo(fd int) *ConnectOption`~~（已废弃）
    获取连接的本地信息，建议使用 `GetConnInfo`

#### 信息查询

-   `GetPort() int`
    获取监听端口（-1 表示未启动）

-   `GetAddr() string`
    获取监听地址（空字符串表示未启动）

### 客户端（BaseClient）

#### 创建与连接

-   `NewBaseClient(connMgr *ConnectionManager, callback *BaseListenerCallback) *BaseClient`
    创建客户端实例

-   `Connect(opt *ClientOption) (int, error)`
    使用配置选项连接服务器，返回虚拟 fd

-   `ConnectSimple(remoteIP string, remotePort int) (int, error)`
    简化的连接方法（使用默认配置），返回虚拟 fd

-   `Close()`
    关闭连接

#### 数据操作

-   `SendBytes(data []byte) error`
    发送数据

-   `GetConnInfo() *ConnectOption`
    获取连接的完整信息（包含本地和远程两端）

-   ~~`GetConnPeerInfo() *ConnectOption`~~（已废弃）
    获取对端信息，建议使用 `GetConnInfo`

-   ~~`GetConnLocalInfo() *ConnectOption`~~（已废弃）
    获取本地信息，建议使用 `GetConnInfo`

#### 状态查询

-   `GetFd() int`
    获取虚拟 fd（-1 表示未连接）

-   `IsConnected() bool`
    检查是否已连接

### 连接管理器（ConnectionManager）

#### 创建

-   `NewConnectionManager() *ConnectionManager`
    创建新的连接管理器

#### 连接操作

-   `RegisterConn(conn net.Conn, connType ConnectionType) int`
    注册连接并分配虚拟 fd

-   `UnregisterConn(fd int)`
    注销连接

-   `GetConn(fd int) (net.Conn, bool)`
    获取连接

-   `CloseConn(fd int) error`
    关闭指定连接

-   `CloseAll()`
    关闭所有连接

#### 数据操作

-   `SendBytes(fd int, data []byte) error`
    通过虚拟 fd 发送数据

-   `GetConnInfo(fd int) *ConnectOption`
    获取连接的完整信息（包含本地和远程两端）

-   ~~`GetConnPeerInfo(fd int) *ConnectOption`~~（已废弃）
    获取对端信息，建议使用 `GetConnInfo`

-   ~~`GetConnLocalInfo(fd int) *ConnectOption`~~（已废弃）
    获取本地信息，建议使用 `GetConnInfo`

#### 查询

-   `GetConnType(fd int) (ConnectionType, bool)`
    获取连接类型（服务端/客户端）

-   `GetAllFds() []int`
    获取所有活跃的虚拟 fd

-   `GetConnCount() int`
    获取连接总数

## 回调说明

### OnConnected

**触发时机**：连接建立成功时

**参数**：

-   `fd`：虚拟文件描述符（服务端: ≥1000，客户端: ≥2000）
-   `connType`：连接类型（`ConnectionTypeServer` 或 `ConnectionTypeClient`）
-   `connOpt`：连接信息（包含本地和远程两端的完整信息）
    -   `connOpt.LocalSocket`：本地地址和端口
    -   `connOpt.RemoteSocket`：远程地址和端口
    -   `connOpt.NetConn`：底层网络连接

**说明**：

-   **服务端**：`LocalSocket` 是服务端监听地址，`RemoteSocket` 是客户端地址，`connType` 为 `ConnectionTypeServer`
-   **客户端**：`LocalSocket` 是客户端本地地址（端口通常为系统随机分配），`RemoteSocket` 是服务端地址，`connType` 为 `ConnectionTypeClient`

### OnDisconnected

**触发时机**：连接断开时（主动或被动）

**参数**：

-   `fd`：断开连接的虚拟 fd
-   `connType`：连接类型（`ConnectionTypeServer` 或 `ConnectionTypeClient`）

### OnDataReceived

**触发时机**：收到数据时

**参数**：

-   `fd`：接收数据的虚拟 fd
-   `connType`：连接类型（`ConnectionTypeServer` 或 `ConnectionTypeClient`）
-   `buf`：数据缓冲区
-   `used`：缓冲区中已使用的字节数

**返回值**：

-   `> 0`：已处理的字节数，未处理的数据将保留在缓冲区
-   `0`：暂不处理，等待更多数据
-   `-1`：处理失败，连接将被关闭

**示例**：

```go
OnDataReceived: func(fd int, connType ConnectionType, buf []byte, used int) int {
    // 假设协议格式：2字节长度 + N字节数据
    if used < 2 {
        return 0 // 等待更多数据
    }

    length := int(buf[0])<<8 | int(buf[1])
    if used < 2+length {
        return 0 // 包不完整，等待
    }

    // 处理完整的数据包
    data := buf[2 : 2+length]
    fmt.Printf("收到完整数据包 (fd=%d, type=%v): %s\n", fd, connType, string(data))

    return 2 + length // 返回已处理的字节数
}
```

## 常量说明

```go
const (
    DefaultBufSize        = 1536                // 默认缓冲区大小
    ServerFdStart         = 1000                // 服务端虚拟fd起始值
    ClientFdStart         = 2000                // 客户端虚拟fd起始值
    DefaultConnectTimeout = 5 * time.Second     // 默认连接超时
)
```

## 连接类型

```go
const (
    ConnectionTypeServer ConnectionType = iota  // 服务端连接
    ConnectionTypeClient                        // 客户端连接
)
```

## 注意事项

1. **OnDataReceived 回调必须正确返回已处理的字节数**，否则可能导致数据混乱或内存泄漏

2. **缓冲区溢出保护**：当未处理数据累积超过缓冲区大小时，连接将自动关闭

3. **虚拟 fd 范围**：

    - 服务端连接：1000-1999
    - 客户端连接：2000-2999
    - 如需支持更多连接，可修改起始值常量

4. **线程安全**：所有公共方法均为线程安全，可在多个 goroutine 中并发调用

5. **连接关闭**：客户端的 `Close()` 方法会触发 `OnDisconnected` 回调

6. **共享管理器**：服务端和客户端可以共用同一个 `ConnectionManager`，便于统一管理和监控所有连接

## 测试

运行测试：

```bash
cd pkg/utils/tcp_connection
go test -v
```

测试覆盖：

-   ✅ 客户端完整流程测试
-   ✅ 带选项的连接测试
-   ✅ 虚拟 fd 分配测试
-   ✅ 共享连接管理器测试
-   ✅ 错误处理测试
