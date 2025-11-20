package authentication

import (
	"net"
	"time"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// 数据包协议常量
	MagicNumber           uint32 = 0xBABEFACE        // 魔数标识
	AuthPktHeadLen        int    = 24                // 数据包头部长度（字节）
	AuthSocketMaxDataLen  int    = 64 * 1024         // 最大数据长度 64KB
	AuthKeepAliveInterval int    = 10 * 60           // KeepAlive间隔 10分钟（秒）
	AuthInvalidFd         int    = -1                // 无效的文件描述符
	InvalidChannelId      int    = -1                // 无效的通道ID
)

const (
	// 连接类型常量
	AuthLinkTypeWifi        int32 = 1 // WiFi连接
	AuthLinkTypeBr          int32 = 2 // BR连接（暂不支持）
	AuthLinkTypeBle         int32 = 3 // BLE连接（暂不支持）
	AuthLinkTypeP2p         int32 = 4 // P2P连接
	AuthLinkTypeEnhancedP2p int32 = 5 // 增强P2P连接
)

const (
	// 数据类型常量（对应C的AuthDataType枚举）
	DataTypeAuth           uint32 = 0xFFFF0001 // 设备认证数据
	DataTypeDeviceInfo     uint32 = 0xFFFF0002 // 设备信息同步
	DataTypeDeviceId       uint32 = 0xFFFF0003 // 设备ID同步
	DataTypeConnection     uint32 = 0xFFFF0004 // 连接数据
	DataTypeCloseAck       uint32 = 0xFFFF0005 // 关闭确认
	DataTypeMetaNegotiation uint32 = 0xFFFF0006 // Meta协商
)

const (
	// 模块ID常量
	ModuleTrustEngine   int32 = 1  // 信任引擎
	ModuleAuthSdk       int32 = 3  // 认证SDK
	ModuleAuthConnection int32 = 5 // 连接管理
	ModuleAuthChannel   int32 = 8  // 认证通道
	ModuleAuthMsg       int32 = 9  // 认证消息
	ModuleMetaAuth      int32 = 21 // Meta认证
)

const (
	// ConnId相关常量
	Int32BitNum     = 32
	MaskUint64H32   = 0xFFFFFFFF00000000 // 高32位掩码
	MaskUint64L32   = 0x00000000FFFFFFFF // 低32位掩码
)

// ============================================================================
// 监听器模块类型（对应C的ListenerModule）
// ============================================================================

type ListenerModule int32

const (
	Auth ListenerModule = iota
	AuthP2p
	AuthEnhancedP2pStart
	AuthEnhancedP2pEnd = AuthEnhancedP2pStart + 100
)

// ============================================================================
// 数据包结构（对应C的SocketPktHead）
// ============================================================================

// SocketPktHead Socket数据包头部
type SocketPktHead struct {
	Magic  uint32 // 魔数 0xBABEFACE
	Module int32  // 模块ID
	Seq    int64  // 序列号
	Flag   int32  // 标志位
	Len    uint32 // 数据长度
}

// AuthDataHead 认证数据包头部（对应C的AuthDataHead）
type AuthDataHead struct {
	DataType uint32 // 数据类型
	Module   int32  // 模块ID
	Seq      int64  // 序列号
	Flag     int32  // 标志位
	Len      uint32 // 数据长度
}

// ============================================================================
// 连接信息结构
// ============================================================================

// SocketOption Socket选项
type SocketOption struct {
	Addr string // IP地址
	Port int    // 端口号
}

// AuthConnInfo 认证连接信息（对应C的AuthConnInfo）
type AuthConnInfo struct {
	Type     int32  // 连接类型（AuthLinkType*）
	Ip       string // IP地址
	Port     int    // 端口
	Udid     string // 设备唯一标识
	PeerUid  string // 对端UID
	// 扩展字段（暂不使用）
	// AuthId   int64  // P2P认证连接ID
	// ModuleId int32  // 增强P2P模块ID
}

// LocalListenerInfo 本地监听器信息
type LocalListenerInfo struct {
	Type       int32  // 连接类型
	SocketInfo SocketOption // Socket信息
}

