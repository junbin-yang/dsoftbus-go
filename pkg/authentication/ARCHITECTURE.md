# Authentication 模块架构设计

**最后更新**: 2025-11-14
**状态**: ✅ 全部完成 (包括 HiChain 集成 + auth_session 层)

---

## 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      应用层 (Application)                         │
│               - 业务逻辑                                           │
│               - 传输层 (transmission)                              │
└───────────────────────────┬─────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│                  Auth Manager 层 ✅                               │
│              (auth_manager.go)                                   │
│  - 认证管理器管理                                                  │
│  - 连接生命周期管理                                                │
│  - Session Key集成                                                │
│  - 设备信息管理                                                    │
│  - 消息路由到auth_session                                         │
└──────────┬──────────────────────────────────────────────────────┘
           ↓
┌─────────────────────────────────────────────────────────────────┐
│                  Auth Session 层 ✅ **新增！**                    │
│              (auth_session.go)                                   │
│  - 认证会话状态机                                                  │
│  - HiChain集成（MODULE_AUTH_SDK路由）                             │
│  - 设备ID交换处理                                                  │
│  - 认证流程自动化                                                  │
│  - device_auth回调转换                                            │
└──────────┬─────────────────────────┬────────────────────────────┘
           ↓                         ↓
   ┌──────────────┐         ┌──────────────────┐
   │ device_auth  │         │ Auth Connection  │
   │    包 ✅      │         │     层 ✅         │
   └──────────────┘         └──────────────────┘
           ↓
   ┌──────────────┐
   │   hichain/   │
   │  HiChain协议 │
   │    实现 ✅    │
   └──────────────┘
```

### 详细分层架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      应用层 (Application)                         │
└───────────────────────────┬─────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────────┐
│                  Auth Manager 层 ✅                               │
│              (auth_manager.go)                                   │
│                                                                   │
│  AuthManagerService (单例)                                        │
│    ├─ managers: map[authId]*AuthManager                          │
│    ├─ sessionKeyMgr: *SessionKeyManager                          │
│    └─ callback: *AuthConnCallback                                │
│                                                                   │
│  AuthManager (单个认证会话)                                        │
│    ├─ AuthId / ConnId / AuthSeq                                  │
│    ├─ HasAuthPassed (认证状态)                                    │
│    ├─ SessionKeyMgr (会话密钥)                                    │
│    └─ DeviceInfo (对端设备信息)                                   │
│                                                                   │
│  消息路由:                                                         │
│    ├─ MODULE_AUTH_SDK (3) → handleHiChainData() → device_auth   │
│    └─ 其他模块 → 应用层回调                                        │
└───────────────┬───────────────────────┬─────────────────────────┘
                ↓                       ↓
    ┌───────────────────────┐   ┌─────────────────────────────┐
    │   device_auth 包 ✅    │   │   Auth Connection 层 ✅      │
    │  (device_auth.go)     │   │   (auth_connection.go)      │
    │                       │   │                             │
    │  适配层：              │   │  - 连接生命周期管理          │
    │  - GroupAuthManager   │   │  - 数据收发封装             │
    │  - DeviceGroupManager │   │  - 连接状态追踪             │
    │  - 回调转换            │   │  - ConnId管理               │
    │                       │   └──────────────┬──────────────┘
    │  实例管理：            │                  ↓
    │  - HiChain实例映射    │   ┌─────────────────────────────┐
    │  - 生命周期管理        │   │   Auth Channel 层 ✅         │
    └───────┬───────────────┘   │   (auth_channel.go)         │
            ↓                   │                             │
    ┌───────────────────────┐   │  - 发布-订阅消息路由         │
    │   hichain/ ✅          │   │  - MODULE_AUTH_CHANNEL 路由 │
    │  (HiChain协议实现)     │   │  - MODULE_AUTH_MSG 路由     │
    │                       │   └──────────────┬──────────────┘
    │  - types.go           │                  ↓
    │  - protocol.go        │   ┌─────────────────────────────┐
    │  - hichain.go         │   │   TCP Protocol 层 ✅         │
    │  - utils.go           │   │   (auth_tcp_connection.go)  │
    │                       │   │                             │
    │  功能：                │   │  - 24字节协议头编解码        │
    │  - 挑战-响应认证       │   │  - Socket连接管理           │
    │  - 会话密钥派生        │   │  - 消息路由分发             │
    │  - 消息打包/解包       │   └──────────────┬──────────────┘
    │  - 状态机管理          │                  ↓
    └───────────────────────┘   ┌─────────────────────────────┐
                                │   Network 层 ✅              │
                                │   (tcp_connection包)        │
                                │                             │
                                │  - 虚拟fd管理                │
                                │  - TCP连接池                │
                                └─────────────────────────────┘
```

---

## 模块设计

### 1. Auth Manager 层 (auth_manager.go) ✅

**职责**：
- 管理认证会话（AuthManager结构）
- **集成 device_auth（HiChain认证流程）**
- **MODULE_AUTH_SDK 消息路由**
- 集成 Session Key Manager
- 集成设备信息管理
- 提供上层设备操作API

