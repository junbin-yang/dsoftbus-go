package hichain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// 协议消息类型
const (
	MsgTypeAuthStart     = 1 // 认证开始消息
	MsgTypeAuthChallenge = 2 // 认证挑战消息
	MsgTypeAuthResponse  = 3 // 认证响应消息
	MsgTypeAuthConfirm   = 4 // 认证确认消息
	MsgTypeAuthResult    = 5 // 认证结果消息
)

// AuthMessage 表示HiChain认证消息结构
type AuthMessage struct {
	MessageType int    `json:"message"`             // 消息类型（对应MsgTypeXXX常量）
	SessionID   uint32 `json:"sessionId"`           // 会话ID（关联对应的认证会话）
	Challenge   string `json:"challenge,omitempty"` // 挑战值（可选，用于身份验证）
	Response    string `json:"response,omitempty"`  // 响应值（可选，对挑战的应答）
	AuthID      string `json:"authId,omitempty"`    // 认证ID（可选，发送方的身份标识）
	Result      int    `json:"result,omitempty"`    // 结果（可选，认证成功/失败标识）
}

// generateChallenge 生成随机挑战值（用于认证过程中的身份验证）
func generateChallenge() ([]byte, error) {
	challenge := make([]byte, 32) // 生成32字节的随机挑战值
	_, err := rand.Read(challenge)
	if err != nil {
		return nil, err
	}
	return challenge, nil
}

// computeResponse 计算认证响应值（基于挑战值和认证ID）
func computeResponse(challenge []byte, authID string) []byte {
	h := sha256.New()
	h.Write(challenge)      // 混入挑战值
	h.Write([]byte(authID)) // 混入认证ID（身份标识）
	return h.Sum(nil)       // 返回SHA256哈希结果作为响应
}

// deriveSessionKey 从认证数据派生会话密钥
// 重要：为了确保双方派生相同的密钥，使用字典序对设备ID排序
func deriveSessionKey(challenge []byte, response []byte, selfAuthID, peerAuthID string) []byte {
	h := sha256.New()
	h.Write(challenge) // 混入挑战值
	h.Write(response)  // 混入响应值

	// 按字典序排序设备ID，确保双方使用相同的顺序
	// 这样无论是客户端还是服务端，都会得到相同的密钥
	var id1, id2 string
	if selfAuthID < peerAuthID {
		id1 = selfAuthID
		id2 = peerAuthID
	} else {
		id1 = peerAuthID
		id2 = selfAuthID
	}

	h.Write([]byte(id1)) // ID1（较小）
	h.Write([]byte(id2)) // ID2（较大）

	hash := h.Sum(nil) // 计算SHA256哈希

	// 取哈希结果的前16字节作为会话密钥（符合SessionKeyLength定义）
	sessionKey := make([]byte, SessionKeyLength)
	copy(sessionKey, hash[:SessionKeyLength])

	// 添加详细调试日志
	fmt.Printf("[HICHAIN] 密钥派生参数:\n")
	if len(challenge) >= 8 {
		fmt.Printf("  challenge: %x\n", challenge[:8])
	} else {
		fmt.Printf("  challenge: %x (长度=%d)\n", challenge, len(challenge))
	}
	if len(response) >= 8 {
		fmt.Printf("  response: %x\n", response[:8])
	} else {
		fmt.Printf("  response: %x (长度=%d)\n", response, len(response))
	}
	fmt.Printf("  selfAuthID: %s\n", selfAuthID)
	fmt.Printf("  peerAuthID: %s\n", peerAuthID)
	fmt.Printf("  排序后 id1: %s\n", id1)
	fmt.Printf("  排序后 id2: %s\n", id2)
	fmt.Printf("  派生密钥: %x\n", sessionKey)

	return sessionKey
}

// packMessage 将认证消息打包为JSON字节流
func packMessage(msg *AuthMessage) ([]byte, error) {
	return json.Marshal(msg)
}

