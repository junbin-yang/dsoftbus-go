package hichain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// 协议消息类型
const (
	MsgTypePakeRequest       = 1      // PAKE请求 (0x0001)
	MsgTypePakeResponse      = 0x8001 // PAKE响应 (32769)
	MsgTypePakeClientConfirm = 2      // PAKE客户端确认 (0x0002)
	MsgTypePakeServerConfirm = 0x8002 // PAKE服务端确认 (32770)
	MsgTypePakeExchangeReq   = 3      // PAKE EXCHANGE请求 (0x0003)
	MsgTypePakeExchangeResp  = 0x8003 // PAKE EXCHANGE响应 (32771)
	MsgTypeError             = 0x8080 // 错误消息 (32896)

	// 旧协议兼容（已废弃）
	MsgTypeAuthStart     = 1
	MsgTypeAuthChallenge = 2
	MsgTypeAuthResponse  = 3
	MsgTypeAuthConfirm   = 4
	MsgTypeAuthResult    = 5
)

// 认证形式（authForm）
const (
	AuthFormInvalid          = -1 // 无效类型
	AuthFormAccountUnrelated = 0  // 无账户关联认证（默认）
	AuthFormIdenticalAccount = 1  // 同账户认证
	AuthFormAcrossAccount    = 2  // 跨账户认证
)

// VersionInfo 版本信息
type VersionInfo struct {
	MinVersion     string `json:"minVersion"`
	CurrentVersion string `json:"currentVersion"`
}

// PakePayload PAKE协议的payload结构
type PakePayload struct {
	Salt          string       `json:"salt,omitempty"`          // PAKE salt (hex)
	Epk           string       `json:"epk,omitempty"`           // Ephemeral public key (hex)
	Challenge     string       `json:"challenge,omitempty"`     // Nonce/challenge (hex)
	KcfData       string       `json:"kcfData,omitempty"`       // Key confirmation data (hex)
	Version       *VersionInfo `json:"version,omitempty"`       // 版本信息
	Support256Mod bool         `json:"support256mod,omitempty"` // 是否支持256位模
	OperationCode int          `json:"operationCode,omitempty"` // 操作码
	ErrorCode     int          `json:"errorCode,omitempty"`     // 错误码
	ExAuthInfo    string       `json:"exAuthInfo,omitempty"`    // 扩展认证信息 (EXCHANGE阶段)
	PeerAuthID    string       `json:"peerAuthId,omitempty"`    // 对端认证ID (EXCHANGE阶段，hex编码)
	PeerUserType  int          `json:"peerUserType,omitempty"`  // 对端用户类型 (EXCHANGE阶段)
}