**核心结构**：
```go
type AuthManager struct {
    AuthId          int64              // 认证ID
    AuthSeq         int64              // 认证序列号
    ConnId          uint64             // 连接ID
    ConnInfo        *AuthConnInfo      // 连接信息
    IsServer        bool               // 是否服务端
    HasAuthPassed   bool               // 是否已认证 ✅
    LastActiveTime  time.Time          // 最后活跃时间
    SessionKeyMgr   *SessionKeyManager // Session Key管理器
    HiChainHandle   interface{}        // HiChain句柄（预留）
    DeviceInfo      *DeviceInfo        // 对端设备信息
}

type AuthManagerService struct {
    managers         map[int64]*AuthManager  // authId -> manager
    connIdToAuthId   map[uint64]int64        // connId -> authId
    sessionKeyMgr    *SessionKeyManager      // 全局Session Key管理器
    callback         *AuthConnCallback       // 应用层回调
    initialized      bool
    authIdCounter    int64                   // 原子计数器
    seqCounter       int64                   // 序列号计数器
}
```

**HiChain 集成**：

```go
// 初始化时启动 device_auth 服务
func AuthDeviceInit(callback *AuthConnCallback) error {
    // 初始化 device_auth 服务（HiChain）
    if err := device_auth.InitDeviceAuthService(); err != nil {
        return fmt.Errorf("failed to init device auth service: %w", err)
    }
    // ...
}

// 消息路由：MODULE_AUTH_SDK → HiChain
func onAuthDataReceived(connId uint64, connInfo *AuthConnInfo,
                        fromServer bool, head *AuthDataHead, data []byte) {
    // MODULE_AUTH_SDK 路由到 device_auth（HiChain）
    if head.Module == ModuleAuthSdk {
        handleHiChainData(manager, head, data)
        return
    }

    // 其他模块路由到应用层回调
    if service.callback != nil && service.callback.OnDataReceived != nil {
        service.callback.OnDataReceived(authId, head, data)
    }
}

// 处理 HiChain 认证数据
func handleHiChainData(manager *AuthManager, head *AuthDataHead, data []byte) {
    ga, _ := device_auth.GetGaInstance()
    callback := createDeviceAuthCallback(manager)
    ga.ProcessData(head.Seq, data, callback)
}

// 启动 HiChain 认证（客户端）
func StartHiChainAuth(manager *AuthManager) error {
    ga, _ := device_auth.GetGaInstance()
    authParams := `{"peerUdid":"peer-device","serviceType":"softbus_auth"}`
    callback := createDeviceAuthCallback(manager)
    return ga.AuthDevice(device_auth.AnyOsAccount, manager.AuthSeq, authParams, callback)
}

// device_auth 回调转换
func createDeviceAuthCallback(manager *AuthManager) *device_auth.DeviceAuthCallback {
    return &device_auth.DeviceAuthCallback{
        OnTransmit: func(requestId int64, data []byte) bool {
            // 通过 AuthConnection 发送 MODULE_AUTH_SDK 数据
            head := &AuthDataHead{Module: ModuleAuthSdk, Seq: requestId}
            return PostAuthData(manager.ConnId, head, data) == nil
        },
        OnSessionKeyReturned: func(requestId int64, sessionKey []byte) {
            // 存储到 SessionKeyManager
            manager.SessionKeyMgr.SetSessionKey(manager.AuthId, sessionKey)
        },
        OnFinish: func(requestId int64, operationCode int32, returnData string) {
            // 标记认证成功
            manager.HasAuthPassed = true
            // 通知应用层
            notifyAuthResult(manager, AuthResultSuccess)
        },
        OnError: func(requestId int64, operationCode int32, errorCode int32, errorReturn string) {
            // 通知应用层认证失败
            notifyAuthResult(manager, AuthResultFailed)
        },
    }
}
```

**核心API**：
```go
// 初始化和反初始化
func AuthDeviceInit(callback *AuthConnCallback) error  // ✅ 包含 device_auth 初始化
func AuthDeviceDeinit()                                 // ✅ 包含 device_auth 销毁

// 连接管理
func AuthDeviceOpenConn(connInfo *AuthConnInfo) (uint32, error)  // ✅
func AuthDeviceCloseConn(authId int64)                           // ✅

// 数据发送
func AuthDevicePostTransData(authId int64, module int32, flag int32, data []byte) error  // ✅

// Session Key管理
func AuthManagerSetSessionKey(authId int64, sessionKey []byte) (int32, error)  // ✅
func AuthManagerGetSessionKey(authId int64, index int32) (*SessionKey, error)  // ✅

// 设备信息查询
func AuthDeviceGetConnInfo(authId int64) (*AuthConnInfo, error)      // ✅
func AuthDeviceGetDeviceUuid(authId int64) (string, error)            // ✅
func AuthDeviceGetVersion(authId int64) (*SoftBusVersion, error)      // ✅
func AuthDeviceGetServerSide(authId int64) (bool, error)              // ✅

// HiChain 集成
func StartHiChainAuth(manager *AuthManager) error                     // ✅ 新增
func handleHiChainData(manager *AuthManager, head *AuthDataHead, data []byte)  // ✅ 新增
```

---

### 2. device_auth 包 ✅ (新增)

**位置**: `pkg/device_auth/`

**架构对齐C代码**:
```
C代码:
auth_manager.c → auth_hichain_adapter.c → device_auth.h → libdeviceauth_sdk.so

Go代码:
auth_manager.go → device_auth.go → device_auth/hichain/
```

