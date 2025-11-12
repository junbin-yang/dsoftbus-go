# Discovery Service包

提供基于CoAP协议的服务发布和管理能力，是OpenHarmony SoftBus设备发现功能的上层封装。

## 功能概述

本模块在CoAP设备发现的基础上，提供了完整的服务发布管理功能，包括：

- **服务发布管理**：支持多模块同时发布服务
- **设备信息管理**：统一管理本地设备的元数据
- **能力位图管理**：合并多个服务的能力位图
- **网络接口监控**：自动跟踪网络变化
- **发现回调**：灵活的设备发现事件通知

## 架构设计

### 核心组件

```
service/
├── discovery_service.go  # 服务发布管理API
├── coap_service.go       # CoAP服务封装和全局设备信息管理
└── coap_device.go        # 设备类型定义和映射
```

### 模块关系

```
┌─────────────────────────────────────────────┐
│           Application Layer                  │
│  (PublishService, UnPublishService, etc.)   │
└────────────────┬────────────────────────────┘
                 │
┌────────────────▼────────────────────────────┐
│        Discovery Service Layer               │
│  • Service Publication Management            │
│  • Capability Bitmap Aggregation             │
│  • Device Info Management                    │
└────────────────┬────────────────────────────┘
                 │
┌────────────────▼────────────────────────────┐
│          CoAP Service Layer                  │
│  • Provider Registration                     │
│  • Network Interface Monitoring              │
│  • Discovery Callback Handling               │
└────────────────┬────────────────────────────┘
                 │
┌────────────────▼────────────────────────────┐
│           CoAP Protocol Layer                │
│  (pkg/discovery/coap)                        │
└──────────────────────────────────────────────┘
```

## API文档

### 初始化

#### InitService()

初始化服务发布功能，启动CoAP发现监听。

```go
func InitService() error
```

**功能：**
1. 初始化内部模块数组
2. 调用底层CoAP初始化
3. 从配置文件读取设备信息
4. 注册设备信息到CoAP层

**使用示例：**
```go
import (
    "log"
    "github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
)

func main() {
    if err := service.InitService(); err != nil {
        log.Fatalf("初始化服务失败: %v", err)
    }
    log.Println("服务初始化成功")
}
```

### 服务发布

#### PublishService()

在局域网内发布服务，让其他设备可以发现。

```go
func PublishService(moduleName string, info *PublishInfo) (int, error)
```

**参数：**
- `moduleName`：上层服务的模块名（最大64字符）
- `info`：服务发布信息（见PublishInfo结构）

**返回值：**
- `int`：发布成功的服务ID（publishId）
- `error`：错误信息

**PublishInfo 结构：**
```go
type PublishInfo struct {
    PublishId      int            // 服务发布ID（必须>0）
    Mode           DiscoverMode   // 服务发布模式（被动/主动）
    Medium         ExchangeMedium // 服务发布介质（目前仅支持CoAP）
    Freq           ExchangeFreq   // 服务发布频率（预留）
    Capability     string         // 服务能力名称（见能力映射表）
    CapabilityData []byte         // 服务能力数据（最大64字节）
}
```

**使用示例：**
```go
publishInfo := &service.PublishInfo{
    PublishId:      1,
    Mode:           service.DiscoverModePassive,
    Medium:         service.ExchangeMediumCOAP,
    Freq:           service.ExchangeFreqLow,
    Capability:     "ddmpCapability",
    CapabilityData: []byte("customData"),
}

publishId, err := service.PublishService("myModule", publishInfo)
if err != nil {
    log.Fatalf("发布服务失败: %v", err)
}
log.Printf("服务发布成功，ID=%d\n", publishId)
```

#### UnPublishService()

取消发布服务。

```go
func UnPublishService(moduleName string, publishId int) (bool, error)
```

**参数：**
- `moduleName`：上层服务的模块名
- `publishId`：要取消发布的服务ID

**返回值：**
- `bool`：是否成功取消
- `error`：错误信息

**使用示例：**
```go
success, err := service.UnPublishService("myModule", 1)
if err != nil {
    log.Printf("取消发布失败: %v", err)
} else if success {
    log.Println("服务已取消发布")
}
```

### 设备信息管理

#### SetCommonDeviceInfo()

