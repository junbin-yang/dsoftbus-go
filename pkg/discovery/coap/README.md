# CoAP设备发现包

提供基于CoAP（Constrained Application Protocol）协议的设备发现能力，实现OpenHarmony SoftBus的局域网设备自动发现功能。

## 功能概述

本模块实现了完整的CoAP协议栈和设备发现机制，支持：

- **设备自动发现**：监听UDP 5684端口，自动响应设备发现请求
- **设备广播**：构建并发送设备发现广播包
- **设备信息交换**：使用JSON格式交换设备元数据
- **HarmonyOS兼容**：基于OpenHarmony v4.1.4的设备发现协议

## 架构设计

### 核心组件

```
coap/
├── coap_common.go      # CoAP协议基础定义（头部、选项、数据包结构）
├── coap_adapter.go     # CoAP协议编解码实现
├── coap_socket.go      # UDP套接字管理（服务器/客户端）
├── coap_discover.go    # 设备发现核心逻辑
├── json_payload.go     # 设备信息JSON序列化/反序列化
└── coap_provider.go    # 回调提供者接口
```

### 工作流程

```
1. 初始化阶段
   └─> RegisterProviders() 注册回调函数
       └─> CoapInitDiscovery() 初始化UDP服务器
           └─> 启动监听goroutine

2. 接收设备发现请求
   └─> coapReadLoop() 循环接收UDP数据包
       └─> COAP_SoftBusDecode() 解码CoAP协议
           └─> postServiceDiscover() 处理设备发现
               ├─> ParseServiceDiscover() 解析设备信息
               ├─> discoverCallbackProvider() 触发回调
               └─> coapResponseService() 发送响应

3. 主动发现设备
   └─> BuildDiscoverPacket() 构建CoAP发现包
       └─> CoapCreateUDPClient() 创建客户端
           └─> CoapSocketSend() 发送广播/单播
```

## API文档

### 初始化和注册

#### RegisterProviders()

注册设备信息提供者和回调函数。

```go
func RegisterProviders(p Providers)

type Providers struct {
    LocalDeviceInfo func() *DeviceInfo      // 获取本地设备信息
    LocalIPString   func() (string, error)  // 获取本地IP地址
    Discover        func(dev *DeviceInfo)   // 设备发现回调
}
```

**使用示例：**
```go
coap.RegisterProviders(coap.Providers{
    LocalDeviceInfo: func() *coap.DeviceInfo {
        return &coap.DeviceInfo{
            DeviceId:         "YOUR_DEVICE_ID",
            DeviceName:       "MyDevice",
            DeviceType:       0x0C, // PC
            Version:          "1.0.0",
            CapabilityBitmap: []uint16{192},
        }
    },
    LocalIPString: func() (string, error) {
        return "192.168.1.100", nil
    },
    Discover: func(dev *coap.DeviceInfo) {
        fmt.Printf("发现设备: %s (%s)\n", dev.DeviceName, dev.DeviceId)
    },
})
```

#### CoapInitDiscovery()

初始化CoAP发现服务，开始监听UDP 5684端口。

```go
func CoapInitDiscovery() int
```

**返回值：**
- `NSTACKX_EOK (0)`：成功
- `NSTACKX_EFAILED (-1)`：失败

**使用示例：**
```go
if coap.CoapInitDiscovery() != 0 {
    log.Fatal("初始化CoAP发现服务失败")
}
```

### 主动发现

#### BuildDiscoverPacket()

构建设备发现请求包，用于主动发现局域网内的设备。

```go
func BuildDiscoverPacket(subnetIP string) ([]byte, error)
```

**参数：**
- `subnetIP`：目标子网IP（如"192.168.1.255"用于广播）

**返回值：**
- `[]byte`：编码后的CoAP数据包
- `error`：错误信息

