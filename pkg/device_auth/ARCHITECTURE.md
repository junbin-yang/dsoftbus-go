# Device Auth 模块架构

参考 HarmonyOS v4.1.4 C 代码实现

## 模块层次

```
┌─────────────────────────────────────────┐
│  device_auth (顶层API)                   │  ← 对外接口，对应 C 的 device_auth.h
│  - GroupAuthManager (认证管理)            │
│  - DeviceGroupManager (群组管理)          │
├─────────────────────────────────────────┤
│  hichain (协议实现)                       │  ← 会话管理 + PAKE加密
│  - HiChainHandle (会话实例)               │     对应 C 的 deviceauth_standard
│  - 协议消息处理                           │     + pake_v1_protocol
│  - PAKE V1 EC-SPEKE (X25519)            │
│  - 密钥派生 (HKDF)                       │
└─────────────────────────────────────────┘
```

## 职责划分

### 1. device_auth (pkg/device_auth/)
**职责**：对外API，业务逻辑
- `GroupAuthManager` - 认证流程管理
  - `AuthDevice()` - 发起认证
  - `ProcessData()` - 处理认证数据
  - `CancelRequest()` - 取消认证
- `DeviceGroupManager` - 群组和设备管理
  - `CreateGroup()` - 创建群组
  - `AddMemberToGroup()` - 添加设备（**公钥持久化在这里**）
  - `GetTrustedDevices()` - 查询可信设备

**不包含**：
- ❌ 协议细节
- ❌ 加密实现
- ❌ 会话状态

### 2. hichain (pkg/device_auth/hichain/)
**职责**：协议实现（会话管理 + PAKE加密）
- `HiChainHandle` - 单个认证会话
  - 状态管理 (Init → Started → Authenticating → Completed)
  - 消息路由 (PAKE_REQUEST → PAKE_RESPONSE → ...)
  - 回调转换
- 协议消息处理
  - `handleAuthStart()` - 处理 PAKE_REQUEST
  - `handleAuthChallenge()` - 处理 PAKE_CLIENT_CONFIRM
  - `handleAuthConfirm()` - 处理 PAKE_SERVER_CONFIRM
- PAKE V1 EC-SPEKE 加密函数 (pake_v1_ec.go)
  - `computeX25519BasePoint()` - 计算 SPEKE 基点
  - `generateX25519KeyPair()` - 生成临时密钥对
  - `computeX25519SharedSecret()` - 计算共享密钥
  - `deriveSessionKey()` - 派生会话密钥 (HKDF)

**不包含**：
- ❌ 设备持久化（由 device_auth 层负责）
- ❌ 多会话管理（每个 HiChainHandle 是一个会话）

## 数据流

### 认证流程（服务端）

```
1. 上层调用
   device_auth.AuthDevice()

2. 创建会话
   hichain.GetInstance() → HiChainHandle

3. 接收 PAKE_REQUEST
   device_auth.ProcessData()
   → hichain.ReceiveData()
   → hichain.handleAuthStart()

4. 调用加密函数
   pake.ComputeX25519BasePoint(pin, salt)
   pake.GenerateX25519KeyPair()

5. 发送 PAKE_RESPONSE
   hichain.OnTransmit() → device_auth.OnTransmit()

6. 接收 PAKE_CLIENT_CONFIRM
   验证 kcfData
   派生 sessionKey

7. 发送 PAKE_SERVER_CONFIRM
   认证完成

8. 保存公钥
   device_auth.AddMemberToGroup()
```

## 关键设计决策

### 1. 简化架构：两层设计
- ✅ device_auth = 业务逻辑
- ✅ hichain = 协议实现 + 加密函数
- ✅ 移除独立的 pake 包，减少层次

### 2. 会话管理在 hichain 层
- ✅ 每个 `HiChainHandle` 就是一个会话
- ✅ 不需要额外的 `PakeManager`
- ✅ 简化并发控制

### 3. 公钥持久化在 device_auth 层
- ✅ `DeviceGroupManager.AddMemberToGroup()` 负责持久化
- ✅ hichain 包不关心存储
- ✅ 符合单一职责原则

### 4. PIN 码管理
- 按HarmonyOS实现：随机生成6位数字PIN码
- 当前测试实现：固定为888888（trans_auth_manager.go:264）
- 通过回调传递给hichain层

## 与 C 代码对应关系

| Go 模块 | C 代码路径 | 说明 |
|---------|-----------|------|
| device_auth | interfaces/inner_api/device_auth.h | 对外API |
| hichain | services/authenticators/deviceauth_standard<br>+ services/protocol/pake_v1_protocol | 协议实现 + 加密 |

## 下一步工作

1. ✅ 基础架构搭建完成
2. ✅ 简化为两层架构（移除独立 pake 包）
3. ✅ 完善 PAKE V1 客户端确认处理
4. ✅ 实现 PIN 码管理（固定888888用于测试）
5. ⏳ 实现公钥持久化（SQLite 或文件）
6. ⏳ 添加完整的错误处理
7. ⏳ 集成测试

## 使用示例

```go
// 1. 初始化服务
device_auth.InitDeviceAuthService()

// 2. 获取认证管理器
ga, _ := device_auth.GetGaInstance()

// 3. 设置回调
callback := &device_auth.DeviceAuthCallback{
    OnTransmit: func(requestId int64, data []byte) bool {
        // 发送数据到对端
        return true
    },
    OnSessionKeyReturned: func(requestId int64, sessionKey []byte) {
        // 保存会话密钥
    },
    OnFinish: func(requestId int64, operationCode int32, returnData string) {
        // 认证完成
    },
}

// 4. 发起认证
authParams := `{"peerUdid":"xxx","pinCode":"123456"}`
ga.AuthDevice(device_auth.AnyOsAccount, 1001, authParams, callback)

// 5. 处理对端数据
ga.ProcessData(1001, receivedData, callback)
```
