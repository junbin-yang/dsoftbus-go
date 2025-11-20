package hichain

import (
	"fmt"
	"sync"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

var (
	instanceMu sync.Mutex
	instances  = make(map[uint32]*HiChainHandle) // 实例映射表，通过会话ID（SessionID）管理HiChain实例
)

// GetInstance 创建一个新的HiChain实例
// 参数：
//   - identity：会话标识信息（包含会话ID、服务类型等）
//   - deviceType：设备类型（HCAccessory或HCController）
//   - callback：回调函数集合（用于数据传输、参数获取等）
// 返回：
//   - 创建的HiChain实例
//   - 错误（若参数无效则返回错误）
func GetInstance(identity *SessionIdentity, deviceType int, callback *HCCallBack) (*HiChainHandle, error) {
	if identity == nil || callback == nil {
		return nil, fmt.Errorf("无效参数")
	}

	instanceMu.Lock()
	defer instanceMu.Unlock()

	// 创建HiChain实例并初始化状态为初始态
	handle := &HiChainHandle{
		identity:   identity,
		deviceType: deviceType,
		callback:   callback,
		state:      StateInit,
	}

	// 将实例存入映射表（以会话ID为键）
	instances[identity.SessionID] = handle

	log.Infof("[HICHAIN] 为会话 %d 创建实例，设备类型 %d",
		identity.SessionID, deviceType)

	return handle, nil
}

// Destroy 销毁一个HiChain实例
// 参数：
//   - handle：指向HiChain实例指针的指针（用于销毁后将实例置空，避免悬垂指针）
func Destroy(handle **HiChainHandle) {
	if handle == nil || *handle == nil {
		return
	}

	instanceMu.Lock()
	defer instanceMu.Unlock()

	// 从映射表中删除实例
	sessionID := (*handle).identity.SessionID
	delete(instances, sessionID)

	log.Infof("[HICHAIN] 销毁会话 %d 的实例", sessionID)

	// 将传入的指针置空，确保外部无法再访问已销毁的实例
	*handle = nil
}

// ReceiveData 处理接收到的认证数据
// 参数：
//   - data：接收到的原始数据字节流
// 返回：
//   - 错误（若处理过程出错）
func (h *HiChainHandle) ReceiveData(data []byte) error {
	if h == nil {
		return fmt.Errorf("无效的句柄")
	}

	log.Infof("[HICHAIN] 接收数据：会话=%d，长度=%d",
		h.identity.SessionID, len(data))

	// ⚠️ 关键修复：清理JSON数据中的控制字符（C代码可能添加\x00等）
	// 真实鸿蒙设备发送的JSON可能包含C字符串终止符，需要清理后才能正常解析
	cleanedData := CleanJSONData(data)

	// 解包消息（将清理后的字节流解析为消息结构）
	msg, err := unpackMessage(cleanedData)
	if err != nil {
		log.Errorf("[HICHAIN] 消息解包失败：%v\n", err)
		return err
	}

	// 根据消息类型处理
	// 注意：MsgTypePakeRequest==MsgTypeAuthStart==1, MsgTypePakeClientConfirm==MsgTypeAuthChallenge==2
	switch msg.MessageType {
	case 1: // MsgTypePakeRequest / MsgTypeAuthStart
		log.Infof("[HICHAIN] 收到认证请求消息（type=%d）", msg.MessageType)
		return h.handleAuthStart(msg, cleanedData)

	case 2: // MsgTypePakeClientConfirm / MsgTypeAuthChallenge
		log.Infof("[HICHAIN] 收到认证响应/确认消息（type=%d）", msg.MessageType)
		return h.handleAuthChallenge(msg)

	case 3: // MsgTypePakeExchangeReq / MsgTypeAuthResponse
		if msg.Payload != nil {
			log.Infof("[HICHAIN] 收到PAKE EXCHANGE请求（PAKE_EXCHANGE_REQUEST）")
			return h.handleExchangeRequest(msg)
		} else {
			log.Infof("[HICHAIN] 收到认证响应消息（AUTH_RESPONSE）")
			return h.handleAuthResponse(msg)
		}

	case MsgTypeAuthConfirm:
		log.Infof("[HICHAIN] 收到认证确认消息（AUTH_CONFIRM）")
		return h.handleAuthConfirm(msg)

	case MsgTypePakeResponse:
		log.Infof("[HICHAIN] 收到PAKE响应消息（PAKE_RESPONSE）")
		return h.handleAuthChallenge(msg)

	case MsgTypePakeServerConfirm:
		log.Infof("[HICHAIN] 收到PAKE服务端确认消息（PAKE_SERVER_CONFIRM）")
		return h.handleAuthConfirm(msg)

	case MsgTypeError:
		log.Errorf("[HICHAIN] ✗ 收到错误消息：errorCode=%d", msg.ErrorCode)
		h.state = StateFailed
		h.callback.SetServiceResult(h.identity, HCAuthFailed)
		return fmt.Errorf("HiChain认证错误：errorCode=%d", msg.ErrorCode)

	default:
		log.Errorf("[HICHAIN] ✗ 未知消息类型：%d (0x%x)", msg.MessageType, msg.MessageType)
		return fmt.Errorf("未知消息类型：%d", msg.MessageType)
	}
}

// StartAuth 启动认证流程
// 返回：
//   - 错误（若句柄无效或状态非法）
func (h *HiChainHandle) StartAuth() error {
	if h == nil {
		return fmt.Errorf("无效的句柄")
	}

	// 只有初始态（StateInit）可以启动认证
	if h.state != StateInit {
		return fmt.Errorf("无效状态: %d", h.state)
	}

	log.Infof("[HICHAIN] 启动会话 %d 的认证流程",
		h.identity.SessionID)

	return h.startAuthentication()
}

// GetState 返回当前认证状态
// 返回：
//   - 当前状态（StateInit/StateStarted等，若句柄无效则返回StateFailed）
func (h *HiChainHandle) GetState() int {
	if h == nil {
		return StateFailed
	}
	return h.state
}

// GetSessionKey 返回派生的会话密钥
// 返回：
//   - 会话密钥字节数组（若句柄无效则返回nil）
func (h *HiChainHandle) GetSessionKey() []byte {
	if h == nil {
		return nil
	}
	return h.sessionKey
}