**使用示例：**
```go
// 构建广播包
packet, err := coap.BuildDiscoverPacket("192.168.1.255")
if err != nil {
    log.Fatalf("构建发现包失败: %v", err)
}

// 创建UDP客户端并发送
dst := &net.UDPAddr{
    IP:   net.ParseIP("192.168.1.255"),
    Port: coap.COAP_DEFAULT_PORT,
}
client, err := coap.CoapCreateUDPClient(dst)
if err != nil {
    log.Fatalf("创建UDP客户端失败: %v", err)
}
defer coap.CoapCloseSocket(client)

_, err = coap.CoapSocketSend(client, packet)
if err != nil {
    log.Fatalf("发送发现包失败: %v", err)
}
```

### 套接字管理

#### CoapCreateUDPServer()

创建UDP服务器套接字，用于接收设备发现请求。

```go
func CoapCreateUDPServer(addr *net.UDPAddr) (*SocketInfo, error)
```

#### CoapCreateUDPClient()

创建UDP客户端套接字，用于发送设备发现请求。

```go
func CoapCreateUDPClient(dstAddr *net.UDPAddr) (*SocketInfo, error)
```

#### CoapSocketSend()

通过UDP套接字发送数据。

```go
func CoapSocketSend(socket *SocketInfo, data []byte) (int, error)
```

#### CoapSocketRecv()

从UDP套接字接收数据。

```go
func CoapSocketRecv(socket *SocketInfo, buf []byte) (int, *net.UDPAddr, error)
```

#### CoapCloseSocket()

关闭UDP套接字。

```go
func CoapCloseSocket(socket *SocketInfo) error
```

### JSON数据处理

#### PrepareServiceDiscover()

生成设备发现JSON负载。

```go
func PrepareServiceDiscover(isBroadcast bool) (string, error)
```

**参数：**
- `isBroadcast`：是否为广播模式（广播时包含coapUri字段）

**返回值：**
- `string`：JSON格式的设备信息
- `error`：错误信息

#### ParseServiceDiscover()

解析接收到的设备信息JSON。

```go
func ParseServiceDiscover(buf []byte, out *DeviceInfo) (string, error)
```

**参数：**
- `buf`：JSON数据字节数组
- `out`：输出设备信息结构体

**返回值：**
- `string`：远程设备的coapUri（用于响应）
- `error`：错误信息

#### CleanJSONData()

清理JSON数据中的控制字符（如`\x00`）。

```go
func CleanJSONData(payload []byte) []byte
```

**用途：**处理来自C语言实现的HarmonyOS设备的JSON数据，去除字符串终止符等无效字符。

#### FormatDeviceID()

格式化设备ID为HarmonyOS兼容的JSON格式。

```go
func FormatDeviceID(udid string) string
```

**转换规则：**
- 输入：`"1234567890ABCDEF"`
- 输出：`{"UDID":"1234567890ABCDEF"}`

#### ExtractDeviceID()

从JSON格式的设备ID中提取原始UDID。

```go
func ExtractDeviceID(deviceID string) string
```

**转换规则：**
- 输入：`{"UDID":"1234567890ABCDEF"}`
- 输出：`"1234567890ABCDEF"`

## 数据结构

### DeviceInfo

设备信息结构体，包含设备的所有元数据。

```go
type DeviceInfo struct {
    DeviceId         string      // 设备ID（UDID）
    DeviceName       string      // 设备名称
    DeviceType       uint8       // 设备类型（0x0C=PC, 0x0E=手机等）
    Version          string      // HiCom版本号
    Mode             uint8       // 请求模式（0x55=被动, 0xAA=主动）
    DeviceHash       string      // 设备哈希值
    ServiceData      string      // 服务数据（如"port:6666"）
    CapabilityBitmap []uint16    // 能力位图（不能为空）
    NetChannelInfo   NetChannelInfo // 网络信息
}
```

### COAP_Packet

CoAP数据包结构体。

```go
type COAP_Packet struct {
    Protocol   COAP_ProtocolTypeEnum // 协议类型（UDP/TCP）
    Len        uint32                // 数据包长度
    Header     COAP_Header           // 头部
    Token      COAP_Buffer           // Token
    OptionsNum uint8                 // 选项数量
    Options    [16]COAP_Option       // 选项数组
    Payload    COAP_Buffer           // 负载数据
}
```

