package authmanager

import (
	"fmt"
	"net"
	"sync"

	"github.com/junbin-yang/dsoftbus-go/pkg/authmanager/hichain"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// ClientAuthInterface 客户端认证接口，用于客户端发起HiChain认证
type ClientAuthInterface struct {
	client       *AuthClient              // 关联的认证客户端
	conn         net.Conn                 // 网络连接
	sessionID    uint32                   // HiChain会话ID
	hcHandle     *hichain.HiChainHandle   // HiChain句柄
	hcHandleMu   sync.Mutex               // HiChain句柄的互斥锁
	sessionKey   []byte                   // 协商完成的会话密钥
	authComplete chan error               // 认证完成通知通道
}

// NewClientAuthInterface 创建客户端认证接口
// 参数：
//   - client：认证客户端
//   - conn：网络连接
//   - sessionID：会话ID
// 返回：
//   - 初始化后的ClientAuthInterface实例
func NewClientAuthInterface(client *AuthClient, conn net.Conn, sessionID uint32) *ClientAuthInterface {
	return &ClientAuthInterface{
		client:       client,
		conn:         conn,
		sessionID:    sessionID,
		authComplete: make(chan error, 1),
	}
}

// OnTransmit 通过连接发送HiChain数据（实现HiChain的传输回调）
// 参数：
//   - identity：会话标识信息
//   - data：待发送的数据
// 返回：
//   - 错误信息（发送失败时）
func (cai *ClientAuthInterface) OnTransmit(identity *hichain.SessionIdentity, data []byte) error {
	log.Infof("[CLIENT_AUTH] 发送数据: 会话=%d, 长度=%d", identity.SessionID, len(data))

	// 通过AUTH_SDK模块发送数据
	// 重要：在整个HiChain认证过程中使用固定的seq（sessionID）
	// 这样服务端会将所有消息路由到同一个HiChainHandle实例
	seqNum := int64(identity.SessionID)

	return AuthConnPostBytes(cai.conn, ModuleAuthSDK, 0, seqNum, data, nil)
}

// GetProtocolParams 获取协议参数（实现HiChain的参数获取回调）
// 参数：
//   - identity：会话标识信息
//   - operationCode：操作码
// 返回：
//   - 协议参数
//   - 错误信息（获取失败时）
func (cai *ClientAuthInterface) GetProtocolParams(identity *hichain.SessionIdentity, operationCode int32) (*hichain.ProtocolParams, error) {
	log.Infof("[CLIENT_AUTH] 获取协议参数: 会话=%d, 操作码=%d", identity.SessionID, operationCode)

	// 构建并返回协议参数
	params := &hichain.ProtocolParams{
		KeyLength:  AuthSessionKeyLen,                // 会话密钥长度（16字节）
		SelfAuthID: cai.client.localDevInfo.DeviceID, // 本地设备ID
		PeerAuthID: cai.client.authConn.AuthID,       // 对端设备认证ID
	}

	return params, nil
}

// SetSessionKey 存储派生的会话密钥（实现HiChain的密钥设置回调）
// 参数：
//   - identity：会话标识信息
//   - sessionKey：HiChain生成的会话密钥
// 返回：
//   - 错误信息（存储失败时）
func (cai *ClientAuthInterface) SetSessionKey(identity *hichain.SessionIdentity, sessionKey *hichain.SessionKey) error {
	log.Infof("[CLIENT_AUTH] 设置会话密钥: 会话=%d, 密钥长度=%d", identity.SessionID, sessionKey.Length)

	// 保存会话密钥
	cai.sessionKey = make([]byte, sessionKey.Length)
	copy(cai.sessionKey, sessionKey.Key)

	log.Infof("[CLIENT_AUTH] 会话密钥已保存")
	return nil
}

// SetServiceResult 处理认证结果（实现HiChain的结果通知回调）
// 参数：
//   - identity：会话标识信息
//   - result：认证结果（0表示成功）
// 返回：
//   - 错误信息（处理失败时）
func (cai *ClientAuthInterface) SetServiceResult(identity *hichain.SessionIdentity, result int32) error {
	log.Infof("[CLIENT_AUTH] 认证结果: 会话=%d, 结果=%d", identity.SessionID, result)

	if result == hichain.HCOk {
		log.Infof("[CLIENT_AUTH] 会话%d认证成功", identity.SessionID)
		// 通知认证完成
		cai.authComplete <- nil
	} else {
		log.Errorf("[CLIENT_AUTH] 会话%d认证失败", identity.SessionID)
		cai.authComplete <- fmt.Errorf("认证失败，结果码: %d", result)
	}

	// 清理HiChain句柄
	cai.destroyHiChain()

	return nil
}

// ConfirmReceiveRequest 确认接收请求（实现HiChain的接收确认回调）
// 参数：
//   - identity：会话标识信息
//   - operationCode：操作码
// 返回：
//   - 处理结果（HCOk表示成功）
func (cai *ClientAuthInterface) ConfirmReceiveRequest(identity *hichain.SessionIdentity, operationCode int32) int32 {
	log.Infof("[CLIENT_AUTH] 确认接收请求: 会话=%d, 操作码=%d", identity.SessionID, operationCode)
	return hichain.HCOk // 总是返回成功
}

// initHiChain 初始化HiChain实例
// 返回：
//   - 错误信息（初始化失败时）
func (cai *ClientAuthInterface) initHiChain() error {
	cai.hcHandleMu.Lock()
	defer cai.hcHandleMu.Unlock()

	// 检查是否已初始化
	if cai.hcHandle != nil {
		return nil
	}

	log.Infof("[CLIENT_AUTH] 初始化会话%d的HiChain", cai.sessionID)

	// 创建会话标识
	identity := &hichain.SessionIdentity{
		SessionID:     cai.sessionID,
		PackageName:   AuthDefaultID, // 默认包名
		ServiceType:   AuthDefaultID, // 默认服务类型
		OperationCode: 0,             // 操作码（预留）
	}

	// 创建HiChain回调函数（绑定当前ClientAuthInterface的方法）
	callback := &hichain.HCCallBack{
		OnTransmit:            cai.OnTransmit,
		GetProtocolParams:     cai.GetProtocolParams,
		SetSessionKey:         cai.SetSessionKey,
		SetServiceResult:      cai.SetServiceResult,
		ConfirmReceiveRequest: cai.ConfirmReceiveRequest,
	}

	// 获取HiChain实例（客户端使用HCController类型）
	handle, err := hichain.GetInstance(identity, hichain.HCController, callback)
	if err != nil {
		log.Errorf("[CLIENT_AUTH] 获取HiChain实例失败: %v", err)
		return err
	}

	cai.hcHandle = handle
	log.Info("[CLIENT_AUTH] HiChain初始化成功")

	return nil
}

// destroyHiChain 销毁HiChain实例
func (cai *ClientAuthInterface) destroyHiChain() {
	cai.hcHandleMu.Lock()
	defer cai.hcHandleMu.Unlock()

	if cai.hcHandle != nil {
		hichain.Destroy(&cai.hcHandle)
		cai.hcHandle = nil
		log.Infof("[CLIENT_AUTH] 销毁会话%d的HiChain", cai.sessionID)
	}
}

// StartAuth 启动HiChain认证流程
// 返回：
//   - 错误信息（启动失败时）
func (cai *ClientAuthInterface) StartAuth() error {
	// 初始化HiChain
	if err := cai.initHiChain(); err != nil {
		return fmt.Errorf("初始化HiChain失败: %w", err)
	}

	// 启动认证
	cai.hcHandleMu.Lock()
	handle := cai.hcHandle
	cai.hcHandleMu.Unlock()

	if handle == nil {
		return fmt.Errorf("HiChain句柄为空")
	}

	log.Infof("[CLIENT_AUTH] 启动HiChain认证流程")
	return handle.StartAuth()
}

// ProcessReceivedData 处理接收到的HiChain数据
// 参数：
//   - data：接收到的数据
// 返回：
//   - 错误信息（处理失败时）
func (cai *ClientAuthInterface) ProcessReceivedData(data []byte) error {
	cai.hcHandleMu.Lock()
	handle := cai.hcHandle
	cai.hcHandleMu.Unlock()

	if handle == nil {
		return fmt.Errorf("HiChain句柄为空")
	}

	// 交由HiChain处理数据
	return handle.ReceiveData(data)
}

// WaitForCompletion 等待认证完成
// 返回：
//   - 认证结果错误（认证失败时）
func (cai *ClientAuthInterface) WaitForCompletion() error {
	log.Info("[CLIENT_AUTH] 等待认证完成...")
	return <-cai.authComplete
}

// GetSessionKey 获取协商完成的会话密钥
// 返回：
//   - 会话密钥字节数组
func (cai *ClientAuthInterface) GetSessionKey() []byte {
	return cai.sessionKey
}