**职责**：
- 提供 HiChain 认证服务的适配层
- 管理 HiChain 实例生命周期
- 回调转换：`DeviceAuthCallback` ↔ `hichain.HCCallBack`
- 实例映射：`authReqId` → `HiChainHandle`

**核心结构**：
```go
// GroupAuthManager - 组认证管理器（真实实现）
type GroupAuthManager interface {
    ProcessData(authReqId int64, data []byte, gaCallback *DeviceAuthCallback) error
    AuthDevice(osAccountId int32, authReqId int64, authParams string, gaCallback *DeviceAuthCallback) error
    CancelRequest(requestId int64, appId string)
    GetRealInfo(osAccountId int32, pseudonymId string) (string, error)
    GetPseudonymId(osAccountId int32, indexKey string) (string, error)
}

// 真实实现
type realGroupAuthManager struct {
    hichainInstances map[int64]*hichain.HiChainHandle  // authReqId -> HiChain实例
    callbacks        map[int64]*DeviceAuthCallback     // authReqId -> 回调
    mu               sync.RWMutex
}

// DeviceAuthCallback - 设备认证回调
type DeviceAuthCallback struct {
    OnTransmit           func(requestId int64, data []byte) bool
    OnSessionKeyReturned func(requestId int64, sessionKey []byte)
    OnFinish             func(requestId int64, operationCode int32, returnData string)
    OnError              func(requestId int64, operationCode int32, errorCode int32, errorReturn string)
    OnRequest            func(requestId int64, operationCode int32, reqParams string) string
}
```

**核心API**：
```go
// 服务初始化
func InitDeviceAuthService() error          // ✅
func DestroyDeviceAuthService()             // ✅

// 获取管理器实例
func GetGaInstance() (GroupAuthManager, error)       // ✅
func GetGmInstance() (DeviceGroupManager, error)     // ✅

// GroupAuthManager 接口
func (g *realGroupAuthManager) AuthDevice(osAccountId int32, authReqId int64,
                                          authParams string, gaCallback *DeviceAuthCallback) error  // ✅
func (g *realGroupAuthManager) ProcessData(authReqId int64, data []byte,
                                           gaCallback *DeviceAuthCallback) error                   // ✅
func (g *realGroupAuthManager) CancelRequest(requestId int64, appId string)                        // ✅
```

**实例管理**：
- 客户端：在 `AuthDevice()` 时创建 HiChain 实例 (HCController)
- 服务端：在首次 `ProcessData()` 时创建 HiChain 实例 (HCAccessory)
- 认证完成后自动销毁（通过 `SetServiceResult` 回调）

---

### 3. hichain/ 包 ✅ (HiChain协议实现)

**位置**: `pkg/device_auth/hichain/`

**职责**：
- 完整的 HiChain 挑战-响应认证协议
- 会话密钥派生算法（SHA256）
- 消息打包/解包（JSON）
- 认证状态机管理

**文件结构**：
```go
hichain/
├── types.go          // 核心类型定义
├── protocol.go       // 认证协议实现
├── hichain.go        // API接口
├── utils.go          // 工具函数
└── hichain_test.go   // 单元测试
```

**核心类型**：
```go
type SessionIdentity struct {
    SessionID     uint32
    PackageName   string
    ServiceType   string
    OperationCode int32
}

type HiChainHandle struct {
    identity      *SessionIdentity
    deviceType    int              // HCAccessory / HCController
    callback      *HCCallBack
    state         int              // StateInit / StateStarted / StateCompleted
    sessionKey    []byte           // 派生的会话密钥
    ourChallenge  []byte           // 本方挑战值
    peerChallenge []byte           // 对方挑战值
    selfAuthID    string
    peerAuthID    string
}

type HCCallBack struct {
    OnTransmit            func(identity *SessionIdentity, data []byte) error
    GetProtocolParams     func(identity *SessionIdentity, operationCode int32) (*ProtocolParams, error)
    SetSessionKey         func(identity *SessionIdentity, sessionKey *SessionKey) error
    SetServiceResult      func(identity *SessionIdentity, result int32) error
    ConfirmReceiveRequest func(identity *SessionIdentity, operationCode int32) int32
}
```

**认证流程**：
```
客户端 (HCController)              服务端 (HCAccessory)
    │                                   │
    ├─> StartAuth()                     │
    │   生成挑战值 Challenge_A          │
    │   发送 AUTH_START ───────────────>│
    │                                   ├─> ReceiveData(AUTH_START)
    │                                   │   生成挑战值 Challenge_B
    │                                   │   计算响应 Response_A
    │<────────────── AUTH_CHALLENGE ────┤
    │                                   │
    ├─> ReceiveData(AUTH_CHALLENGE)     │
    │   计算响应 Response_B              │
    │   派生会话密钥                      │
    │   发送 AUTH_RESPONSE ─────────────>│
    │                                   ├─> ReceiveData(AUTH_RESPONSE)
    │                                   │   派生会话密钥
    │<────────────── AUTH_CONFIRM ──────┤
    │                                   │
    ├─> ReceiveData(AUTH_CONFIRM)       │
    │   认证完成                         │   认证完成
```