// unpackMessage 将JSON字节流解包为认证消息结构
func unpackMessage(data []byte) (*AuthMessage, error) {
	var msg AuthMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// startAuthentication 启动认证流程（作为发起方）
func (h *HiChainHandle) startAuthentication() error {
	// 获取协议参数（包含自身和对端的认证ID等）
	params, err := h.callback.GetProtocolParams(h.identity, OpCodeAuthenticate)
	if err != nil {
		return err
	}

	h.selfAuthID = params.SelfAuthID // 记录自身认证ID
	h.peerAuthID = params.PeerAuthID // 记录对端认证ID

	// 生成随机挑战值
	challenge, err := generateChallenge()
	if err != nil {
		return err
	}

	// 保存本地挑战值（后续用于密钥派生）
	h.ourChallenge = challenge

	// 创建认证开始消息
	msg := &AuthMessage{
		MessageType: MsgTypeAuthStart,
		SessionID:   h.identity.SessionID,
		Challenge:   fmt.Sprintf("%x", challenge), // 挑战值转为十六进制字符串
		AuthID:      h.selfAuthID,                 // 携带自身认证ID
	}

	// 打包并发送消息
	data, err := packMessage(msg)
	if err != nil {
		return err
	}

	err = h.callback.OnTransmit(h.identity, data)
	if err != nil {
		return err
	}

	h.state = StateStarted // 更新状态为"已启动"
	return nil
}

// handleAuthStart 处理认证开始消息（作为接收方）
func (h *HiChainHandle) handleAuthStart(msg *AuthMessage) error {
	h.peerAuthID = msg.AuthID // 记录发送方（对端）的认证ID

	// 解码对端挑战值（从十六进制字符串）
	peerChallenge, err := hex.DecodeString(msg.Challenge)
	if err != nil {
		return fmt.Errorf("解码对端挑战值失败: %w", err)
	}
	h.peerChallenge = peerChallenge // 保存对端挑战值（后续用于密钥派生）

	// 获取协议参数（包含自身认证ID等）
	params, err := h.callback.GetProtocolParams(h.identity, OpCodeAuthenticate)
	if err != nil {
		return err
	}
	h.selfAuthID = params.SelfAuthID // 记录自身认证ID

	// 计算对"对端挑战值"的响应
	response := computeResponse(peerChallenge, h.selfAuthID)

	// 生成自身的挑战值（用于验证对端）
	ourChallenge, err := generateChallenge()
	if err != nil {
		return err
	}

	// 保存本地挑战值（后续用于密钥派生）
	h.ourChallenge = ourChallenge

	// 构建挑战响应消息（携带自身挑战值和对端挑战的响应）
	respMsg := &AuthMessage{
		MessageType: MsgTypeAuthChallenge,
		SessionID:   h.identity.SessionID,
		Challenge:   fmt.Sprintf("%x", ourChallenge), // 自身挑战值转为十六进制
		Response:    fmt.Sprintf("%x", response),     // 对端挑战的响应转为十六进制
		AuthID:      h.selfAuthID,                    // 携带自身认证ID
	}

	// 打包并发送消息
	data, err := packMessage(respMsg)
	if err != nil {
		return err
	}

	err = h.callback.OnTransmit(h.identity, data)
	if err != nil {
		return err
	}

	h.state = StateAuthenticating // 更新状态为"认证中"
	return nil
}

// handleAuthChallenge 处理认证挑战消息（作为发起方收到挑战时）
func (h *HiChainHandle) handleAuthChallenge(msg *AuthMessage) error {
	// 验证对端的响应（简化实现，生产环境中应与本地存储的挑战值比对）
	h.peerAuthID = msg.AuthID // 记录对端认证ID

	// 解码对端挑战值（从十六进制字符串）
	peerChallenge, err := hex.DecodeString(msg.Challenge)
	if err != nil {
		return fmt.Errorf("解码对端挑战值失败: %w", err)
	}
	h.peerChallenge = peerChallenge // 保存对端挑战值（后续用于密钥派生）

	// 计算对"对端新挑战值"的响应
	response := computeResponse(peerChallenge, h.selfAuthID)

	// 构建响应消息
	respMsg := &AuthMessage{
		MessageType: MsgTypeAuthResponse,
		SessionID:   h.identity.SessionID,
		Response:    fmt.Sprintf("%x", response), // 响应值转为十六进制
	}

	// 打包并发送消息
	data, err := packMessage(respMsg)
	if err != nil {
		return err
	}

	err = h.callback.OnTransmit(h.identity, data)
	if err != nil {
		return err
	}

	// 派生会话密钥（基于双方的挑战和响应）
	// 重要：使用我们自己刚计算的response（对serverChallenge的响应）
	// 而不是msg.Response（那是服务端对clientChallenge的响应）
	sessionKey := deriveSessionKey(h.peerChallenge, response, h.selfAuthID, h.peerAuthID)
	h.sessionKey = sessionKey

	// 通知上层设置会话密钥
	err = h.callback.SetSessionKey(h.identity, &SessionKey{
		Key:    sessionKey,
		Length: SessionKeyLength,
	})
	if err != nil {
		return err
	}

	h.state = StateCompleted // 更新状态为"认证完成"

	// 发送认证确认消息
	confirmMsg := &AuthMessage{
		MessageType: MsgTypeAuthConfirm,
		SessionID:   h.identity.SessionID,
		Result:      HCOk, // 标识认证成功
	}

	data, err = packMessage(confirmMsg)
	if err != nil {
		return err
	}

	err = h.callback.OnTransmit(h.identity, data)
	if err != nil {
		return err
	}

	// 通知上层认证成功
	h.callback.SetServiceResult(h.identity, HCOk)

	return nil
}

// handleAuthResponse 处理认证响应消息（作为接收方收到响应时）
func (h *HiChainHandle) handleAuthResponse(msg *AuthMessage) error {
	// 验证响应（简化实现，生产环境中应与本地存储的挑战值比对）

	// 解码对端的响应值（从十六进制字符串）
	peerResponse, err := hex.DecodeString(msg.Response)
	if err != nil {
		return fmt.Errorf("解码对端响应值失败: %w", err)
	}

	// 派生会话密钥（使用本地发出的挑战值）
	// 服务端（接收方）使用自己发出的挑战值和客户端的响应值派生密钥
	sessionKey := deriveSessionKey(h.ourChallenge, peerResponse, h.selfAuthID, h.peerAuthID)
	h.sessionKey = sessionKey

	// 通知上层设置会话密钥
	err = h.callback.SetSessionKey(h.identity, &SessionKey{
		Key:    sessionKey,
		Length: SessionKeyLength,
	})
	if err != nil {
		return err
	}

	h.state = StateCompleted // 更新状态为"认证完成"

	// 通知上层认证成功
	h.callback.SetServiceResult(h.identity, HCOk)

	return nil
}

// handleAuthConfirm 处理认证确认消息
func (h *HiChainHandle) handleAuthConfirm(msg *AuthMessage) error {
	if msg.Result == HCOk {
		// 确认认证成功
		h.state = StateCompleted
		h.callback.SetServiceResult(h.identity, HCOk)
	} else {
		// 确认认证失败
		h.state = StateFailed
		h.callback.SetServiceResult(h.identity, HCAuthFailed)
	}
	return nil
}
