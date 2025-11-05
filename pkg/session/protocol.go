package session

// 会话限制常量（与C版本兼容）
const (
	// 最大会话数量
	MaxSessionNum       = 3  // 每个服务器最多3个会话
	MaxSessionServerNum = 8  // 最多8个会话服务器
	MaxSessionSumNum    = 24 // 总计最多24个会话

	// 名称长度限制
	NameLength   = 64 // 会话名称/模块名称长度
	MaxDevIDLen  = 64 // 设备ID最大长度
	MaxAuthIDLen = 64 // 认证ID最大长度

	// 密钥长度
	SessionKeyLength  = 32 // 会话密钥长度（256位）
	AuthSessionKeyLen = 16 // 认证会话密钥长度（128位）
)

// 数据包相关常量
const (
	// 数据包头部大小
	AuthPacketHeadSize  = 24 // 认证包头部大小（4+8+4+4+4=24）
	TransPacketHeadSize = 16 // 传输包头部大小（4+4+4+4=16）

	// 缓冲区大小
	RecvBuffSize    = 4096 // 接收缓冲区（4KB）
	SendBuffMaxSize = 4048 // 最大发送数据（4096-4-16-28）
)

// 协议魔数
const (
	// 数据包头部标识符
	PkgHeaderIdentifier = 0xBABEFACE // 包头部魔数

	// 数据包类型
	PacketTypeData = 0x01 // 数据包
	PacketTypeAck  = 0x02 // 确认包
)

// 会话状态
const (
	SessionStateUnknown   = 0 // 未知状态
	SessionStateConnected = 1 // 已连接
	SessionStateOpened    = 2 // 已打开
	SessionStateClosed    = 3 // 已关闭
)

// 数据包结构定义

// AuthPacket 认证数据包头部（24字节）
type AuthPacket struct {
	Module   uint32 // 模块ID（4字节）
	Seq      int64  // 序列号（8字节）
	Flags    uint32 // 标志位（4字节）
	DataLen  uint32 // 数据长度（4字节）
	Reserved uint32 // 保留字段（4字节）
}

// TransPacket 传输数据包头部（16字节，与C版本兼容）
type TransPacket struct {
	MagicNum uint32 // 魔数（4字节）= 0xBABEFACE
	SeqNum   int32  // 序列号（4字节）注意：C版本使用int而非long long
	Flags    uint32 // 标志位（4字节）
	DataLen  uint32 // 数据长度（4字节）包含加密开销
}

// FirstPacketData 首次连接数据包（JSON格式）
type FirstPacketData struct {
	BusName       string `json:"BUS_NAME"`        // 会话名称
	SessionKey    string `json:"SESSION_KEY"`     // 会话密钥（Base64编码）
	MySessionName string `json:"MY_SESSION_NAME"` // 本地会话名称
	DeviceID      string `json:"DEVICE_ID"`       // 设备ID
	BusVersion    int    `json:"BUS_VERSION"`     // 总线版本
}

// ResponsePacketData 握手响应数据包（JSON格式）
type ResponsePacketData struct {
	DeviceID      string `json:"DEVICE_ID"`       // 本地设备ID
	SessionName   string `json:"SESSION_NAME"`    // 会话名称
	BusVersion    int    `json:"BUS_VERSION"`     // 总线版本
	MySessionName string `json:"MY_SESSION_NAME"` // 本地会话名称
	Result        int    `json:"RESULT"`          // 结果码（0=成功）
}

// JSON字段名常量
const (
	JSONFieldBusName       = "BUS_NAME"
	JSONFieldSessionKey    = "SESSION_KEY"
	JSONFieldMySessionName = "MY_SESSION_NAME"
	JSONFieldDeviceID      = "DEVICE_ID"
	JSONFieldBusVersion    = "BUS_VERSION"
	JSONFieldResult        = "RESULT"
)

// 其他常量
const (
	// 默认会话名称（未知会话）
	DefaultUnknownSessionName = "softbus_Lite_unknown"

	// 监听队列长度
	ListenBacklog = 4

	// 默认总线版本
	DefaultBusVersion = 1
)