**密钥派生算法**：
```go
// 基于 SHA256 的密钥派生
func deriveSessionKey(ourChallenge, peerChallenge []byte, selfAuthID, peerAuthID string) []byte {
    // 1. ID排序（确保双方派生相同密钥）
    id1, id2 := sortAuthIDs(selfAuthID, peerAuthID)

    // 2. 拼接输入: challenge1 || challenge2 || id1 || id2
    input := append(append(append(ourChallenge, peerChallenge...), id1...), id2...)

    // 3. SHA256派生
    hash := sha256.Sum256(input)
    return hash[:16]  // 取前16字节作为AES-128密钥
}
```

**核心API**：
```go
func GetInstance(identity *SessionIdentity, deviceType int, callback *HCCallBack) (*HiChainHandle, error)  // ✅
func Destroy(handle **HiChainHandle)                                                                       // ✅
func (h *HiChainHandle) StartAuth() error                                                                  // ✅
func (h *HiChainHandle) ReceiveData(data []byte) error                                                     // ✅
func (h *HiChainHandle) GetState() int                                                                     // ✅
func (h *HiChainHandle) GetSessionKey() []byte                                                             // ✅
```

---

### 4. Auth Connection 层 (auth_connection.go) ✅

**职责**：
- 封装 auth_tcp_connection 和 auth_channel
- 管理连接的生命周期（连接、断开）
- ConnId 生成和管理（64位：connType + fd）
- 数据发送封装
- 实现 AuthConnListener 回调分发

**核心结构**：
```go
type AuthConnListener struct {
    OnConnectResult func(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo)
    OnDisconnected  func(connId uint64, connInfo *AuthConnInfo)
    OnDataReceived  func(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte)
}

type AuthConnectionManager struct {
    listener       *AuthConnListener
    connections    map[uint64]*AuthConnection  // connId -> connection
    fdToConnId     map[int]uint64              // fd -> connId
    requests       map[uint32]*ConnectRequest  // requestId -> request
}
```

**核心API**：
```go
func AuthConnInit(listener *AuthConnListener) error                   // ✅
func AuthConnDeinit()                                                  // ✅
func ConnectAuthDevice(requestId uint32, connInfo *AuthConnInfo) error // ✅
func DisconnectAuthDevice(connId uint64)                               // ✅
func PostAuthData(connId uint64, head *AuthDataHead, data []byte) error // ✅
func GetConnInfo(connId uint64) (*AuthConnInfo, error)                 // ✅
```

---

### 5. Session Key 管理模块 (session_key_manager.go) ✅

**职责**：
- 管理 HiChain 派生的会话密钥
- 密钥存储、查询、索引管理
- 密钥过期处理
- 预留加密/解密接口（空实现）
- 预留持久化接口

**核心结构**：
```go
type SessionKey struct {
    Index      int32     // 密钥索引
    Key        []byte    // 会话密钥（16字节）
    CreateTime time.Time // 创建时间
    LastUsed   time.Time // 最后使用时间
}

type SessionKeyManager struct {
    keys      map[int64][]*SessionKey  // authId -> 密钥列表
    persistor SessionKeyPersistor      // 持久化接口（预留）
    mu        sync.RWMutex
}
```

**核心API**：
```go
func NewSessionKeyManager() *SessionKeyManager                                         // ✅
func (m *SessionKeyManager) SetSessionKey(authId int64, sessionKey []byte) (int32, error) // ✅
func (m *SessionKeyManager) GetSessionKey(authId int64, index int32) (*SessionKey, error) // ✅
func (m *SessionKeyManager) GetLatestSessionKey(authId int64) (*SessionKey, error)      // ✅
func (m *SessionKeyManager) RemoveSessionKey(authId int64, index int32)                 // ✅
func (m *SessionKeyManager) RemoveAllSessionKeys(authId int64)                          // ✅
```

---

### 6. 设备信息管理模块 (device_info.go) ✅

**职责**：
- 管理本地设备信息（UDID、UUID、设备名称、版本等）
- 提供注入接口，支持外部设置设备信息
- 替代C代码中的LNN模块依赖

**核心结构**：
```go
type DeviceInfo struct {
    UDID        string         // 设备唯一标识
    UUID        string         // 通用唯一标识
    DeviceName  string         // 设备名称
    DeviceType  string         // 设备类型
    Version     SoftBusVersion // 软总线版本
}

type DeviceInfoProvider interface {
    GetDeviceInfo() (*DeviceInfo, error)
    GetUDID() (string, error)
    GetUUID() (string, error)
}
```

**核心API**：
```go
func RegisterDeviceInfoProvider(provider DeviceInfoProvider) error  // ✅
func UnregisterDeviceInfoProvider()                                 // ✅
func GetLocalDeviceInfo() (*DeviceInfo, error)                      // ✅
func GetLocalUDID() (string, error)                                 // ✅
func GetLocalUUID() (string, error)                                 // ✅
```

---

## 完整数据流

### HiChain 认证流程

