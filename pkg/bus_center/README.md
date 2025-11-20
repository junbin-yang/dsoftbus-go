# Bus Center 模块

Bus Center（总线中心）是 DSoftBus-Go 的组网管理核心模块，负责维护分布式网络中的设备节点信息和网络拓扑。

## 功能特性

- **节点信息管理**: 维护设备节点的基本信息（NetworkID、DeviceID、DeviceName等）
- **网络账本**: 管理所有已发现和已认证的设备节点
- **网络构建器**: 处理设备加入/退出网络的流程
- **事件系统**: 提供节点上线/下线/信息变更的事件通知

## 架构设计

```
BusCenter (总线中心)
    ├── NetLedger (网络账本)
    │   └── 节点信息存储和查询
    └── NetBuilder (网络构建器)
        └── 节点加入/退出流程
```

## 核心组件

### 1. NodeInfo (节点信息)

```go
type NodeInfo struct {
    NetworkID     string      // 网络ID
    DeviceID      string      // 设备ID
    DeviceName    string      // 设备名称
    DeviceType    int         // 设备类型
    Status        NodeStatus  // 节点状态（在线/离线）
    AuthSeq       int64       // 认证序列号
    DiscoveryType string      // 发现类型
    ConnectAddr   string      // 连接地址
    JoinTime      time.Time   // 加入时间
    LastSeen      time.Time   // 最后活跃时间
}
```

### 2. NetLedger (网络账本)

负责存储和管理所有节点信息：

- `AddNode()`: 添加节点
- `RemoveNode()`: 移除节点
- `GetNode()`: 获取节点信息
- `GetAllNodes()`: 获取所有节点
- `GetOnlineNodes()`: 获取在线节点
- `UpdateNodeStatus()`: 更新节点状态

### 3. NetBuilder (网络构建器)

处理节点加入/退出流程：

- `NotifyNodeOnline()`: 通知节点上线
- `NotifyNodeOffline()`: 通知节点下线
- `RegisterCallback()`: 注册事件回调

### 4. BusCenter (总线中心)

统一管理接口：

- `Start()`: 启动总线中心
- `Stop()`: 停止总线中心
- `OnDeviceOnline()`: 设备上线
- `OnDeviceOffline()`: 设备下线
- `GetNodeInfo()`: 获取节点信息
- `GetAllNodes()`: 获取所有节点
- `GetOnlineNodes()`: 获取在线节点
- `RegisterEventCallback()`: 注册事件回调

## 使用示例

### 初始化

```go
import "github.com/junbin-yang/dsoftbus-go/pkg/bus_center"

// 获取BusCenter单例
bc := bus_center.GetInstance()

// 启动
if err := bc.Start(); err != nil {
    log.Fatal(err)
}
defer bc.Stop()
```

### 注册事件回调

```go
bc.RegisterEventCallback(func(event bus_center.LNNEvent, node *bus_center.NodeInfo) {
    switch event {
    case bus_center.EventNodeOnline:
        log.Printf("节点上线: %s", node.DeviceName)
    case bus_center.EventNodeOffline:
        log.Printf("节点下线: %s", node.DeviceName)
    }
})
```

### 设备上线

```go
node := &bus_center.NodeInfo{
    NetworkID:     "device-network-id",
    DeviceID:      "device-id",
    DeviceName:    "MyDevice",
    Status:        bus_center.StatusOnline,
    DiscoveryType: "CoAP",
    ConnectAddr:   "192.168.1.100:6666",
}

if err := bc.OnDeviceOnline(node); err != nil {
    log.Fatal(err)
}
```

### 查询节点

```go
// 获取所有节点
allNodes := bc.GetAllNodes()

// 获取在线节点
onlineNodes := bc.GetOnlineNodes()

// 获取特定节点
node := bc.GetNodeInfo("device-network-id")
```

## 与其他模块的集成

### 与认证模块集成

当设备认证成功后，通知 Bus Center 设备上线：

```go
// 在认证成功回调中
func onAuthOpened(authId int64, deviceInfo *DeviceInfo) {
    bc := bus_center.GetInstance()
    node := &bus_center.NodeInfo{
        NetworkID:     deviceInfo.NetworkID,
        DeviceID:      deviceInfo.DeviceID,
        DeviceName:    deviceInfo.DeviceName,
        Status:        bus_center.StatusOnline,
        AuthSeq:       authId,
        DiscoveryType: "CoAP",
    }
    bc.OnDeviceOnline(node)
}
```

### 与发现模块集成

当设备被发现时，可以预先添加到 Bus Center（状态为离线）：

```go
func onDeviceDiscovered(device *DiscoveryDevice) {
    bc := bus_center.GetInstance()
    node := &bus_center.NodeInfo{
        NetworkID:     device.DeviceID,
        DeviceID:      device.DeviceID,
        DeviceName:    device.DeviceName,
        Status:        bus_center.StatusOffline,
        DiscoveryType: "CoAP",
    }
    bc.OnDeviceOnline(node)
}
```

## CLI 工具集成

在 CLI 工具中使用 `nodes` 命令查看 Bus Center 中的节点：

```bash
softbus-cli> nodes
NetworkID            DeviceName           Status     ConnectAddr          JoinTime
----------------------------------------------------------------------------------------------------
1234567890abcdef...  HarmonyDevice        在线       192.168.1.100:6666   2025-11-20 09:30:15
```

## 测试

运行单元测试：

```bash
cd pkg/bus_center
go test -v
```

测试覆盖：
- ✅ BusCenter 单例模式
- ✅ 启动/停止功能
- ✅ 节点上线/下线
- ✅ 节点信息查询
- ✅ 事件回调机制
- ✅ NetLedger 增删改查
- ✅ 在线节点过滤

## 未来扩展

- [ ] 节点信息持久化
- [ ] 节点心跳检测
- [ ] 节点自动过期清理
- [ ] 网络拓扑可视化
- [ ] 多网段支持
- [ ] 节点分组管理

## 参考

- HarmonyOS C 代码: `core/bus_center/lnn/`
- 项目状态: [PROJECT_STATUS.md](../../PROJECT_STATUS.md)
