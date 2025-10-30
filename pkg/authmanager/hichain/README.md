# HiChain 模块

HiChain 模块是一个用于设备间认证与会话管理的协议实现，提供了完整的认证流程处理、会话密钥派生及消息交互能力，支持配件设备（HCAccessory）与控制器设备（HCController）之间的安全认证。

## 功能说明

该模块主要实现以下功能:

- 管理 HiChain 认证会话的生命周期（创建、销毁、状态跟踪）

- 处理完整的认证流程（包括认证启动、挑战、响应、确认等阶段）

- 生成随机挑战值、计算认证响应及派生会话密钥

- 消息的打包（JSON 序列化）与解包（JSON 反序列化）

- 通过回调函数与上层模块交互（数据传输、参数获取、结果通知等）

#### 文件结构

```
hichain/
├── types.go          - 数据结构定义
├── protocol.go       - 认证协议实现
├── hichain.go        - API接口
└── hichain_test.go   - 单元测试
```

#### 核心功能

**types.go** - 定义了：
- `SessionIdentity` - 会话标识
- `ProtocolParams` - 协议参数
- `SessionKey` - 会话密钥
- `HCCallBack` - 回调函数接口
- `HiChainHandle` - HiChain实例句柄

**protocol.go** - 实现了：
- 挑战-响应认证机制
- 会话密钥派生算法
- 消息打包/解包
- 认证流程处理

**hichain.go** - 提供了：
- `GetInstance()` - 创建HiChain实例
- `Destroy()` - 销毁实例
- `ReceiveData()` - 处理接收数据
- `StartAuth()` - 启动认证
- `GetState()` - 获取状态
- `GetSessionKey()` - 获取会话密钥

## 使用方法

1、 创建会话实例

通过 GetInstance 创建认证会话实例，需指定会话标识、设备类型及回调函数：

```go
// 构建会话标识
identity := &SessionIdentity{
    SessionID:   1001,
    PackageName: "com.example.service",
    ServiceType: "test-service",
}

// 实现回调函数
callback := &HCCallBack{
    OnTransmit: func(identity *SessionIdentity, data []byte) error {
        // 实现数据发送逻辑
        return nil
    },
    GetProtocolParams: func(identity *SessionIdentity, opCode int32) (*ProtocolParams, error) {
        // 返回协议参数（自身/对端认证ID等）
        return &ProtocolParams{
            KeyLength:  SessionKeyLength,
            SelfAuthID: "device-self-123",
            PeerAuthID: "device-peer-456",
        }, nil
    },
    // 实现其他回调...
}

// 创建实例（设备类型为配件）
handle, err := GetInstance(identity, HCAccessory, callback)
if err != nil {
    // 处理错误
}
```

2、启动认证流程

通过 StartAuth 启动认证：

```go
err := handle.StartAuth()
if err != nil {
    // 处理启动失败（如状态非法）
}
```

3、处理接收数据

通过 ReceiveData 处理收到的认证消息：

```go
// 收到对端发送的认证数据
receivedData := []byte(...) 
err := handle.ReceiveData(receivedData)
if err != nil {
    // 处理消息解析或处理错误
}
```

4、销毁会话实例

认证完成或需要终止时，通过 Destroy 销毁实例：

```go
Destroy(&handle) // 销毁后 handle 会被置空
```

## 认证协议

### 消息类型

1. **AUTH_START (Type 1)** - 发起认证
   - 包含：挑战值、设备ID
   
2. **AUTH_CHALLENGE (Type 2)** - 挑战响应
   - 包含：新挑战、对方挑战的响应、设备ID
   
3. **AUTH_RESPONSE (Type 3)** - 响应确认
   - 包含：对挑战的响应
   
4. **AUTH_CONFIRM (Type 4)** - 认证确认
   - 包含：认证结果

### 认证流程

```
设备A (发起方)                   设备B (响应方)
    │                               │
    │───────── AUTH_START ─────────►│
    │     Challenge_A, AuthID_A     │
    │                               │
    │◄─────── AUTH_CHALLENGE ───────│
    │    Challenge_B, Response_A    │
    │    AuthID_B                   │
    │                               │
    │──────── AUTH_RESPONSE ───────►│
    │    Response_B                 │
    │                               │
    │    [派生会话密钥]              │    [派生会话密钥]
    │                               │
    │◄──────── AUTH_CONFIRM ────────│
    │    Result: OK                 │
    │                               │
    │    [认证完成]                  │
```

1、启动认证（发起方）：

- 调用 StartAuth 生成挑战值，发送 MsgTypeAuthStart 消息

- 状态从 StateInit 转为 StateStarted

2、处理认证启动（接收方）：

- 收到 MsgTypeAuthStart 后，生成自身挑战值并计算对发起方挑战的响应

- 发送 MsgTypeAuthChallenge 消息，状态转为 StateAuthenticating

3、处理挑战（发起方）：

- 收到 MsgTypeAuthChallenge 后，计算对接收方挑战的响应

- 派生会话密钥，发送 MsgTypeAuthResponse 消息

- 发送 MsgTypeAuthConfirm 确认，状态转为 StateCompleted

4、处理响应（接收方）：

- 收到 MsgTypeAuthResponse 后，派生会话密钥

- 状态转为 StateCompleted，通知上层认证结果

5、确认结果：

- 双方通过 SetServiceResult 回调通知上层认证结果（成功 / 失败）