package authmanager

// 数据包相关常量
const (
	MessageIndexLen           = 4                                                       // 消息索引长度（字节）
	MessageEncryptOverheadLen = MessageIndexLen + MessageGcmNonceLen + MessageGcmMacLen // 加密额外开销（索引+nonce+MAC）

	DefaultBufSize      = 1536                            // 默认缓冲区大小（字节）
	PacketHeadSize      = 24                              // 数据包头部大小（字节）
	PacketDataSize      = DefaultBufSize - PacketHeadSize // 数据包数据部分最大大小（字节）
	PkgHeaderIdentifier = 0xBABEFACE                      // 数据包头部标识符（用于校验数据包合法性）
)

// 在线状态
const (
	OnlineUnknown = 0  // 未知状态
	OnlineYes     = 1  // 在线
	OnlineNo      = -1 // 离线
)

// 模块类型（用于区分不同功能模块）
const (
	ModuleNone        = 0 // 无模块
	ModuleTrustEngine = 1 // 信任引擎模块
	ModuleHiChain     = 2 // HiChain模块
	ModuleAuthSDK     = 3 // 认证SDK模块
	ModuleHiChainSync = 4 // HiChain同步模块
	ModuleConnection  = 5 // 连接管理模块
	ModuleSession     = 6 // 会话管理模块
	ModuleSmartComm   = 7 // 智能通信模块
	ModuleAuthChannel = 8 // 认证通道模块
	ModuleAuthMsg     = 9 // 认证消息模块
)

// 标志位（用于数据包头部）
const (
	FlagReply = 1 // 回复标志（标识该数据包是一个回复）
)

// 认证状态
const (
	AuthUnknown = 0 // 未知状态
	AuthInit    = 1 // 初始化状态
)

// 总线版本
const (
	BusUnknown = 0 // 未知版本
	BusV1      = 1 // 版本1
	BusV2      = 2 // 版本2
)

// 认证相关常量
const (
	AuthDefaultID        = "default" // 默认认证ID
	AuthDefaultIDLen     = 7         // 默认认证ID长度
	AuthSessionKeyLen    = 16        // 会话密钥长度（字节）
	AuthSessionKeyMaxNum = 2         // 最大会话密钥数量
	AuthSessionMaxNum    = 2         // 最大会话数量
	AuthConnMaxNum       = 32        // 最大认证连接数量
)

// 最大长度限制（字符数）
const (
	MaxAuthIDLen  = 64  // 认证ID最大长度
	MaxDevIDLen   = 64  // 设备ID最大长度
	MaxDevIPLen   = 46  // 设备IP地址最大长度（支持IPv6）
	MaxDevNameLen = 128 // 设备名称最大长度
	MaxDevTypeLen = 32  // 设备类型最大长度
	MaxVersionLen = 64  // 版本信息最大长度
)

// 消息状态码
const (
	CodeVerifyIP    = 0 // IP验证相关消息
	CodeVerifyDevID = 1 // 设备ID验证相关消息
)

// 消息标签（用于JSON字段标识）
const (
	CmdTag         = "TECmd"       // 命令标签
	DataTag        = "TEData"      // 数据标签
	DeviceIDTag    = "TEDeviceId"  // 设备ID标签
	CmdGetAuthInfo = "getAuthInfo" // 获取认证信息命令
	CmdRetAuthInfo = "retAuthInfo" // 返回认证信息命令
)

// 设备相关常量
const (
	DeviceConnCapWifi = 0x1f     // WiFi连接能力标志
	DeviceTypeDefault = "DEV_L0" // 默认设备类型
)