## 协议详解

### CoAP消息格式

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|Ver| T |  TKL  |      Code     |          Message ID           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Token (if any, TKL bytes) ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Options (if any) ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|1 1 1 1 1 1 1 1|    Payload (if any) ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

**字段说明：**
- **Ver (2 bits)**：版本号，固定为1
- **T (2 bits)**：消息类型（0=CON, 1=NONCON, 2=ACK, 3=RESET）
- **TKL (4 bits)**：Token长度（0-8字节）
- **Code (8 bits)**：方法码（1=GET, 2=POST, 3=PUT, 4=DELETE）
- **Message ID (16 bits)**：消息ID
- **Options**：选项列表（URI-Host、URI-Path等）
- **Payload**：JSON格式的设备信息

### 设备发现JSON格式

```json
{
  "deviceId": "{\"UDID\":\"1234567890ABCDEF\"}",
  "devicename": "MyDevice",
  "type": 12,
  "hicomversion": "1.0.0",
  "mode": 0,
  "deviceHash": "0",
  "serviceData": "port:6666",
  "wlanIp": "192.168.1.100",
  "capabilityBitmap": [192],
  "coapUri": "coap://192.168.1.100/device_discover"
}
```

**字段说明：**
- **deviceId**：设备ID（HarmonyOS使用JSON包装格式）
- **devicename**：设备名称
- **type**：设备类型（0x0C=PC, 0x0E=手机, 0x9C=电视等）
- **hicomversion**：HiCom协议版本
- **mode**：发现模式（0=被动, 1=主动）
- **deviceHash**：设备哈希值
- **serviceData**：服务数据（包含认证端口等信息）
- **wlanIp**：无线局域网IP地址
- **capabilityBitmap**：能力位图数组
- **coapUri**：CoAP URI（仅在广播时包含，用于响应）

## 常量定义

### 端口和缓冲区

```go
const (
    COAP_DEFAULT_PORT = 5684  // CoAP默认UDP端口
    COAP_MAX_PDU_SIZE = 1024  // 最大PDU长度
    COAP_TTL_VALUE    = 64    // 默认TTL值
)
```

### CoAP选项编号

```go
const (
    DISCOVERY_MSG_URI_HOST = 3   // URI-Host选项
    DISCOVERY_MSG_URI_PATH = 11  // URI-Path选项
    COAP_MAX_OPTION        = 16  // 最大选项数量
)
```

### 错误码

```go
const (
    DISCOVERY_ERR_SUCCESS              = 0  // 成功
    DISCOVERY_ERR_HEADER_INVALID_SHORT = 1  // 头部过短
    DISCOVERY_ERR_VER_INVALID          = 2  // 版本无效
    DISCOVERY_ERR_TOKEN_INVALID_SHORT  = 3  // Token过短
    DISCOVERY_ERR_OPT_INVALID_BIG      = 7  // 选项过多
    DISCOVERY_ERR_OPT_INVALID_LEN      = 8  // 选项长度无效
    DISCOVERY_ERR_OPT_INVALID_DELTA    = 11 // 选项Delta无效
    DISCOVERY_ERR_INVALID_PKT          = 15 // 数据包无效
    DISCOVERY_ERR_BAD_REQ              = 21 // 错误请求
)
```

## 完整使用示例

### 示例1：被动发现（接收广播）

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
)

func main() {
    // 1. 注册回调函数
    coap.RegisterProviders(coap.Providers{
        LocalDeviceInfo: func() *coap.DeviceInfo {
            return &coap.DeviceInfo{
                DeviceId:         "1234567890ABCDEF",
                DeviceName:       "MyGoDevice",
                DeviceType:       0x0C, // PC
                Version:          "1.0.0",
                Mode:             0,
                DeviceHash:       "0",
                ServiceData:      "port:6666",
                CapabilityBitmap: []uint16{192},
            }
        },
        LocalIPString: func() (string, error) {
            return "192.168.1.100", nil
        },
        Discover: func(dev *coap.DeviceInfo) {
            fmt.Printf("发现设备: %s (%s) at %s\n",
                dev.DeviceName, dev.DeviceId,
                dev.NetChannelInfo.Network.IP.String())
        },
    })

    // 2. 初始化CoAP发现服务
    if coap.CoapInitDiscovery() != 0 {
        log.Fatal("初始化CoAP发现服务失败")
    }
    fmt.Println("CoAP发现服务已启动，监听端口5684...")

    // 3. 保持运行
    select {}
}
```

### 示例2：主动发现（发送广播）

```go
package main