设置通用设备信息（如设备ID、类型、名称等）。

```go
func SetCommonDeviceInfo(devInfo []CommonDeviceInfo) (bool, error)
```

**参数：**
- `devInfo`：设备信息数组

**CommonDeviceInfo 结构：**
```go
type CommonDeviceInfo struct {
    Key   CommonDeviceKey // 信息类型
    Value string          // 信息内容
}

// 设备信息键
const (
    CommonDeviceKeyDevID   CommonDeviceKey = 0 // 设备ID
    CommonDeviceKeyDevType CommonDeviceKey = 1 // 设备类型
    CommonDeviceKeyDevName CommonDeviceKey = 2 // 设备名称
)
```

**使用示例：**
```go
devInfo := []service.CommonDeviceInfo{
    {
        Key:   service.CommonDeviceKeyDevName,
        Value: "MyNewDeviceName",
    },
    {
        Key:   service.CommonDeviceKeyDevType,
        Value: "PC",
    },
}

success, err := service.SetCommonDeviceInfo(devInfo)
if err != nil {
    log.Printf("设置设备信息失败: %v", err)
} else if success {
    log.Println("设备信息已更新")
}
```

### 高级功能

#### SetDiscoverCallback()

设置设备发现回调函数，在发现新设备时触发。

```go
func SetDiscoverCallback(callback func(dev *coap.DeviceInfo))
```

**使用示例：**
```go
service.SetDiscoverCallback(func(dev *coap.DeviceInfo) {
    log.Printf("发现新设备:")
    log.Printf("  名称: %s", dev.DeviceName)
    log.Printf("  ID: %s", dev.DeviceId)
    log.Printf("  类型: 0x%02X", dev.DeviceType)
    log.Printf("  IP: %s", dev.NetChannelInfo.Network.IP.String())
    log.Printf("  服务数据: %s", dev.ServiceData)
})
```

#### UpdateAuthPortToCoapService()

更新设备的认证端口信息。

```go
func UpdateAuthPortToCoapService(new_port int)
```

**使用示例：**
```go
// 当认证服务器启动后，更新端口信息
authPort := 6666
service.UpdateAuthPortToCoapService(authPort)
```

## 数据结构

### 服务能力映射

系统预定义了以下服务能力：

| 能力名称 | 位图值 | 说明 |
|---------|-------|------|
| hicall | 0 | MeeTime通话 |
| profile | 1 | 智能域视频反向连接 |
| homevisionPic | 2 | Vision图库 |
| castPlus | 3 | Cast+投屏 |
| aaCapability | 4 | Vision输入法 |
| dvKit | 5 | 设备虚拟化工具包 |
| ddmpCapability | 6 | 分布式中间件 |

### 设备类型

```go
const (
    DeviceTypeUnknown DeviceType = 0x00 // 未知设备
    DeviceTypePhone   DeviceType = 0x0E // 智能手机
    DeviceTypePad     DeviceType = 0x11 // 平板
    DeviceTypeTV      DeviceType = 0x9C // 智能电视
    DeviceTypePC      DeviceType = 0x0C // 电脑
    DeviceTypeAudio   DeviceType = 0x0A // 音频设备
    DeviceTypeCar     DeviceType = 0x83 // 车载设备
    DeviceTypeL0      DeviceType = 0xF1 // 小型设备L0
    DeviceTypeL1      DeviceType = 0xF2 // 小型设备L1
)
```

### 发布模式

```go
const (
    DiscoverModePassive DiscoverMode = 0x55 // 被动模式
    DiscoverModeActive  DiscoverMode = 0xAA // 主动模式
)
```

### 交换介质

```go
const (
    ExchangeMediumAuto ExchangeMedium = 0 // 自动选择
    ExchangeMediumBLE  ExchangeMedium = 1 // 蓝牙
    ExchangeMediumCOAP ExchangeMedium = 2 // Wi-Fi（CoAP）
    ExchangeMediumUSB  ExchangeMedium = 3 // USB
)
```

## 完整使用示例

### 示例1：基础服务发布

