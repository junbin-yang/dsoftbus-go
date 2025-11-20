package device_auth

// ============================================================================
// 常量定义（对应C的enum和define）
// ============================================================================

// OsAccountEnum 本地系统账户类型
const (
	DefaultOsAccount int32 = 0  // 默认本地系统账户
	AnyOsAccount     int32 = -2 // 前台用户的本地系统账户
)

// GroupType 组类型
type GroupType int32

const (
	AllGroup                    GroupType = 0    // 所有组类型（用于查询）
	IdenticalAccountGroup       GroupType = 1    // 相同云账户组
	PeerToPeerGroup             GroupType = 256  // P2P绑定组
	AcrossAccountAuthorizeGroup GroupType = 1282 // 跨账户授权组
)

// GroupVisibility 组可见性类型
type GroupVisibility int32

const (
	GroupVisibilityPrivate GroupVisibility = 0  // 私有组
	GroupVisibilityPublic  GroupVisibility = -1 // 公开组
)

// GroupOperationCode 组操作代码
type GroupOperationCode int32

const (
	GroupCreate  GroupOperationCode = 0 // 组创建
	GroupDisband GroupOperationCode = 1 // 组销毁
	MemberInvite GroupOperationCode = 2 // 邀请对端设备加入本地可信组
	MemberJoin   GroupOperationCode = 3 // 加入对端可信组
	MemberDelete GroupOperationCode = 4 // 与对端设备解绑
)

// GroupAuthForm 组认证类型
type GroupAuthForm int32

const (
	AuthFormInvalidType       GroupAuthForm = -1 // 无效的组认证类型
	AuthFormAccountUnrelated  GroupAuthForm = 0  // P2P组认证类型
	AuthFormIdenticalAccount  GroupAuthForm = 1  // 相同云账户的组认证类型
	AuthFormAcrossAccount     GroupAuthForm = 2  // 跨账户共享的组认证类型
)

// CredType 凭证类型
type CredType int32

const (
	SymmetricCred  CredType = 1 // 对称凭证类型
	AsymmetricCred CredType = 2 // 非对称凭证类型
)

// UserType 设备类型
type UserType int32

const (
	DeviceTypeAccessory  UserType = 0 // 设备类型是附件
	DeviceTypeController UserType = 1 // 设备类型是控制器
	DeviceTypeProxy      UserType = 2 // 设备类型是代理
)

// RequestResponse 请求响应结果
type RequestResponse uint32

const (
	RequestRejected RequestResponse = 0x80000005 // 拒绝来自对端设备的请求
	RequestAccepted RequestResponse = 0x80000006 // 接受来自对端设备的请求
)

// CredOperationCode 凭证操作代码
const (
	CredOpInvalid int32 = -1 // 无效操作码（用于初始化）
	CredOpQuery   int32 = 0  // ProcessCredential的查询凭证操作码
	CredOpCreate  int32 = 1  // ProcessCredential的创建凭证操作码
	CredOpImport  int32 = 2  // ProcessCredential的导入凭证操作码
	CredOpDelete  int32 = 3  // ProcessCredential的删除凭证操作码
)

// ReturnFlag 返回标志
const (
	ReturnFlagInvalid   int32 = -1 // 无效标志（用于初始化）
	ReturnFlagDefault   int32 = 0  // 仅返回结果的标志
	ReturnFlagPublicKey int32 = 1  // 返回结果和公钥的标志
)

// AcquireType 获取类型
type AcquireType int32

const (
	AcquireTypeInvalid AcquireType = -1 // 无效获取类型（用于初始化）
	P2PBind            AcquireType = 0  // P2P绑定的获取类型
)

// ProtocolExpandValue 协议扩展值（用于绑定）
type ProtocolExpandValue int32

const (
	// 与轻量级设备交互时，使用此标志支持基于对称凭证的绑定
	LiteProtocolStandardMode ProtocolExpandValue = 1
	// 与使用短PIN的ISO的轻量级设备交互时使用此标志
	LiteProtocolCompatibilityMode ProtocolExpandValue = 2
)

// HiChain错误码
const (
	HC_SUCCESS           int32 = 0
	HC_ERR               int32 = -1
	HC_ERR_INVALID_PARAMS int32 = -2
)

// ============================================================================
// 回调接口定义
// ============================================================================

// DataChangeListener 提供监视可信组和设备变更的能力
type DataChangeListener struct {
	// OnGroupCreated 创建新组时调用
	OnGroupCreated func(groupInfo string)

	// OnGroupDeleted 销毁组时调用
	OnGroupDeleted func(groupInfo string)

	// OnDeviceBound 组添加可信设备时调用
	OnDeviceBound func(peerUdid string, groupInfo string)

	// OnDeviceUnBound 组删除可信设备时调用
	OnDeviceUnBound func(peerUdid string, groupInfo string)

	// OnDeviceNotTrusted 设备在所有组中都没有信任关系时调用
	OnDeviceNotTrusted func(peerUdid string)

	// OnLastGroupDeleted 设备在某种类型的所有组中都没有信任关系时调用
	OnLastGroupDeleted func(peerUdid string, groupType int32)

	// OnTrustedDeviceNumChanged 可信设备数量变化时调用
	OnTrustedDeviceNumChanged func(curTrustedDeviceNum int32)
}