import (
    "fmt"
    "log"
    "net"
    "time"

    "github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
)

func main() {
    // 1. 注册提供者（同上）
    coap.RegisterProviders(coap.Providers{
        // ...
    })

    // 2. 初始化服务
    if coap.CoapInitDiscovery() != 0 {
        log.Fatal("初始化失败")
    }

    // 3. 定时发送广播
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        // 构建发现包
        packet, err := coap.BuildDiscoverPacket("192.168.1.255")
        if err != nil {
            log.Printf("构建发现包失败: %v", err)
            continue
        }

        // 创建UDP客户端
        dst := &net.UDPAddr{
            IP:   net.ParseIP("192.168.1.255"),
            Port: coap.COAP_DEFAULT_PORT,
        }
        client, err := coap.CoapCreateUDPClient(dst)
        if err != nil {
            log.Printf("创建客户端失败: %v", err)
            continue
        }

        // 发送广播
        n, err := coap.CoapSocketSend(client, packet)
        if err != nil {
            log.Printf("发送失败: %v", err)
        } else {
            fmt.Printf("已发送%d字节的设备发现广播\n", n)
        }

        coap.CoapCloseSocket(client)
    }
}
```

## 注意事项

### 1. HarmonyOS兼容性

- **设备ID格式**：HarmonyOS期望设备ID为JSON格式`{"UDID":"xxx"}`，使用`FormatDeviceID()`进行转换
- **JSON清理**：HarmonyOS（C实现）发送的JSON可能包含`\x00`字符，使用`CleanJSONData()`清理
- **能力位图**：必须包含有效的能力位图，否则HarmonyOS设备无法发现

### 2. 网络配置

- **防火墙**：确保UDP 5684端口未被防火墙阻止
- **组播TTL**：默认TTL为64，适用于局域网环境
- **组播回环**：已禁用组播回环，避免接收自己发送的包

### 3. 性能考虑

- **缓冲区大小**：默认PDU大小为1024字节，足够容纳设备信息
- **并发安全**：内部使用sync.Mutex保护共享资源，支持并发调用
- **goroutine管理**：监听goroutine在`CoapDeinitDiscovery()`时会自动退出

### 4. 错误处理

- 所有公共API都返回错误，调用方应检查并处理错误
- 解码失败的数据包会被静默丢弃，避免影响正常运行
- 网络错误会记录日志，但不会中断监听循环

## 设备类型参考

| 类型码 | 设备类型 | 说明 |
|-------|---------|------|
| 0x00  | Unknown | 未知设备 |
| 0x0C  | PC      | 个人电脑 |
| 0x0E  | Phone   | 智能手机 |
| 0x11  | Pad     | 平板电脑 |
| 0x9C  | TV      | 智能电视 |
| 0x0A  | Audio   | 音频设备 |
| 0x83  | Car     | 车载设备 |
| 0xF1  | L0      | 小型设备L0 |
| 0xF2  | L1      | 小型设备L1 |

## 能力位图参考

| 位值 | 能力名称 | 说明 |
|-----|---------|------|
| 192 | 基础能力 | 设备发现基础能力（必需） |
| 其他 | 待扩展   | 预留给特定服务能力 |

## 相关文档

- [OpenHarmony SoftBus文档](https://gitee.com/openharmony/communication_softbus)
- [CoAP RFC 7252](https://datatracker.ietf.org/doc/html/rfc7252)

## 许可证

Apache License 2.0