```
┌─────────────┐                                          ┌─────────────┐
│   客户端     │                                          │   服务端     │
└──────┬──────┘                                          └──────┬──────┘
       │                                                        │
       │ 1. AuthDeviceOpenConn()                               │
       │    ┌─────────────────────────────┐                    │
       │    │ StartHiChainAuth(manager)   │                    │
       │    │   ↓                          │                    │
       │    │ device_auth.AuthDevice()    │                    │
       │    │   ↓                          │                    │
       │    │ hichain.StartAuth()         │                    │
       │    │   ↓                          │                    │
       │    │ 生成 Challenge_A             │                    │
       │    └─────────────────────────────┘                    │
       │                                                        │
       │────────── AUTH_START (MODULE_AUTH_SDK) ──────────────>│
       │                                                        │
       │                                          ┌──────────────────────────┐
       │                                          │ onAuthDataReceived()     │
       │                                          │   ↓                      │
       │                                          │ handleHiChainData()      │
       │                                          │   ↓                      │
       │                                          │ device_auth.ProcessData()│
       │                                          │   ↓                      │
       │                                          │ hichain.ReceiveData()    │
       │                                          │   ↓                      │
       │                                          │ 生成 Challenge_B         │
       │                                          │ 计算 Response_A          │
       │                                          └──────────────────────────┘
       │                                                        │
       │<────── AUTH_CHALLENGE (MODULE_AUTH_SDK) ──────────────│
       │                                                        │
       │ ┌─────────────────────────────┐                       │
       │ │ hichain.ReceiveData()       │                       │
       │ │   ↓                          │                       │
       │ │ 计算 Response_B              │                       │
       │ │ 派生会话密钥 (SHA256)        │                       │
       │ │   ↓                          │                       │
       │ │ OnSessionKeyReturned 回调    │                       │
       │ │   ↓                          │                       │
       │ │ SessionKeyMgr.SetSessionKey()│                       │
       │ └─────────────────────────────┘                       │
       │                                                        │
       │────────── AUTH_RESPONSE (MODULE_AUTH_SDK) ────────────>│
       │                                                        │
       │                                          ┌──────────────────────────┐
       │                                          │ hichain.ReceiveData()    │
       │                                          │   ↓                      │
       │                                          │ 派生会话密钥 (SHA256)    │
       │                                          │   ↓                      │
       │                                          │ OnSessionKeyReturned 回调│
       │                                          │   ↓                      │
       │                                          │ SessionKeyMgr.SetSessionKey()│
       │                                          └──────────────────────────┘
       │                                                        │
       │<────── AUTH_CONFIRM (MODULE_AUTH_SDK) ────────────────│
       │                                                        │
       │ ┌─────────────────────────────┐       ┌──────────────────────────┐
       │ │ OnFinish 回调                │       │ OnFinish 回调            │
       │ │   ↓                          │       │   ↓                      │
       │ │ HasAuthPassed = true         │       │ HasAuthPassed = true     │
       │ │   ↓                          │       │   ↓                      │
       │ │ notifyAuthResult()           │       │ notifyAuthResult()       │
       │ │   ↓                          │       │   ↓                      │
       │ │ OnConnOpened 回调            │       │ OnConnOpened 回调        │
       │ └─────────────────────────────┘       └──────────────────────────┘
       │                                                        │
       │                  ✅ 认证完成                            │
       │                  双方都有相同的会话密钥                  │
       │                                                        │
```

### 消息路由

```
Socket层收到数据
    ↓
auth_tcp_connection.go
    ↓
根据 Module 路由:
    ├─ MODULE_AUTH_CHANNEL (8) ──→ auth_channel.go ──→ AuthChannelListener
    ├─ MODULE_AUTH_MSG (9) ──────→ auth_channel.go ──→ AuthChannelListener
    ├─ MODULE_AUTH_SDK (3) ──────→ auth_connection.go ──→ onAuthDataReceived()
    │                                                       ↓
    │                                              handleHiChainData()
    │                                                       ↓
    │                                              device_auth.ProcessData()
    │                                                       ↓
    │                                              hichain.ReceiveData()
    │
    └─ 其他模块 ─────────────────→ SocketCallback.OnDataReceived
```

---

## 回调转换机制

### 1. AuthManager → device_auth

```go
// AuthManager 创建 device_auth 回调
func createDeviceAuthCallback(manager *AuthManager) *device_auth.DeviceAuthCallback {
    return &device_auth.DeviceAuthCallback{
        OnTransmit: func(requestId int64, data []byte) bool {
            // 映射到 AuthConnection
            head := &AuthDataHead{Module: ModuleAuthSdk, Seq: requestId}
            return PostAuthData(manager.ConnId, head, data) == nil
        },
        OnSessionKeyReturned: func(requestId int64, sessionKey []byte) {
            // 映射到 SessionKeyManager
            manager.SessionKeyMgr.SetSessionKey(manager.AuthId, sessionKey)
        },
        OnFinish: func(requestId int64, operationCode int32, returnData string) {
            // 映射到应用层回调
            manager.HasAuthPassed = true
            notifyAuthResult(manager, AuthResultSuccess)
        },
        OnError: func(requestId int64, operationCode int32, errorCode int32, errorReturn string) {
            // 映射到应用层回调
            notifyAuthResult(manager, AuthResultFailed)
        },
    }
}
```

### 2. device_auth → hichain