```go
package main

import (
    "log"
    "time"

    "github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
    "github.com/junbin-yang/dsoftbus-go/pkg/discovery/coap"
)

func main() {
    // 1. 初始化服务
    if err := service.InitService(); err != nil {
        log.Fatalf("初始化失败: %v", err)
    }
    log.Println("服务初始化成功")

    // 2. 设置设备发现回调
    service.SetDiscoverCallback(func(dev *coap.DeviceInfo) {
        log.Printf("[发现设备] %s (%s) at %s",
            dev.DeviceName,
            dev.DeviceId,
            dev.NetChannelInfo.Network.IP.String())
    })

    // 3. 发布服务
    publishInfo := &service.PublishInfo{
        PublishId:      1,
        Mode:           service.DiscoverModePassive,
        Medium:         service.ExchangeMediumCOAP,
        Freq:           service.ExchangeFreqLow,
        Capability:     "ddmpCapability",
        CapabilityData: []byte("port:8080"),
    }

    _, err := service.PublishService("myApp", publishInfo)
    if err != nil {
        log.Fatalf("发布服务失败: %v", err)
    }
    log.Println("服务发布成功")

    // 4. 保持运行
    select {}
}
```

### 示例2：多服务发布

```go
package main

import (
    "log"

    "github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
)

func main() {
    // 初始化
    if err := service.InitService(); err != nil {
        log.Fatalf("初始化失败: %v", err)
    }

    // 发布文件传输服务
    fileTransferInfo := &service.PublishInfo{
        PublishId:      1,
        Mode:           service.DiscoverModePassive,
        Medium:         service.ExchangeMediumCOAP,
        Capability:     "dvKit",
        CapabilityData: []byte("file-transfer"),
    }
    _, err := service.PublishService("fileModule", fileTransferInfo)
    if err != nil {
        log.Printf("发布文件传输服务失败: %v", err)
    }

    // 发布屏幕共享服务
    screenShareInfo := &service.PublishInfo{
        PublishId:      2,
        Mode:           service.DiscoverModePassive,
        Medium:         service.ExchangeMediumCOAP,
        Capability:     "castPlus",
        CapabilityData: []byte("screen-share"),
    }
    _, err = service.PublishService("screenModule", screenShareInfo)
    if err != nil {
        log.Printf("发布屏幕共享服务失败: %v", err)
    }

    log.Println("所有服务已发布")
    select {}
}
```

### 示例3：动态更新设备信息

```go
package main

import (
    "log"
    "time"

    "github.com/junbin-yang/dsoftbus-go/pkg/discovery/service"
)

func main() {
    // 初始化
    if err := service.InitService(); err != nil {
        log.Fatalf("初始化失败: %v", err)
    }

    // 定时更新设备名称
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    counter := 0
    for range ticker.C {
        counter++
        newName := fmt.Sprintf("MyDevice_%d", counter)

        devInfo := []service.CommonDeviceInfo{
            {
                Key:   service.CommonDeviceKeyDevName,
                Value: newName,
            },
        }

        success, err := service.SetCommonDeviceInfo(devInfo)
        if err != nil {
            log.Printf("更新设备名称失败: %v", err)
        } else if success {
            log.Printf("设备名称已更新为: %s", newName)
        }
    }
}
```

## 工作流程

### 服务发布流程

```
1. 应用调用 InitService()
   └─> 初始化模块数组
       └─> 调用 DiscCoapInit()
           ├─> 启动网络管理器
           ├─> 注册 Provider 回调
           └─> 调用 coap.CoapInitDiscovery()
               └─> 启动 UDP 监听

2. 应用调用 PublishService()
   └─> 验证参数
       └─> 查找空闲模块槽
           └─> 解析能力字符串到位图
               └─> 保存模块信息
                   └─> 调用 updateCoapService()
                       ├─> 合并所有模块的能力位图
                       ├─> 拼接 serviceData
                       └─> 更新全局设备信息

3. CoAP 层响应设备发现请求
   └─> 调用 deviceInfoProvider()
       └─> 返回包含所有服务能力的设备信息
```

### 能力位图合并

当多个模块同时发布服务时，系统会自动合并它们的能力位图：

```
Module 1: dvKit (bit 5)           → 0000 0000 0010 0000 (32)
Module 2: castPlus (bit 3)        → 0000 0000 0000 1000 (8)
Module 3: ddmpCapability (bit 6)  → 0000 0000 0100 0000 (64)
────────────────────────────────────────────────────────
Merged Bitmap (OR operation)      → 0000 0000 0110 1000 (104)
```