// DeviceAuthCallback 描述业务需要提供的回调
type DeviceAuthCallback struct {
	// OnTransmit 有数据需要发送时调用
	// 返回true表示发送成功，false表示发送失败
	OnTransmit func(requestId int64, data []byte) bool

	// OnSessionKeyReturned 返回会话密钥时调用
	OnSessionKeyReturned func(requestId int64, sessionKey []byte)

	// OnFinish 异步操作成功时调用
	OnFinish func(requestId int64, operationCode int32, returnData string)

	// OnError 异步操作失败时调用
	OnError func(requestId int64, operationCode int32, errorCode int32, errorReturn string)

	// OnRequest 接收来自其他设备的请求时调用
	// 返回响应的JSON字符串
	OnRequest func(requestId int64, operationCode int32, reqParams string) string
}

// ============================================================================
// 核心接口定义
// ============================================================================

// GroupAuthManager 提供组认证的所有能力
type GroupAuthManager interface {
	// ProcessData 处理认证数据
	ProcessData(authReqId int64, data []byte, gaCallback *DeviceAuthCallback) error

	// AuthDevice 在设备之间发起认证
	AuthDevice(osAccountId int32, authReqId int64, authParams string, gaCallback *DeviceAuthCallback) error

	// CancelRequest 取消认证过程
	CancelRequest(requestId int64, appId string)

	// GetRealInfo 通过假名ID获取真实信息
	GetRealInfo(osAccountId int32, pseudonymId string) (string, error)

	// GetPseudonymId 通过索引获取假名ID
	GetPseudonymId(osAccountId int32, indexKey string) (string, error)
}

// DeviceGroupManager 提供设备组管理的所有能力
type DeviceGroupManager interface {
	// RegCallback 注册业务回调
	RegCallback(appId string, callback *DeviceAuthCallback) error

	// UnRegCallback 注销业务回调
	UnRegCallback(appId string) error

	// RegDataChangeListener 注册数据变更监听回调
	RegDataChangeListener(appId string, listener *DataChangeListener) error

	// UnRegDataChangeListener 注销数据变更监听回调
	UnRegDataChangeListener(appId string) error

	// CreateGroup 创建可信组
	CreateGroup(osAccountId int32, requestId int64, appId string, createParams string) error

	// DeleteGroup 删除可信组
	DeleteGroup(osAccountId int32, requestId int64, appId string, disbandParams string) error

	// AddMemberToGroup 将可信设备添加到可信组
	AddMemberToGroup(osAccountId int32, requestId int64, appId string, addParams string) error

	// DeleteMemberFromGroup 从可信组删除可信设备
	DeleteMemberFromGroup(osAccountId int32, requestId int64, appId string, deleteParams string) error

	// ProcessData 处理绑定或解绑设备的数据
	ProcessData(requestId int64, data []byte) error

	// AddMultiMembersToGroup 批量添加具有账户关系的可信设备
	AddMultiMembersToGroup(osAccountId int32, appId string, addParams string) error

	// DelMultiMembersFromGroup 批量删除具有账户关系的可信设备
	DelMultiMembersFromGroup(osAccountId int32, appId string, deleteParams string) error

	// GetRegisterInfo 获取本地设备的注册信息
	GetRegisterInfo(reqJsonStr string) (string, error)

	// CheckAccessToGroup 检查指定应用是否具有组的访问权限
	CheckAccessToGroup(osAccountId int32, appId string, groupId string) error

	// GetPkInfoList 获取与设备相关的所有公钥信息
	GetPkInfoList(osAccountId int32, appId string, queryParams string) ([]string, error)

	// GetGroupInfoById 获取组的组信息
	GetGroupInfoById(osAccountId int32, appId string, groupId string) (string, error)

	// GetGroupInfo 获取满足查询参数的组的组信息
	GetGroupInfo(osAccountId int32, appId string, queryParams string) ([]string, error)

	// GetJoinedGroups 获取特定组类型的所有组信息
	GetJoinedGroups(osAccountId int32, appId string, groupType GroupType) ([]string, error)

	// GetRelatedGroups 获取与某个设备相关的所有组信息
	GetRelatedGroups(osAccountId int32, appId string, peerDeviceId string) ([]string, error)

	// GetDeviceInfoById 获取可信设备的信息
	GetDeviceInfoById(osAccountId int32, appId string, deviceId string, groupId string) (string, error)

	// GetTrustedDevices 获取组中的所有可信设备信息
	GetTrustedDevices(osAccountId int32, appId string, groupId string) ([]string, error)

	// IsDeviceInGroup 查询组中是否存在指定设备
	IsDeviceInGroup(osAccountId int32, appId string, groupId string, deviceId string) bool

	// CancelRequest 取消绑定或解绑过程
	CancelRequest(requestId int64, appId string)

	// DestroyInfo 销毁内部分配的内存返回的信息
	DestroyInfo(returnInfo *string)
}