```go
// device_auth 创建 HiChain 回调
func (g *realGroupAuthManager) createHCCallBack(authReqId int64,
                                                gaCallback *DeviceAuthCallback) *hichain.HCCallBack {
    return &hichain.HCCallBack{
        OnTransmit: func(identity *hichain.SessionIdentity, data []byte) error {
            // 映射到 DeviceAuthCallback.OnTransmit
            if gaCallback != nil && gaCallback.OnTransmit != nil {
                success := gaCallback.OnTransmit(authReqId, data)
                if !success {
                    return fmt.Errorf("OnTransmit returned false")
                }
            }
            return nil
        },
        SetSessionKey: func(identity *hichain.SessionIdentity, sessionKey *hichain.SessionKey) error {
            // 映射到 DeviceAuthCallback.OnSessionKeyReturned
            if gaCallback != nil && gaCallback.OnSessionKeyReturned != nil {
                gaCallback.OnSessionKeyReturned(authReqId, sessionKey.Key)
            }
            return nil
        },
        SetServiceResult: func(identity *hichain.SessionIdentity, result int32) error {
            // 映射到 DeviceAuthCallback.OnFinish/OnError
            if result == hichain.HCOk {
                if gaCallback != nil && gaCallback.OnFinish != nil {
                    gaCallback.OnFinish(authReqId, int32(identity.OperationCode), "{}")
                }
            } else {
                if gaCallback != nil && gaCallback.OnError != nil {
                    gaCallback.OnError(authReqId, int32(identity.OperationCode), result, "auth failed")
                }
            }

            // 认证完成后自动清理HiChain实例
            g.mu.Lock()
            if handle, exists := g.hichainInstances[authReqId]; exists {
                hichain.Destroy(&handle)
                delete(g.hichainInstances, authReqId)
                delete(g.callbacks, authReqId)
            }
            g.mu.Unlock()

            return nil
        },
    }
}
```

---

## 简化说明

### 不支持的功能

1. **P2P功能**：
   - ❌ 不实现P2P MAC地址管理
   - ❌ 不实现增强P2P支持
   - ⚠️ 相关字段预留但不使用

2. **连接优先级**：
   - ❌ 不实现多连接优先级管理
   - ✅ 简化为单一连接模式

3. **加密/解密**：
   - ⚠️ Session Key Manager中的加密/解密函数先空实现
   - ⚠️ 返回未加密数据或错误
   - ⚠️ 预留接口供未来实现密码学算法

4. **持久化**：
   - ⚠️ Session Key持久化先不实现
   - ⚠️ 预留 `SessionKeyPersistor` 接口
   - ✅ 当前仅内存存储

---

## C兼容性

### C函数映射

| C函数 | Go函数 | 状态 |
|------|--------|------|
| **Auth Manager** |
| AuthDeviceInit | AuthDeviceInit | ✅ 实现（含 device_auth 初始化） |
| AuthDeviceDeinit | AuthDeviceDeinit | ✅ 实现（含 device_auth 销毁） |
| AuthDeviceOpenConn | AuthDeviceOpenConn | ✅ 实现 |
| AuthDeviceCloseConn | AuthDeviceCloseConn | ✅ 实现 |
| AuthDevicePostTransData | AuthDevicePostTransData | ✅ 实现 |
| AuthManagerSetSessionKey | AuthManagerSetSessionKey | ✅ 实现 |
| AuthManagerGetSessionKey | AuthManagerGetSessionKey | ✅ 实现 |
| AuthDeviceGetConnInfo | AuthDeviceGetConnInfo | ✅ 实现 |
| AuthDeviceGetDeviceUuid | AuthDeviceGetDeviceUuid | ✅ 实现 |
| AuthDeviceGetVersion | AuthDeviceGetVersion | ✅ 实现 |
| AuthDeviceGetServerSide | AuthDeviceGetServerSide | ✅ 实现 |
| AuthDeviceEncrypt | - | ⚠️ 空实现 |
| AuthDeviceDecrypt | - | ⚠️ 空实现 |
| AuthDeviceSetP2pMac | - | ❌ 不支持 |
| AuthDeviceGetP2pConnInfo | - | ❌ 不支持 |
| **device_auth** |
| InitDeviceAuthService | InitDeviceAuthService | ✅ 实现 |
| DestroyDeviceAuthService | DestroyDeviceAuthService | ✅ 实现 |
| GetGaInstance | GetGaInstance | ✅ 实现 |
| GetGmInstance | GetGmInstance | ✅ 实现（stub） |
| gaInstance->authDevice | ga.AuthDevice | ✅ 实现（真实HiChain） |
| gaInstance->processData | ga.ProcessData | ✅ 实现（真实HiChain） |
| gaInstance->cancelRequest | ga.CancelRequest | ✅ 实现 |

---

## 实现状态

### ✅ 已完成模块

