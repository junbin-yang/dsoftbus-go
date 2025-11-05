package authmanager

import (
	"sync"
)

// DeviceInfo 表示本地设备信息
type DeviceInfo struct {
	DeviceID   string // 设备唯一标识符
	DeviceName string // 设备名称
	DeviceIP   string // 设备IP地址
	DevicePort int    // 设备端口号
	Version    string // 设备版本信息
}

// DataBuffer 表示连接的接收缓冲区
type DataBuffer struct {
	Buf  []byte // 缓冲区数据
	Size int    // 缓冲区总大小
	Used int    // 已使用的缓冲区大小
}

// AuthConn 表示一个认证连接
type AuthConn struct {
	Fd          int        // 文件描述符（用于标识连接）
	AuthID      string     // 认证ID
	DeviceID    string     // 设备ID
	DeviceIP    string     // 设备IP地址
	BusVersion  int        // 总线版本
	AuthPort    int        // 认证端口
	SessionPort int        // 会话端口
	AuthState   int        // 认证状态
	OnlineState int        // 在线状态
	DB          DataBuffer // 数据缓冲区
	mu          sync.Mutex // 互斥锁
}

// ConnInfo 表示连接信息
type ConnInfo struct {
	MaxVersion int    // 支持的最大版本
	MinVersion int    // 支持的最小版本
	DeviceName string // 设备名称
	DeviceType string // 设备类型
}

// SessionKey 表示认证会话密钥
type SessionKey struct {
	Key      [AuthSessionKeyLen]byte // 密钥内容（固定长度数组）
	Index    int                     // 密钥索引
	DeviceID string                  // 关联的设备ID（全局唯一标识）
}

// AuthSession 表示一个认证会话
type AuthSession struct {
	IsUsed    bool      // 会话是否被使用
	SeqID     int64     // 序列号（用于消息排序）
	SessionID uint32    // 会话ID（唯一标识）
	Conn      *AuthConn // 关联的认证连接
}

// Packet 表示协议数据包头部
type Packet struct {
	Module  int   // 模块标识（区分不同功能模块）
	Flags   int   // 标志位（如加密标识、压缩标识等）
	Seq     int64 // 序列号（用于消息校验和重传）
	DataLen int   // 数据部分长度
}

// 设备认证相关的消息类型

// GetDeviceIDMsg 获取设备ID的消息
type GetDeviceIDMsg struct {
	Cmd      string `json:"TECmd"`      // 命令类型
	Data     string `json:"TEData"`     // 消息数据
	DeviceID string `json:"TEDeviceId"` // 设备ID
}

// VerifyIPMsg IP验证消息（用于设备IP和端口等信息的验证）
type VerifyIPMsg struct {
	Code          int    `json:"CODE"`            // 状态码（成功/失败标识）
	BusMaxVersion int    `json:"BUS_MAX_VERSION"` // 总线支持的最大版本
	BusMinVersion int    `json:"BUS_MIN_VERSION"` // 总线支持的最小版本
	AuthPort      int    `json:"AUTH_PORT"`       // 认证端口
	SessionPort   int    `json:"SESSION_PORT"`    // 会话端口
	ConnCap       int    `json:"CONN_CAP"`        // 连接能力（如最大连接数）
	DeviceName    string `json:"DEVICE_NAME"`     // 设备名称
	DeviceType    string `json:"DEVICE_TYPE"`     // 设备类型
	DeviceID      string `json:"DEVICE_ID"`       // 设备ID
	VersionType   string `json:"VERSION_TYPE"`    // 版本类型
}

// VerifyDeviceIDMsg 设备ID验证消息（用于验证设备身份的合法性）
type VerifyDeviceIDMsg struct {
	Code     int    `json:"CODE"`      // 状态码（成功/失败标识）
	DeviceID string `json:"DEVICE_ID"` // 待验证的设备ID
}
