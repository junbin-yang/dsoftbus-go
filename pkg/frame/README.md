# Frame 模块

Frame（框架）模块是 DSoftBus-Go 的核心框架层，负责统一管理和协调各个子模块的初始化、运行和销毁。

## 功能职责

Frame 模块作为分布式软总线的基础框架，提供：

1. **统一初始化接口**: 协调所有模块的启动顺序
2. **模块生命周期管理**: 管理各模块的初始化和反初始化
3. **全局状态维护**: 维护软总线的整体运行状态

## 架构设计

```
Frame (框架层)
    ├── InitSoftBusServer()
    │   ├── Bus Center Init
    │   ├── Discovery Init
    │   ├── Authentication Init
    │   └── Transmission Init
    │
    └── DeinitSoftBusServer()
        └── 按相反顺序反初始化所有模块
```

## 核心接口

### InitSoftBusServer

初始化软总线服务器，按顺序启动所有模块：

```go
func InitSoftBusServer() error
```

**初始化顺序**:
1. Bus Center (组网管理)
2. Discovery (设备发现)
3. Authentication (设备认证)
4. Transmission (数据传输)

### GetServerIsInit

获取服务器初始化状态：

```go
func GetServerIsInit() bool
```

### DeinitSoftBusServer

反初始化软总线服务器，按相反顺序关闭所有模块：

```go
func DeinitSoftBusServer()
```

## 与 C 代码对应关系

| Go 接口 | C 接口 | 文件位置 |
|---------|--------|----------|
| `InitSoftBusServer()` | `InitSoftBusServer()` | `core/frame/common/src/softbus_server_frame.c:114` |
| `GetServerIsInit()` | `GetServerIsInit()` | `core/frame/common/src/softbus_server_frame.c:59` |
| `DeinitSoftBusServer()` | `ServerModuleDeinit()` | `core/frame/common/src/softbus_server_frame.c:45` |

## 使用示例

### 基本使用

```go
import "github.com/junbin-yang/dsoftbus-go/pkg/frame"

func main() {
    // 初始化软总线框架
    if err := frame.InitSoftBusServer(); err != nil {
        log.Fatalf("初始化失败: %v", err)
    }
    defer frame.DeinitSoftBusServer()

    // 检查初始化状态
    if frame.GetServerIsInit() {
        log.Println("软总线框架已就绪")
    }

    // 使用各个模块...
}
```

### 在 CLI 工具中使用

```go
func (c *CLI) Initialize() error {
    // 使用frame统一初始化接口
    if err := frame.InitSoftBusServer(); err != nil {
        return fmt.Errorf("初始化软总线框架失败: %v", err)
    }

    // 注册回调等应用层逻辑
    // ...

    return nil
}

func (c *CLI) Shutdown() {
    // 使用frame统一反初始化接口
    frame.DeinitSoftBusServer()
}
```

## 模块依赖关系

```
Frame
  ├── Bus Center (pkg/bus_center)
  │   └── 组网管理、节点信息维护
  │
  ├── Discovery (pkg/discovery)
  │   └── 设备发现、CoAP 协议
  │
  ├── Authentication (pkg/authentication)
  │   ├── Device Auth (pkg/device_auth)
  │   └── HiChain 认证协议
  │
  └── Transmission (pkg/transmission)
      └── 数据传输、通道管理
```

## 初始化流程

```
1. InitSoftBusServer()
   ↓
2. busCenterServerInit()
   - 启动 Bus Center
   - 初始化网络账本
   ↓
3. discServerInit()
   - 启动 Discovery 服务
   - 加载设备配置
   ↓
4. authInit()
   - 初始化 DeviceAuth
   - 准备认证服务
   ↓
5. transServerInit()
   - 初始化 Transmission
   - 注册通道监听器
   ↓
6. 框架初始化完成
```

## 反初始化流程

```
1. DeinitSoftBusServer()
   ↓
2. serverModuleDeinit()
   ↓
3. 按相反顺序关闭:
   - Transmission
   - Authentication
   - Device Auth
   - Discovery
   - Bus Center
   ↓
4. 框架关闭完成
```

## 错误处理

Frame 模块采用 fail-fast 策略：

- 任何模块初始化失败，立即停止并回滚已初始化的模块
- 返回详细的错误信息，便于定位问题
- 保证资源正确释放，避免泄漏

```go
if err := frame.InitSoftBusServer(); err != nil {
    // 错误信息包含具体失败的模块
    log.Fatalf("初始化失败: %v", err)
}
```

## 线程安全

Frame 模块使用互斥锁保证线程安全：

- `InitSoftBusServer()` 和 `DeinitSoftBusServer()` 可以安全地并发调用
- 重复初始化会被忽略（幂等性）
- 全局状态 `gIsInit` 受锁保护

## 与其他模块的集成

### Bus Center 集成

```go
// Frame 负责启动 Bus Center
busCenter := bus_center.GetInstance()
busCenter.Start()
```

### Discovery 集成

```go
// Frame 负责初始化 Discovery 服务
service.InitService()
```

### Authentication 集成

```go
// Frame 负责初始化 DeviceAuth
device_auth.InitDeviceAuthService()
```

### Transmission 集成

```go
// Frame 负责初始化 Transmission
transmission.TransServerInit()
```

## 设计原则

1. **单一职责**: Frame 只负责模块的生命周期管理
2. **依赖倒置**: 各模块通过接口与 Frame 交互
3. **开闭原则**: 易于扩展新模块，无需修改现有代码
4. **最小知识**: Frame 只知道模块的初始化接口，不关心内部实现

## 未来扩展

- [ ] 支持模块热插拔
- [ ] 添加模块健康检查
- [ ] 支持模块依赖声明
- [ ] 添加模块初始化超时控制
- [ ] 支持模块初始化并发优化

## 参考

- HarmonyOS C 代码: `core/frame/common/src/softbus_server_frame.c`
- 项目状态: [PROJECT_STATUS.md](../../PROJECT_STATUS.md)