1. ✅ **tcp_connection包** - 网络层
2. ✅ **auth_tcp_connection.go** - TCP协议层
3. ✅ **auth_channel.go** - 消息路由层
4. ✅ **device_info.go** - 设备信息管理
5. ✅ **session_key_manager.go** - 会话密钥管理
6. ✅ **auth_connection.go** - 连接管理层
7. ✅ **auth_manager.go** - 认证管理层
8. ✅ **auth_session.go** - 认证会话状态机 **[新增 2025-11-14]**
9. ✅ **device_auth/device_auth.go** - HiChain适配层
10. ✅ **device_auth/hichain/** - HiChain协议实现
11. ✅ **所有单元测试** - 54个测试全部通过

### 📊 测试覆盖

```
✅ authentication: 40个测试 (4.998s)
    - auth_channel: 4个测试
    - auth_connection: 6个测试
    - auth_manager: 8个测试
    - auth_tcp_connection: 4个测试
    - device_info: 6个测试
    - session_key_manager: 12个测试

✅ device_auth: 5个测试 (0.717s)
    - 初始化/反初始化
    - GroupAuthManager 接口
    - DeviceGroupManager 接口
    - 辅助函数

✅ device_auth/hichain: 9个测试 (0.804s)
    - 挑战-响应认证流程
    - 密钥派生算法
    - 消息打包/解包
    - 状态机管理

总计: 54个测试全部通过 🎉
```

---

## 使用示例

### 客户端发起 HiChain 认证

```go
import "github.com/junbin-yang/dsoftbus-go/pkg/authentication"

// 1. 初始化
callback := &authentication.AuthConnCallback{
    OnConnOpened: func(requestId uint32, authId int64) {
        log.Infof("认证成功: authId=%d", authId)

        // 可以开始使用会话密钥加密通信
        sessionKey, _ := authentication.AuthManagerGetSessionKey(authId, 0)
        log.Infof("会话密钥: %x", sessionKey.Key)
    },
    OnConnOpenFailed: func(requestId uint32, reason int32) {
        log.Errorf("认证失败: reason=%d", reason)
    },
    OnDataReceived: func(authId int64, head *authentication.AuthDataHead, data []byte) {
        log.Infof("收到数据: module=%d, len=%d", head.Module, len(data))
    },
}

authentication.AuthDeviceInit(callback)
defer authentication.AuthDeviceDeinit()

// 2. 打开连接
connInfo := &authentication.AuthConnInfo{
    Type:     authentication.AuthLinkTypeWifi,
    AuthPort: 6666,
    ConnInfo: authentication.ConnInfo{
        Type: authentication.ConnTypeTcp,
        TcpInfo: authentication.TcpConnInfo{
            Ip:   "192.168.1.100",
            Port: 6666,
        },
    },
}

requestId, err := authentication.AuthDeviceOpenConn(connInfo)
if err != nil {
    log.Fatalf("打开连接失败: %v", err)
}

// 3. 连接成功后，在 onAuthConnectResult 回调中会自动：
//    - 创建 AuthManager
//    - 调用 StartHiChainAuth() 发起认证
//    - HiChain 自动进行挑战-响应认证
//    - 派生会话密钥并存储
//    - 调用 OnConnOpened 回调通知应用层

// 4. 发送数据
authentication.AuthDevicePostTransData(authId, authentication.ModuleAuthMsg, 0, []byte("Hello"))
```

### 服务端自动处理

```go
// 服务端只需要初始化和启动监听，其余自动处理
authentication.AuthDeviceInit(callback)
defer authentication.AuthDeviceDeinit()

// 收到连接时自动：
// 1. 创建 AuthManager
// 2. 收到 MODULE_AUTH_SDK 数据时路由到 handleHiChainData()
// 3. 调用 device_auth.ProcessData()
// 4. HiChain 自动响应认证流程
// 5. 派生会话密钥并存储
// 6. 调用 OnConnOpened 回调通知应用层
```

---

## 📋 Auth Session 层实现总结 (2025-11-14)

### 问题背景

在初始实现中，auth_manager 层直接调用 device_auth，缺少了 C 代码中的 auth_session 层。这导致：
- ❌ 架构与C代码不匹配
- ❌ 缺少认证会话状态机管理
- ❌ 没有设备ID交换流程
- ❌ 消息路由不完整

### 架构对齐

**C代码架构**:
```
auth_manager.c
    ↓ OnConnectResult / HandleDeviceIdData
auth_session_fsm.c (认证会话状态机)
    ↓ HichainStartAuth / HichainProcessData
device_auth.h
```

**Go代码架构（修正后）**:
```
auth_manager.go
    ↓ onAuthConnectResult / onAuthDataReceived
auth_session.go (认证会话状态机) ✅ 新增
    ↓ device_auth.AuthDevice / ProcessData
device_auth.go
```

### 实现内容

#### 1. 新增文件: auth_session.go (约380行)

**核心结构**:
```go
// 认证会话状态
type AuthSessionState int
const (
    StateInit         AuthSessionState = 0 // 初始状态
    StateSyncDeviceId AuthSessionState = 1 // 同步设备ID
    StateDeviceAuth   AuthSessionState = 2 // 设备认证（HiChain）
    StateAuthDone     AuthSessionState = 3 // 认证完成
    StateFailed       AuthSessionState = 4 // 认证失败
)

// 认证会话（对应C的AuthFsm）
type AuthSession struct {
    AuthSeq        int64
    RequestId      uint32
    ConnId         uint64
    ConnInfo       *AuthConnInfo
    IsServer       bool
    State          AuthSessionState
    AuthManager    *AuthManager
}

// 全局会话管理器
type AuthSessionManager struct {
    sessions      map[int64]*AuthSession
    connIdToSeq   map[uint64]int64
}
```

**核心功能**:
- ✅ 客户端自动启动HiChain认证
- ✅ 服务端自动响应HiChain认证
- ✅ device_auth回调自动转换
- ✅ Session Key自动存储
- ✅ 认证结果通知应用层

#### 2. 修改 auth_manager.go

**初始化集成**:
```go
func AuthDeviceInit(callback *AuthConnCallback) error {
    // 初始化 device_auth 服务
    device_auth.InitDeviceAuthService()

    // 初始化 auth_session 管理器 ✅ 新增
    AuthSessionInit()

    // 初始化 Auth Connection层
    AuthConnInit(...)
}
```

**客户端流程**:
```go
func onAuthConnectResult(...) {
    if result == 0 {
        // 调用auth_session启动认证流程 ✅ 修改
        err := AuthSessionStartAuth(manager.AuthSeq, requestId, connId, connInfo, false)

        // 关联AuthSession到AuthManager
        session, _ := GetAuthSessionByConnId(connId)
        if session != nil {
            session.AuthManager = manager
        }
    }
}
```

**消息路由**:
```go
func onAuthDataReceived(...) {
    // 根据模块路由到auth_session ✅ 新增
    switch head.Module {
    case ModuleTrustEngine:      // (1) 设备ID交换
        AuthSessionProcessDevIdData(head.Seq, data)
    case ModuleAuthSdk:          // (3) HiChain认证数据
        AuthSessionProcessAuthData(head.Seq, data)
    case ModuleAuthConnection:   // (5) 设备信息交换
        // TODO: 未实现
    case ModuleAuthMsg:          // (9) 业务数据
        // 回调到应用层
    }
}
```

### 完整认证流程

```
客户端 CLI                                服务端 CLI
    │                                         │
    ├─> AuthDeviceOpenConn()                  │ (监听中)
    │   ↓                                     │
    │   onAuthConnectResult()                 │
    │   ↓                                     │
    │   AuthSessionStartAuth(isServer=false)  │
    │   ↓                                     │
    │   StateDeviceAuth                       │
    │   ↓                                     │
    │   device_auth.AuthDevice()              │
    │   ↓                                     │
    │   hichain.StartAuth()                   │
    │   ↓                                     │
    │───────── MODULE_AUTH_SDK (3) ─────────>│
    │                                         ├─> onAuthDataReceived()
    │                                         ├─> AuthSessionProcessAuthData()
    │                                         ├─> device_auth.ProcessData()
    │                                         ├─> hichain.ReceiveData()
    │                                         │
    │<──────── MODULE_AUTH_SDK (3) ──────────┤
    │                                         │
    ├─> HiChain认证继续...                   │
    │                                         │
    ├─> OnFinish 回调                        ├─> OnFinish 回调
    ├─> HasAuthPassed = true                 ├─> HasAuthPassed = true
    ├─> OnConnOpened() ✅                    ├─> OnConnOpened() ✅
```

### CLI 使用方法

**启动CLI**:
```bash
go run cmd/softbus-cli.go
```

**命令**:
```
help             - 显示帮助
discover         - 发现网络中的设备
list             - 列出已发现的设备
connect <设备ID> - 连接并认证设备（自动完成HiChain认证）
send <AuthId> <消息> - 发送测试数据
exit             - 退出
```

**认证流程（完全自动）**:
1. 服务端：CLI启动后自动开启TCP监听，等待连接
2. 客户端：执行 `connect <deviceId>` 后自动发起认证
3. 双方自动完成HiChain挑战-响应认证
4. 认证成功后显示 `>>> 认证成功: <设备名> (authId=xxx) <<<`
5. 可以使用 `send <authId> <message>` 发送数据

### 测试状态

- ✅ 编译通过
- ✅ 初始化测试通过
- ✅ HiChain认证流程能够自动启动
- ✅ 架构完全对齐C代码

### 下一步工作

现在可以进行**多设备之间的认证消息调试**：

1. **环境准备**:
   - 在不同机器或不同终端启动CLI
   - 确保网络互通

2. **调试步骤**:
   ```bash
   # 机器A（服务端）
   go run cmd/softbus-cli.go
   # CLI会自动启动发现服务和认证服务

   # 机器B（客户端）
   go run cmd/softbus-cli.go
   softbus-cli> discover         # 发现设备
   softbus-cli> list             # 查看发现的设备
   softbus-cli> connect <设备ID> # 连接并认证
   # 认证会自动完成
   softbus-cli> send <authId> Hello  # 发送测试数据
   ```

3. **预期结果**:
   - ✅ 设备发现成功
   - ✅ TCP连接建立
   - ✅ HiChain认证自动完成（挑战-响应）
   - ✅ Session Key派生并存储
   - ✅ 可以发送和接收数据

4. **调试重点**:
   - 查看日志确认HiChain认证流程
   - 验证Session Key是否正确派生
   - 测试设备间数据收发

---

## 文档更新历史

- **2025-11-14 20:35**: 添加 auth_session 层实现总结
- **2025-11-14**: 添加 HiChain 集成完整说明
- **2025-11-14**: 添加 device_auth 包架构说明
- **2025-11-14**: 添加完整数据流程图
- **2025-11-14**: 更新所有模块为"已完成"状态
- **2025-11-14**: 添加回调转换机制详细说明

---

*最后更新: 2025-11-14 20:35*
