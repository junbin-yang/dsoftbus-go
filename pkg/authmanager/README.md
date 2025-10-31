# AuthManager 模块

这是一个与鸿蒙分布式设备认证兼容的Go语言认证管理器实现。

## 概述

本实现提供设备到设备的认证功能，具有以下特性：

- **协议兼容性**：鸿蒙分布式设备认证兼容的数据包格式
- **加密支持**：使用AES-GCM加密实现安全通信
- **会话管理**：为加密通道提供会话密钥管理
- **设备发现**：设备信息交换和验证
- **连接管理**：基于TCP的连接处理

## 协议规范

### 数据包格式

所有数据包遵循24字节的头部格式：

```
+----------------+----------------+----------------+
| 标识符（4）     | 模块（4）       | 序列号（8）     |
+----------------+----------------+----------------+
| 标志（4）       | 数据长度（4）   | 数据（可变）    |
+----------------+----------------+----------------+
```

- **标识符**：0xBABEFACE（魔数）
- **模块**：模块类型（0-9）
- **序列号**：64位序列号
- **标志**：数据包标志（位0 = 回复标志）
- **数据长度**：数据负载长度
- **数据**：JSON或加密负载

### 模块类型

- `MODULE_NONE` (0)：无模块
- `MODULE_TRUST_ENGINE` (1)：信任引擎消息
- `MODULE_HICHAIN` (2)：HiChain认证
- `MODULE_AUTH_SDK` (3)：认证SDK消息
- `MODULE_HICHAIN_SYNC` (4)：HiChain同步
- `MODULE_CONNECTION` (5)：连接管理
- `MODULE_SESSION` (6)：会话管理
- `MODULE_SMART_COMM` (7)：智能通信
- `MODULE_AUTH_CHANNEL` (8)：认证通道
- `MODULE_AUTH_MSG` (9)：认证消息

### 消息类型

#### GetAuthInfo消息（MODULE_TRUST_ENGINE = 1）
```json
{
  "TECmd": "getAuthInfo" | "retAuthInfo",
  "TEData": "<设备ID>",
  "TEDeviceId": "<认证ID>"
}
```

#### VerifyIP消息（MODULE_CONNECTION = 5）
```json
{
  "CODE": 0,
  "BUS_MAX_VERSION": 2,
  "BUS_MIN_VERSION": 2,
  "AUTH_PORT": 8000,
  "SESSION_PORT": 9000,
  "CONN_CAP": 31,
  "DEVICE_NAME": "设备名称",
  "DEVICE_TYPE": "DEV_L0",
  "DEVICE_ID": "设备ID",
  "VERSION_TYPE": "1.0.0"
}
```

#### VerifyDeviceID消息（MODULE_CONNECTION = 5）
```json
{
  "CODE": 1,
  "DEVICE_ID": "设备ID"
}
```

### 加密

某些模块的消息使用AES-GCM加密：

**加密负载格式：**
```
+----------------+----------------+-----------------------------+
| 索引 (4)       | IV (12)        | 密文 + MAC (可变)           |
+----------------+----------------+-----------------------------+
```

- **索引**：会话密钥索引
- **IV**：12字节随机数（前8字节来自序列号）
- **密文 + MAC**：AES-GCM加密数据及16字节认证标签

**使用加密的模块：**
- MODULE_CONNECTION (5)
- MODULE_SESSION (6)
- MODULE_SMART_COMM (7)

**使用明文的模块：**
- MODULE_TRUST_ENGINE (1)
- MODULE_HICHAIN (2)
- MODULE_AUTH_SDK (3)
- MODULE_HICHAIN_SYNC (4)
- MODULE_AUTH_CHANNEL (8)
- MODULE_AUTH_MSG (9)

## 项目结构

```
authmanager/
├── constants.go          # 协议常量
├── types.go             # 数据结构
├── errors.go            # 错误定义
├── crypto.go            # AES-GCM加密/解密
├── auth_conn.go         # 连接和数据包处理
├── session_key.go       # 会话密钥管理
├── auth_manager.go      # 主认证管理器
├── auth_interface.go    # HiChain集成层
├── tcp_server.go        # TCP服务器实现
├── bus_manager.go       # 总线管理器
├── client.go            # 客户端实现
├── hichain/             # HiChain认证协议
│   ├── types.go         # HiChain数据结构
│   ├── protocol.go      # 认证协议实现
│   ├── hichain.go       # HiChain API
│   └── hichain_test.go  # 单元测试
├── examples/
│   ├── server_demo.go   # 服务器入口点
│   └── client_demo.go   # 客户端入口点
└── README.md            # 本文档
```

## 使用方法

### 运行服务器

```bash
# 使用默认设置启动服务器
./bin/server_demo.exe

# 使用自定义设置启动服务器
./bin/server_demo.exe -id device001 -name "我的设备" -ip 127.0.0.1 -version 1.0.0
```

服务器将输出监听端口：
```
认证端口: 12345
```

### 运行客户端

```bash
# 连接到服务器
./bin/client_demo.exe -remote-ip 127.0.0.1 -remote-port 12345

# 使用自定义客户端设置
./bin/client_demo.exe -id client001 -name "我的客户端" -remote-ip 127.0.0.1 -remote-port 12345
```

### 命令行选项

**服务器：**
- `-id`：设备ID（默认："device001"）
- `-name`：设备名称（默认："GoDevice"）
- `-ip`：本地IP地址（默认："127.0.0.1"）
- `-version`：设备版本（默认："1.0.0"）

**客户端：**
- `-id`：客户端设备ID（默认："client001"）
- `-name`：客户端设备名称（默认："GoClient"）
- `-ip`：客户端IP地址（默认："127.0.0.1"）
- `-version`：客户端版本（默认："1.0.0"）
- `-remote-ip`：远程设备IP（必需）
- `-remote-port`：远程设备认证端口（必需）

## 认证流程

```
客户端                           服务器
  |                                |
  |--- GetAuthInfo --------------> |  模块1
  |<-- RetAuthInfo --------------- |
  |                                |
  |--- VerifyIP -----------------> |  模块5
  |<-- RetVerifyIP --------------  |
  |                                |
  |--- VerifyDeviceID -----------> |  模块5
  |<-- RetVerifyDeviceID --------  |
  |                                |
  |    认证完成                     |
```

## 限制

本实现目前不包括：

1. **完整的HiChain功能**：群组管理、证书验证（仅实现基本认证）
2. **会话层**：完整的会话管理（仅占位符）
3. **发现服务**：设备发现机制
4. **持久化存储**：设备凭证存储