// AuthMessage 表示HiChain认证消息结构
type AuthMessage struct {
	MessageType int          `json:"message"`             // 消息类型（对应MsgTypeXXX常量）
	SessionID   uint32       `json:"sessionId,omitempty"` // 会话ID（关联对应的认证会话）
	RequestID   string       `json:"requestId,omitempty"` // 请求ID（HarmonyOS需要此字段来关联请求）
	Payload     *PakePayload `json:"payload,omitempty"`   // PAKE Payload信息

	// EXCHANGE阶段设备信息字段
	PeerDeviceID string `json:"peerDeviceId,omitempty"` // 服务器设备ID（对端设备ID）
	PeerAuthID   string `json:"peerAuthId,omitempty"`   // 服务器认证ID（备用）
	ConnDeviceID string `json:"connDeviceId,omitempty"` // 客户端设备ID（连接设备ID）
	PeerUdid     string `json:"peerUdid,omitempty"`     // 客户端设备ID（备用）

	// 旧协议字段（兼容）
	AuthForm  int    `json:"authForm,omitempty"`  // 认证形式
	Challenge string `json:"challenge,omitempty"` // 挑战值
	Response  string `json:"response,omitempty"`  // 响应值
	AuthID    string `json:"authId,omitempty"`    // 认证ID
	Result    int    `json:"result,omitempty"`    // 结果
	ErrorCode int    `json:"errorCode,omitempty"` // 错误码
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
	// ⚠️ 关键修复：HarmonyOS v4.1.4要求必须包含authForm字段
	// 否则会报错：Failed to get auth form! (HC_ERR_JSON_GET = 8195)
	msg := &AuthMessage{
		MessageType: MsgTypeAuthStart,
		SessionID:   h.identity.SessionID,
		AuthForm:    AuthFormAccountUnrelated,     // ⚠️ 设置认证形式为"无账户关联"
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

// handleAuthStart 处理PAKE_REQUEST（作为服务器接收方）
func (h *HiChainHandle) handleAuthStart(msg *AuthMessage, rawData []byte) error {
	log.Infof("[HICHAIN] 处理PAKE_REQUEST")

	h.requestID = msg.RequestID

	// 从消息中提取客户端设备ID
	if msg.ConnDeviceID != "" {
		h.peerAuthID = msg.ConnDeviceID
	} else if msg.PeerDeviceID != "" {
		h.peerAuthID = msg.PeerDeviceID
	}

	// 获取PIN码和服务器设备ID（从AuthSessionContext）
	params, err := h.callback.GetProtocolParams(h.identity, OpCodeAuthenticate)
	if err != nil {
		return err
	}
	pinCode := params.PinCode
	if pinCode == "" {
		pinCode = "888888" // 默认PIN
	}

	// ⚠️ 关键：服务器设备ID必须从params获取，不能从消息中提取
	// 因为PAKE_REQUEST中的peerDeviceId和connDeviceId都是客户端的设备ID
	h.selfAuthID = params.SelfAuthID
	if h.selfAuthID == "" {
		return fmt.Errorf("服务器设备ID未设置，请在AuthSessionContext中设置LocalDeviceID")
	}

	//log.Infof("[HICHAIN] 设备ID信息: selfAuthID=%s, peerAuthID=%s", h.selfAuthID, h.peerAuthID)

	// PAKE V1 EC-SPEKE 协议实现
	// 1. 生成服务器salt (16字节，符合HarmonyOS标准实现)
	salt, err := generateRandomBytes(16)
	if err != nil {
		return fmt.Errorf("生成salt失败: %w", err)
	}
	h.pakeSalt = salt

	// 2. 从PIN派生PSK并计算SPEKE基点
	psk := []byte(pinCode)
	base, err := computeX25519BasePoint(psk, salt)
	if err != nil {
		return fmt.Errorf("计算基点失败: %w", err)
	}
	h.pakeBase = base

	// 3. 生成临时密钥对
	// EC-SPEKE-X25519: epk = esk * base
	// 注意：必须使用GenerateX25519KeyPair生成私钥，它会进行X25519标准的clamping操作
	// 然后重新计算公钥 = esk * base（SPEKE使用自定义base而不是标准基点G）
	eskSelf, _, err := generateX25519KeyPair() // 生成私钥（已clamped），忽略基于G的公钥
	if err != nil {
		return fmt.Errorf("生成X25519私钥失败: %w", err)
	}
	// 计算临时公钥：epkSelf = eskSelf * base（SPEKE）
	epkSelf, err := computeX25519PublicKey(eskSelf, base)
	if err != nil {
		return fmt.Errorf("计算X25519临时公钥失败: %w", err)
	}
	h.pakeEsk = eskSelf
	h.pakeEpk = epkSelf

	// 4. 生成challenge (16字节)
	challenge, err := generateRandomBytes(16)
	if err != nil {
		return fmt.Errorf("生成challenge失败: %w", err)
	}
	h.ourChallenge = challenge

	//log.Infof("[HICHAIN] === 生成PAKE_RESPONSE ===")
	//log.Infof("[HICHAIN]   - salt (16字节): %s", bytesToHex(salt))
	//log.Infof("[HICHAIN]   - epk (32字节): %s", bytesToHex(epkSelf))
	//log.Infof("[HICHAIN]   - challenge (16字节): %s", bytesToHex(challenge))

	// 5. 构建PAKE_RESPONSE
	// 使用客户端请求中的版本信息（如果有），否则使用默认值
	version := &VersionInfo{
		MinVersion:     "1.0.0",
		CurrentVersion: "2.0.26",
	}
	if msg.Payload != nil && msg.Payload.Version != nil {
		version = msg.Payload.Version
	}

	respMsg := &AuthMessage{
		MessageType: MsgTypePakeResponse,
		RequestID:   h.requestID,
		Payload: &PakePayload{
			Salt:      bytesToHex(salt),
			Epk:       bytesToHex(epkSelf),
			Challenge: bytesToHex(challenge),
			Version:   version,
		},
	}

	data, err := packMessage(respMsg)
	if err != nil {
		return err
	}

	err = h.callback.OnTransmit(h.identity, data)
	if err != nil {
		return err
	}

	h.state = StateAuthenticating
	return nil
}

// handleAuthChallenge 处理PAKE_CLIENT_CONFIRM（服务端收到客户端确认）
func (h *HiChainHandle) handleAuthChallenge(msg *AuthMessage) error {
	log.Infof("[HICHAIN] 处理PAKE_CLIENT_CONFIRM")

	if msg.Payload == nil {
		return fmt.Errorf("payload为空")
	}

	// 1. 解析客户端epk
	clientEpk, err := hexToBytes(msg.Payload.Epk)
	if err != nil {
		return fmt.Errorf("解析客户端epk失败: %w", err)
	}
	h.pakePeerEpk = clientEpk

	// 2. 解析客户端challenge
	clientChallenge, err := hexToBytes(msg.Payload.Challenge)
	if err != nil {
		return fmt.Errorf("解析客户端challenge失败: %w", err)
	}
	h.peerChallenge = clientChallenge

	// 3. 计算共享密钥: sharedSecret = eskSelf * epkPeer
	sharedSecret, err := computeX25519SharedSecret(h.pakeEsk, clientEpk)
	if err != nil {
		return fmt.Errorf("计算共享密钥失败: %w", err)
	}
	h.pakeSharedSec = sharedSecret

	log.Infof("[HICHAIN] === 计算共享密钥 ===")
	log.Infof("[HICHAIN]   - 服务器esk: %s", bytesToHex(h.pakeEsk))
	log.Infof("[HICHAIN]   - 客户端epk: %s", bytesToHex(clientEpk))
	log.Infof("[HICHAIN]   - sharedSecret: %s", bytesToHex(sharedSecret))
	log.Infof("[HICHAIN]   - salt: %s", bytesToHex(h.pakeSalt))

	// 4. 派生unionKey (PAKE V1需要48字节: sessionKey[0:16] + hmacKey[16:48])
	unionKey, err := deriveSessionKey(sharedSecret, h.pakeSalt, 48)
	if err != nil {
		return fmt.Errorf("派生unionKey失败: %w", err)
	}

	// 分割unionKey
	sessionKey := unionKey[0:16]
	hmacKey := unionKey[16:48]
	h.sessionKey = sessionKey

	log.Infof("[HICHAIN] === 验证客户端kcfData ===")
	log.Infof("[HICHAIN] 输入参数:")
	log.Infof("[HICHAIN]   - unionKey (48字节): %s", bytesToHex(unionKey))
	log.Infof("[HICHAIN]   - sessionKey [0:16]: %s", bytesToHex(sessionKey))
	log.Infof("[HICHAIN]   - hmacKey [16:48]: %s", bytesToHex(hmacKey))
	log.Infof("[HICHAIN]   - serverChallenge (我们的, 16字节): %s", bytesToHex(h.ourChallenge))
	log.Infof("[HICHAIN]   - clientChallenge (对端的, 16字节): %s", bytesToHex(clientChallenge))

	// 5. 解析并验证客户端的kcfData
	clientKcfData, err := hexToBytes(msg.Payload.KcfData)
	if err != nil {
		return fmt.Errorf("解析客户端kcfData失败: %w", err)
	}

	// 验证客户端的kcfData (VerifyProof)
	// 服务器验证客户端: message = challengePeer + challengeSelf = challengeClient + challengeServer
	expectedClientKcf := computeKcfDataV1(hmacKey, h.ourChallenge, clientChallenge, false)

	log.Infof("[HICHAIN] 验证公式: HMAC-SHA256(hmacKey, challengeClient + challengeServer)")
	log.Infof("[HICHAIN]   - 拼接消息 (32字节): %s", bytesToHex(append(clientChallenge, h.ourChallenge...)))

	// 直接逐字节比较
	if len(expectedClientKcf) != len(clientKcfData) {
		log.Errorf("[HICHAIN] ✗ 客户端kcfData长度不匹配: 期望%d, 实际%d", len(expectedClientKcf), len(clientKcfData))
		return fmt.Errorf("客户端kcfData验证失败")
	}

	match := true
	for i := 0; i < len(expectedClientKcf); i++ {
		if expectedClientKcf[i] != clientKcfData[i] {
			match = false
			break
		}
	}

	if !match {
		log.Errorf("[HICHAIN] ✗ 客户端kcfData验证失败")
		log.Errorf("[HICHAIN]   期望: %s", bytesToHex(expectedClientKcf))
		log.Errorf("[HICHAIN]   实际: %s", bytesToHex(clientKcfData))
		return fmt.Errorf("客户端kcfData验证失败")
	}
	log.Infof("[HICHAIN] ✓ 客户端kcfData验证成功")

	// 6. 通知上层会话密钥
	h.callback.SetSessionKey(h.identity, &SessionKey{
		Key:    sessionKey,
		Length: int32(len(sessionKey)),
	})

	// 7. 生成服务器的kcfData (GenerateProof)
	serverKcfData := computeKcfDataV1(hmacKey, h.ourChallenge, clientChallenge, true)

	// 8. 发送PAKE_SERVER_CONFIRM (PAKE V1不包含challenge字段)
	confirmMsg := &AuthMessage{
		MessageType: MsgTypePakeServerConfirm,
		RequestID:   h.requestID,
		Payload: &PakePayload{
			KcfData: bytesToHex(serverKcfData),
			// PAKE V1协议不在PAKE_SERVER_CONFIRM中发送challenge
		},
	}

	data, err := packMessage(confirmMsg)
	if err != nil {
		return err
	}

	err = h.callback.OnTransmit(h.identity, data)
	if err != nil {
		return err
	}

	// PAKE V1: 发送PAKE_SERVER_CONFIRM后等待客户端确认或EXCHANGE请求
	// 暂不立即设置为完成状态,等待后续消息
	h.state = StateAuthenticating
	log.Infof("[HICHAIN] PAKE_SERVER_CONFIRM已发送,等待客户端响应")
	return nil
}

// handleAuthResponse 处理旧协议响应（兼容）
func (h *HiChainHandle) handleAuthResponse(msg *AuthMessage) error {
	log.Infof("[HICHAIN] 收到旧协议响应（暂不支持）")
	return fmt.Errorf("旧协议暂不支持")
}

// handleAuthConfirm 处理认证确认消息
func (h *HiChainHandle) handleAuthConfirm(msg *AuthMessage) error {
	if msg.Result == HCOk {
		// 确认认证成功
		h.state = StateCompleted
		log.Infof("[HICHAIN] ✓ 认证成功完成！")
		h.callback.SetServiceResult(h.identity, HCOk)
	} else {
		// 确认认证失败
		h.state = StateFailed
		log.Errorf("[HICHAIN] ✗ 认证失败 (result=%d)", msg.Result)
		h.callback.SetServiceResult(h.identity, HCAuthFailed)
	}
	return nil
}

// handleExchangeRequest 处理PAKE EXCHANGE请求（完整实现）
func (h *HiChainHandle) handleExchangeRequest(msg *AuthMessage) error {
	log.Infof("[HICHAIN] 处理PAKE_EXCHANGE_REQUEST")

	if msg.Payload == nil || msg.Payload.ExAuthInfo == "" {
		return fmt.Errorf("EXCHANGE请求缺少exAuthInfo")
	}

	// 1. 解析客户端的exAuthInfo
	exAuthInfo, err := hexToBytes(msg.Payload.ExAuthInfo)
	if err != nil {
		return fmt.Errorf("解析exAuthInfo失败: %w", err)
	}

	log.Infof("[HICHAIN] 收到exAuthInfo长度: %d字节", len(exAuthInfo))

	// 2. 解密客户端数据: nonce(12字节) + cipher
	if len(exAuthInfo) < 12 {
		return fmt.Errorf("exAuthInfo长度不足")
	}

	clientNonce := exAuthInfo[:12]
	cipherData := exAuthInfo[12:]

	log.Infof("[HICHAIN] 客户端nonce: %s", bytesToHex(clientNonce))
	log.Infof("[HICHAIN] 密文长度: %d字节", len(cipherData))

	// 使用sessionKey解密
	plaintext, err := decryptAesGcm(h.sessionKey, clientNonce, cipherData, []byte("hichain_exchange_request"))
	if err != nil {
		log.Errorf("[HICHAIN] 解密失败: %v", err)
		return fmt.Errorf("解密客户端数据失败: %w", err)
	}

	log.Infof("[HICHAIN] 解密成功，明文长度: %d字节", len(plaintext))

	// 3. 分离authInfo和signature (签名64字节)
	if len(plaintext) < 64 {
		return fmt.Errorf("明文长度不足")
	}

	authInfoLen := len(plaintext) - 64
	clientAuthInfoJSON := plaintext[:authInfoLen]
	clientSignature := plaintext[authInfoLen:]

	log.Infof("[HICHAIN] 客户端authInfo: %s", string(CleanJSONData(clientAuthInfoJSON)))
	log.Infof("[HICHAIN] 客户端签名: %s", bytesToHex(clientSignature))

	// 4. 解析并验证客户端签名
	var clientAuthInfo map[string]interface{}
	if err := json.Unmarshal(CleanJSONData(clientAuthInfoJSON), &clientAuthInfo); err != nil {
		return fmt.Errorf("解析客户端authInfo失败: %w", err)
	}

	clientAuthPkHex, ok := clientAuthInfo["authPk"].(string)
	if !ok {
		return fmt.Errorf("客户端authInfo缺少authPk字段")
	}

	clientPublicKey, err := hexToBytes(clientAuthPkHex)
	if err != nil || len(clientPublicKey) != 32 {
		return fmt.Errorf("客户端公钥格式错误")
	}

	// 解析客户端authId（hex编码的设备ID）
	clientAuthIdHex, _ := clientAuthInfo["authId"].(string)
	var clientDeviceID string
	if clientAuthIdBytes, err := hexToBytes(clientAuthIdHex); err == nil {
		clientDeviceID = string(clientAuthIdBytes)
		h.peerAuthID = clientDeviceID // 更新对端设备ID
		log.Infof("[HICHAIN] 解析到客户端设备ID: %s", clientDeviceID)
	}

	// 验证签名: challengePeer + challengeSelf + authInfo
	// 客户端生成时视角：challengeClient(自己) + challengeServer(对端) + authInfo
	verifyMsg := append(h.peerChallenge, h.ourChallenge...)
	verifyMsg = append(verifyMsg, clientAuthInfoJSON...)

	if !verifyED25519Signature(clientPublicKey, verifyMsg, clientSignature) {
		log.Errorf("[HICHAIN] ✗ 客户端签名验证失败")
		return fmt.Errorf("客户端签名验证失败")
	}
	log.Infof("[HICHAIN] ✓ 客户端签名验证成功")

	// 5. 生成或加载服务器的ED25519密钥对
	if len(h.longTermPrivateKey) == 0 {
		// 尝试从内存缓存加载
		privateKey, publicKey := GetLocalPrivateKey(h.selfAuthID)
		if privateKey != nil {
			h.longTermPrivateKey = privateKey
			h.longTermPublicKey = publicKey
			log.Infof("[HICHAIN] 使用缓存的ED25519密钥对")
		} else {
			// 生成新密钥对
			privateKey, publicKey, err := generateED25519KeyPair()
			if err != nil {
				return fmt.Errorf("生成ED25519密钥对失败: %w", err)
			}
			h.longTermPrivateKey = privateKey
			h.longTermPublicKey = publicKey
			// 保存到内存缓存
			SaveLocalPrivateKey(h.selfAuthID, privateKey, publicKey)
			log.Infof("[HICHAIN] 生成并缓存ED25519密钥对: pubKey=%s", bytesToHex(publicKey))
		}
	}

	// 5. 生成服务器的authInfo和签名
	serverAuthInfo := map[string]interface{}{
		"authId": bytesToHex([]byte(h.selfAuthID)), // authId需要hex编码
		"authPk": bytesToHex(h.longTermPublicKey),
	}

	serverAuthInfoJSON, _ := json.Marshal(serverAuthInfo)

	// 拼接待签名消息
	// 签名消息顺序必须是 challengeSelf + challengePeer + authInfo
	// 服务器视角：challengeServer(自己) + challengeClient(对端) + authInfo
	signMessage := append(h.ourChallenge, h.peerChallenge...)
	signMessage = append(signMessage, serverAuthInfoJSON...)

	// 使用ED25519签名（兼容HarmonyOS HUKS的SHA256预哈希）
	serverSignature, err := signED25519(h.longTermPrivateKey, signMessage)
	if err != nil {
		return fmt.Errorf("ED25519签名失败: %w", err)
	}

	// 5. 拼接authInfo + signature
	serverPlaintext := append(serverAuthInfoJSON, serverSignature...)

	// 6. 生成服务器nonce并加密(标准绑定交换使用12字节)
	serverNonce, err := generateRandomBytes(12)
	if err != nil {
		log.Errorf("[EXCHANGE] 生成服务器 nonce 失败: %v", err)
		return err
	}

	serverCipher, err := encryptAesGcm(h.sessionKey, serverNonce, serverPlaintext, []byte("hichain_exchange_response"))
	if err != nil {
		return fmt.Errorf("加密服务器数据失败: %w", err)
	}

	// 7. 拼接nonce + cipher
	serverExAuthInfo := append(serverNonce, serverCipher...)

	// 8. 发送EXCHANGE响应（包含设备ID信息）
	log.Infof("[HICHAIN] === 构造EXCHANGE_RESPONSE ===")
	log.Infof("[HICHAIN]   selfAuthID (服务器): %s", h.selfAuthID)
	log.Infof("[HICHAIN]   clientDeviceID (客户端): %s", clientDeviceID)
	log.Infof("[HICHAIN]   payload.peerAuthId (hex): %s", bytesToHex([]byte(h.selfAuthID)))

	respMsg := &AuthMessage{
		MessageType:  MsgTypePakeExchangeResp,
		RequestID:    h.requestID,
		PeerDeviceID: h.selfAuthID,   // 服务器设备ID
		PeerAuthID:   h.selfAuthID,   // 服务器认证ID（备用）
		ConnDeviceID: clientDeviceID, // 客户端设备ID
		PeerUdid:     clientDeviceID, // 客户端设备ID（备用）
		Payload: &PakePayload{
			ExAuthInfo: bytesToHex(serverExAuthInfo),
			//PeerAuthID:   bytesToHex([]byte(h.selfAuthID)),
			PeerUserType: 0,
		},
	}

	data, err := packMessage(respMsg)
	if err != nil {
		return err
	}

	err = h.callback.OnTransmit(h.identity, data)
	if err != nil {
		return err
	}

	// 9. 保存客户端公钥到内存缓存（用于后续快速认证）
	SaveDeviceAuthInfo(h.peerAuthID, clientPublicKey)
	log.Infof("[HICHAIN] 已缓存对端公钥: deviceID=%s", h.peerAuthID)

	// 10. 保存对端设备信息
	// 待实现bus_center后保存

	// 11. 发送设备信息消息（DATA_TYPE_DEVICE_INFO）
	// 这是完成认证流程必需的步骤，HarmonyOS需要收到设备信息才会显示绑定成功
	// ..

	h.state = StateCompleted
	h.callback.SetServiceResult(h.identity, HCOk)
	log.Infof("[HICHAIN] ✓ 认证成功完成(含EXCHANGE)！")
	return nil
}

// extractSaltFromPakeRequest 从PAKE_REQUEST原始数据中提取客户端salt
func extractSaltFromPakeRequest(data []byte) ([]byte, error) {
	// 解析原始JSON
	var rawMsg map[string]interface{}
	if err := json.Unmarshal(CleanJSONData(data), &rawMsg); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	// 获取msg数组
	msgArray, ok := rawMsg["msg"].([]interface{})
	if !ok || len(msgArray) == 0 {
		return nil, fmt.Errorf("PAKE_REQUEST消息缺少msg数组")
	}

	// 遍历msg数组找到包含salt的元素
	for _, msgItem := range msgArray {
		if msgMap, ok := msgItem.(map[string]interface{}); ok {
			if dataMap, ok := msgMap["data"].(map[string]interface{}); ok {
				if saltHex, ok := dataMap["salt"].(string); ok {
					salt, err := hexToBytes(saltHex)
					if err != nil {
						return nil, fmt.Errorf("解析salt失败: %w", err)
					}
					return salt, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("PAKE_REQUEST消息中未找到salt字段")
}