### ServiceData 格式

服务数据使用逗号分隔的键值对格式：

```
port:<authPort>,<module1_data>,<module2_data>,...

示例：
"port:6666,file-transfer,screen-share"
```

## 配置文件

服务初始化时会从配置文件读取设备信息，配置文件格式参考：

```yaml
device:
  name: "MyDevice"
  uuid: "1234567890ABCDEF"
  interface: "eth0"
  type: "PC"
```

## 注意事项

### 1. 模块管理

- **模块数量限制**：最多支持3个模块同时发布服务
- **模块槽复用**：取消发布后，模块槽会被释放，可供新服务使用
- **自动反初始化**：当所有模块都取消发布时，CoAP服务会自动停止

### 2. 能力管理

- **能力位图**：每个服务能力对应一个位（0-15），最多支持16种能力
- **能力合并**：多个服务的能力位图使用OR运算合并
- **能力数据**：每个服务可携带最多64字节的自定义数据

### 3. 网络管理

- **接口优先级**：优先使用配置文件指定的网络接口
- **自动回退**：指定接口不可用时，自动使用默认接口
- **IPv4支持**：目前仅支持IPv4地址

### 4. 线程安全

- **并发安全**：所有公共API都使用互斥锁保护，支持并发调用
- **回调线程**：设备发现回调在单独的goroutine中执行
- **Provider调用**：Provider回调函数应避免长时间阻塞

## 错误处理

### 常见错误

| 错误 | 原因 | 解决方法 |
|-----|------|---------|
| 参数错误 | 模块名或能力参数无效 | 检查参数长度和有效性 |
| 重复发布的服务 | 同一模块和ID已发布 | 使用不同的publishId或先取消发布 |
| 无法发布更多服务 | 模块槽已满（3个） | 取消不需要的服务或等待槽释放 |
| 解析服务能力失败 | 能力名称不存在 | 使用预定义的能力名称 |
| 服务未发布 | 尝试取消未发布的服务 | 检查模块名和publishId |
| 配置文件错误 | 必需字段缺失 | 补全配置文件中的字段 |
| 网络管理器初始化失败 | 网络接口问题 | 检查网络配置和权限 |

## 性能指标

- **初始化时间**：< 100ms
- **服务发布时间**：< 10ms
- **设备发现延迟**：< 1s（取决于网络状况）
- **内存占用**：< 1MB（基础功能）
- **并发服务数**：3个（可通过修改MAX_MODULE_COUNT扩展）

## 调试建议

### 启用详细日志

```go
import log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"

// 设置日志级别
log.SetLevel(log.DebugLevel)
```

### 检查设备信息

```go
// 获取当前设备信息
devInfo := service.DiscCoapGetDeviceInfo()
log.Printf("当前设备信息: %+v", devInfo)
```

### 监控网络接口

```go
// 获取本地网络信息
localIP, mask, err := service.GetLocalNetworkInfo()
if err != nil {
    log.Printf("获取网络信息失败: %v", err)
} else {
    log.Printf("本地IP: %s, 掩码: %s", localIP, mask)
}
```

## 与CoAP包的关系

Service包是CoAP包的高层封装：

```
┌───────────────────────────────────────┐
│  service 包 (High-Level API)          │
│  ├─ 服务发布管理                       │
│  ├─ 能力位图合并                       │
│  ├─ 设备信息管理                       │
│  └─ 配置文件支持                       │
└─────────────┬─────────────────────────┘
              │ 调用
┌─────────────▼─────────────────────────┐
│  coap 包 (Low-Level Protocol)         │
│  ├─ CoAP 协议编解码                    │
│  ├─ UDP 套接字管理                     │
│  ├─ 设备发现核心逻辑                   │
│  └─ JSON 负载处理                      │
└───────────────────────────────────────┘
```

**使用建议：**
- 应用层开发建议使用 **service 包**，API简单易用
- 需要自定义协议或特殊需求时使用 **coap 包**

## 相关文档

- [CoAP包文档](../coap/README.md)
- [OpenHarmony SoftBus文档](https://gitee.com/openharmony/communication_softbus)

## 待办事项

- [ ] 实现基础服务HiChain认证模块

## 许可证

Apache License 2.0
