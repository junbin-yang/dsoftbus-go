package hichain

const (
	HCOk            = 0  // 操作成功
	HCError         = -1 // 通用错误
	HCInvalidParams = -2 // 无效参数错误
	HCAuthFailed    = -3 // 认证失败错误

	// 操作码
	OpCodeAuthenticate = 1 // 认证操作
	OpCodeAddMember    = 2 // 添加成员操作
	OpCodeDeleteMember = 3 // 删除成员操作
	OpCodeAddGroup     = 4 // 添加组操作
	OpCodeDeleteGroup  = 5 // 删除组操作

	// 认证状态
	StateInit           = 0 // 初始状态
	StateStarted        = 1 // 已启动状态
	StateAuthenticating = 2 // 认证中状态
	StateCompleted      = 3 // 认证完成状态
	StateFailed         = 4 // 认证失败状态

	// 密钥长度
	SessionKeyLength = 16 // 会话密钥长度（字节）

	// 设备类型
	HCAccessory  = 0 // 配件设备
	HCController = 1 // 控制器设备
)

// HiChain会话标识信息
type SessionIdentity struct {
	SessionID     uint32 // 会话唯一ID
	PackageName   string // 包名
	ServiceType   string // 服务类型
	OperationCode int32  // 操作码（对应OpCodeXXX常量）
}

// ProtocolParams 表示协议参数
type ProtocolParams struct {
	KeyLength  int32  // 密钥长度
	SelfAuthID string // 自身认证ID
	PeerAuthID string // 对端认证ID
	PinCode    string // PIN码（用于PAKE认证）
}

// SessionKey 表示会话密钥
type SessionKey struct {
	Key    []byte // 密钥字节数组
	Length int32  // 密钥长度（字节）
}

// Callback functions

// OnTransmitFunc 传输数据的回调函数
// 参数：会话标识（identity）、待传输数据（data）
type OnTransmitFunc func(identity *SessionIdentity, data []byte) error

// GetProtocolParamsFunc 获取协议参数的回调函数
// 参数：会话标识（identity）、操作码（operationCode）
// 返回：协议参数（ProtocolParams）、错误
type GetProtocolParamsFunc func(identity *SessionIdentity, operationCode int32) (*ProtocolParams, error)

// SetSessionKeyFunc 设置会话密钥的回调函数
// 参数：会话标识（identity）、会话密钥（sessionKey）
type SetSessionKeyFunc func(identity *SessionIdentity, sessionKey *SessionKey) error

// SetServiceResultFunc 设置服务操作结果的回调函数
// 参数：会话标识（identity）、操作结果（result，对应HCOk/HCError等常量）
type SetServiceResultFunc func(identity *SessionIdentity, result int32) error

// ConfirmReceiveRequestFunc 确认接收请求的回调函数
// 参数：会话标识（identity）、操作码（operationCode）
// 返回：处理结果（对应HCOk/HCError等常量）
type ConfirmReceiveRequestFunc func(identity *SessionIdentity, operationCode int32) int32

// HCCallBack 表示HiChain相关的回调函数集合
type HCCallBack struct {
	OnTransmit            OnTransmitFunc            // 传输数据回调
	GetProtocolParams     GetProtocolParamsFunc     // 获取协议参数回调
	SetSessionKey         SetSessionKeyFunc         // 设置会话密钥回调
	SetServiceResult      SetServiceResultFunc      // 设置服务结果回调
	ConfirmReceiveRequest ConfirmReceiveRequestFunc // 确认接收请求回调
}

type HiChainHandle struct {
	identity       *SessionIdentity // 会话标识信息
	deviceType     int              // 设备类型（对应HCAccessory/HCController）
	callback       *HCCallBack      // 回调函数集合
	state          int              // 认证状态（对应StateXXX常量）
	sessionKey     []byte           // 会话密钥字节数组
	peerAuthID     string           // 对端认证ID
	selfAuthID     string           // 自身认证ID
	ourChallenge   []byte           // 本地发出的挑战值（用于密钥派生）
	peerChallenge  []byte           // 对端发来的挑战值（用于密钥派生）
	requestID      string           // 请求ID（从对端接收，用于响应关联）
	dhPrivateKey   interface{}      // DH私钥 (*big.Int)
	dhSharedSecret interface{}      // DH共享密钥 (*big.Int)

	// PAKE协议中间状态（用于多步骤认证）
	pakeBase       []byte // SPEKE基点
	pakeSalt       []byte // 服务器salt
	pakeEsk        []byte // 临时私钥
	pakeEpk        []byte // 临时公钥
	pakePeerEpk    []byte // 对端临时公钥
	pakeSharedSec  []byte // PAKE共享密钥

	// EXCHANGE阶段长期密钥（ED25519）
	longTermPrivateKey []byte // ED25519私钥（64字节）
	longTermPublicKey  []byte // ED25519公钥（32字节）
}