// ConnectOption 连接选项（扩展tcp_connection.ConnectOption）
type ConnectOption struct {
	LocalSocket  *SocketOption // 本地地址端口
	RemoteSocket *SocketOption // 远程地址端口
	NetConn      *net.Conn     // 网络连接
}

// ============================================================================
// 回调接口定义
// ============================================================================

// SocketCallback Socket层回调（对应C的SocketCallback）
type SocketCallback struct {
	OnConnected    func(module ListenerModule, fd int, isClient bool)
	OnDisconnected func(fd int)
	OnDataReceived func(module ListenerModule, fd int, head *AuthDataHead, data []byte)
}

// AuthChannelData Auth通道数据（对应C的AuthChannelData）
type AuthChannelData struct {
	Module int32  // 模块ID
	Seq    int64  // 序列号
	Flag   int32  // 标志位
	Len    uint32 // 数据长度
	Data   []byte // 数据内容
}

// AuthChannelListener Auth通道监听器（对应C的AuthChannelListener）
type AuthChannelListener struct {
	OnDataReceived func(channelId int, data *AuthChannelData)
	OnDisconnected func(channelId int)
}

// AuthConnListener 认证连接监听器（对应C的AuthConnListener）
type AuthConnListener struct {
	OnConnectResult func(requestId uint32, connId uint64, result int32, connInfo *AuthConnInfo)
	OnDisconnected  func(connId uint64, connInfo *AuthConnInfo)
	OnDataReceived  func(connId uint64, connInfo *AuthConnInfo, fromServer bool, head *AuthDataHead, data []byte)
}

// ============================================================================
// 连接请求结构
// ============================================================================

// ConnRequest 连接请求
type ConnRequest struct {
	RequestId  uint32        // 请求ID
	Fd         int           // 文件描述符
	ConnInfo   *AuthConnInfo // 连接信息
	RetryTimes uint32        // 重试次数
}

// ClientOption 客户端连接选项
type ClientOption struct {
	RemoteIP        string        // 远程IP
	RemotePort      int           // 远程端口
	Timeout         time.Duration // 连接超时
	KeepAlive       bool          // 是否启用KeepAlive
	KeepAlivePeriod time.Duration // KeepAlive周期
}

// ============================================================================
// 软总线版本信息
// ============================================================================

// SoftBusVersion 软总线版本（对应C的SoftBusVersion）
type SoftBusVersion struct {
	Major uint16 // 主版本号
	Minor uint16 // 次版本号
}

// ============================================================================
// 工具函数
// ============================================================================

// GenConnId 生成64位连接ID（高32位=连接类型，低32位=fd）
func GenConnId(connType int32, fd int32) uint64 {
	highPart := (uint64(connType) << Int32BitNum) & MaskUint64H32
	lowPart := uint64(fd) & MaskUint64L32
	return highPart | lowPart
}

// GetConnType 从ConnId提取连接类型（高32位）
func GetConnType(connId uint64) int32 {
	return int32((connId >> Int32BitNum) & MaskUint64L32)
}

// GetConnId 从ConnId提取连接ID（低32位）
func GetConnId(connId uint64) uint32 {
	return uint32(connId & MaskUint64L32)
}

// GetFd 从ConnId提取文件描述符（低32位）
func GetFd(connId uint64) int32 {
	return int32(connId & MaskUint64L32)
}

// GetConnTypeStr 将连接类型转换为字符串
func GetConnTypeStr(connId uint64) string {
	connType := GetConnType(connId)
	switch connType {
	case AuthLinkTypeWifi:
		return "wifi/eth"
	case AuthLinkTypeBr:
		return "br"
	case AuthLinkTypeBle:
		return "ble"
	case AuthLinkTypeP2p:
		return "p2p"
	case AuthLinkTypeEnhancedP2p:
		return "enhanced_p2p"
	default:
		return "unknown"
	}
}

// ModuleToDataType 将模块ID转换为数据类型
func ModuleToDataType(module int32) uint32 {
	switch module {
	case ModuleTrustEngine:
		return DataTypeDeviceId
	case ModuleAuthSdk:
		return DataTypeAuth
	case ModuleAuthConnection:
		return DataTypeDeviceInfo
	default:
		return DataTypeConnection
	}
}
