package hichain

import (
	"testing"
)

// TestSessionIdentity 测试SessionIdentity结构体初始化
func TestSessionIdentity(t *testing.T) {
	identity := &SessionIdentity{
		SessionID:     1,
		PackageName:   "test",
		ServiceType:   "test",
		OperationCode: OpCodeAuthenticate,
	}

	// 验证结构体字段初始化是否正确
	if identity.SessionID != 1 {
		t.Errorf("期望SessionID为1，实际为%d", identity.SessionID)
	}
	if identity.PackageName != "test" {
		t.Errorf("期望PackageName为'test'，实际为%s", identity.PackageName)
	}
	if identity.ServiceType != "test" {
		t.Errorf("期望ServiceType为'test'，实际为%s", identity.ServiceType)
	}
	if identity.OperationCode != OpCodeAuthenticate {
		t.Errorf("期望OperationCode为%d，实际为%d", OpCodeAuthenticate, identity.OperationCode)
	}
}

// TestGenerateChallenge 测试随机挑战值生成功能
func TestGenerateChallenge(t *testing.T) {
	challenge, err := generateChallenge()
	if err != nil {
		t.Fatalf("生成挑战值失败: %v", err)
	}

	// 验证挑战值长度是否为32字节
	if len(challenge) != 32 {
		t.Errorf("期望挑战值长度为32，实际为%d", len(challenge))
	}

	// 验证两次生成的挑战值是否不同（随机性测试）
	challenge2, _ := generateChallenge()
	if string(challenge) == string(challenge2) {
		t.Error("两次生成的挑战值应不同（随机数生成异常）")
	}
}

// TestComputeResponse 测试认证响应值计算功能
func TestComputeResponse(t *testing.T) {
	challenge := []byte("test_challenge_12345678901234567890") // 测试用挑战值
	authID := "test_device_id"                                 // 测试用认证ID

	response := computeResponse(challenge, authID)

	// 验证响应值长度（SHA256结果应为32字节）
	if len(response) != 32 {
		t.Errorf("期望响应值长度为32，实际为%d", len(response))
	}

	// 验证相同输入是否生成相同输出（确定性测试）
	response2 := computeResponse(challenge, authID)
	if string(response) != string(response2) {
		t.Error("相同输入应生成相同响应值（计算逻辑不一致）")
	}

	// 验证不同输入是否生成不同输出（唯一性测试）
	response3 := computeResponse([]byte("different_challenge"), authID)
	if string(response) == string(response3) {
		t.Error("不同挑战值应生成不同响应值")
	}
}

// TestDeriveSessionKey 测试会话密钥派生功能
func TestDeriveSessionKey(t *testing.T) {
	challenge := []byte("challenge") // 挑战值
	response := []byte("response")   // 响应值
	selfAuthID := "device_a"         // 自身认证ID
	peerAuthID := "device_b"         // 对端认证ID

	sessionKey := deriveSessionKey(challenge, response, selfAuthID, peerAuthID)

	// 验证会话密钥长度是否符合预期（16字节）
	if len(sessionKey) != SessionKeyLength {
		t.Errorf("期望会话密钥长度为%d，实际为%d", SessionKeyLength, len(sessionKey))
	}

	// 验证相同输入是否生成相同密钥（确定性测试）
	sessionKey2 := deriveSessionKey(challenge, response, selfAuthID, peerAuthID)
	if string(sessionKey) != string(sessionKey2) {
		t.Error("相同输入应生成相同会话密钥（派生逻辑不一致）")
	}

	// 验证不同输入是否生成不同密钥（唯一性测试）
	sessionKey3 := deriveSessionKey([]byte("diff_challenge"), response, selfAuthID, peerAuthID)
	if string(sessionKey) == string(sessionKey3) {
		t.Error("不同挑战值应生成不同会话密钥")
	}
}

// TestPackUnpackMessage 测试消息打包与解包功能
func TestPackUnpackMessage(t *testing.T) {
	// 构建测试消息
	msg := &AuthMessage{
		MessageType: MsgTypeAuthStart,
		SessionID:   12345,
		Challenge:   "test_challenge",
		AuthID:      "test_device",
		Result:      HCOk,
	}

	// 打包消息
	data, err := packMessage(msg)
	if err != nil {
		t.Fatalf("消息打包失败: %v", err)
	}

	// 解包消息
	msg2, err := unpackMessage(data)
	if err != nil {
		t.Fatalf("消息解包失败: %v", err)
	}

	// 验证解包结果与原始消息一致
	if msg2.MessageType != msg.MessageType {
		t.Errorf("消息类型不匹配：期望%d，实际%d", msg.MessageType, msg2.MessageType)
	}
	if msg2.SessionID != msg.SessionID {
		t.Errorf("会话ID不匹配：期望%d，实际%d", msg.SessionID, msg2.SessionID)
	}
	if msg2.Challenge != msg.Challenge {
		t.Errorf("挑战值不匹配：期望%s，实际%s", msg.Challenge, msg2.Challenge)
	}
	if msg2.AuthID != msg.AuthID {
		t.Errorf("认证ID不匹配：期望%s，实际%s", msg.AuthID, msg2.AuthID)
	}
	if msg2.Result != msg.Result {
		t.Errorf("结果值不匹配：期望%d，实际%d", msg.Result, msg2.Result)
	}
}

// TestGetInstance 测试HiChain实例的创建与销毁
func TestGetInstance(t *testing.T) {
	// 构建会话标识
	identity := &SessionIdentity{
		SessionID:   1,
		PackageName: "test",
		ServiceType: "test",
	}

	// 构建测试回调
	callback := &HCCallBack{
		OnTransmit: func(identity *SessionIdentity, data []byte) error {
			return nil
		},
		GetProtocolParams: func(identity *SessionIdentity, operationCode int32) (*ProtocolParams, error) {
			return &ProtocolParams{
				KeyLength:  SessionKeyLength,
				SelfAuthID: "device_a",
				PeerAuthID: "device_b",
			}, nil
		},
		SetSessionKey: func(identity *SessionIdentity, sessionKey *SessionKey) error {
			return nil
		},
		SetServiceResult: func(identity *SessionIdentity, result int32) error {
			return nil
		},
		ConfirmReceiveRequest: func(identity *SessionIdentity, operationCode int32) int32 {
			return HCOk
		},
	}

	// 创建实例
	handle, err := GetInstance(identity, HCAccessory, callback)
	if err != nil {
		t.Fatalf("创建实例失败: %v", err)
	}

	// 验证实例是否有效
	if handle == nil {
		t.Fatal("实例不应为nil")
	}
	if handle.identity.SessionID != identity.SessionID {
		t.Errorf("实例会话ID不匹配：期望%d，实际%d", identity.SessionID, handle.identity.SessionID)
	}
	if handle.deviceType != HCAccessory {
		t.Errorf("实例设备类型不匹配：期望%d，实际%d", HCAccessory, handle.deviceType)
	}
	if handle.GetState() != StateInit {
		t.Errorf("期望初始状态为%d，实际为%d", StateInit, handle.GetState())
	}

	// 验证实例是否已存入全局映射
	instanceMu.Lock()
	_, exists := instances[identity.SessionID]
	instanceMu.Unlock()
	if !exists {
		t.Error("实例未存入全局映射表")
	}

	// 销毁实例
	Destroy(&handle)

	// 验证实例已被销毁
	if handle != nil {
		t.Error("销毁后实例应变为nil")
	}

	// 验证全局映射中已移除实例
	instanceMu.Lock()
	_, exists = instances[identity.SessionID]
	instanceMu.Unlock()
	if exists {
		t.Error("销毁后全局映射表中仍存在实例")
	}
}

// TestHiChainAuthenticationFlow 测试完整的HiChain认证流程
func TestHiChainAuthenticationFlow(t *testing.T) {
	var transmittedData []byte // 存储传输的消息数据
	var sessionKeySet bool     // 标记会话密钥是否已设置
	var resultReceived int32   // 存储认证结果

	// 构建会话标识
	identity := &SessionIdentity{
		SessionID:   100,
		PackageName: "test",
		ServiceType: "test",
	}

	// 构建测试回调（模拟发送和参数获取）
	callback := &HCCallBack{
		OnTransmit: func(identity *SessionIdentity, data []byte) error {
			// 保存传输的数据用于验证
			transmittedData = make([]byte, len(data))
			copy(transmittedData, data)
			return nil
		},
		GetProtocolParams: func(identity *SessionIdentity, operationCode int32) (*ProtocolParams, error) {
			return &ProtocolParams{
				KeyLength:  SessionKeyLength,
				SelfAuthID: "device_test",
				PeerAuthID: "device_peer",
			}, nil
		},
		SetSessionKey: func(identity *SessionIdentity, sessionKey *SessionKey) error {
			sessionKeySet = true
			// 验证会话密钥长度
			if len(sessionKey.Key) != SessionKeyLength {
				t.Errorf("期望密钥长度为%d，实际为%d", SessionKeyLength, len(sessionKey.Key))
			}
			return nil
		},
		SetServiceResult: func(identity *SessionIdentity, result int32) error {
			resultReceived = result
			return nil
		},
		ConfirmReceiveRequest: func(identity *SessionIdentity, operationCode int32) int32 {
			return HCOk
		},
	}

	// 创建实例并启动认证
	handle, err := GetInstance(identity, HCAccessory, callback)
	if err != nil {
		t.Fatalf("创建实例失败: %v", err)
	}
	defer Destroy(&handle)

	// 启动认证流程
	err = handle.StartAuth()
	if err != nil {
		t.Fatalf("启动认证失败: %v", err)
	}

	// 验证状态变为"已启动"
	if handle.GetState() != StateStarted {
		t.Errorf("期望状态为%d，实际为%d", StateStarted, handle.GetState())
	}

	// 验证认证开始消息已发送
	if len(transmittedData) == 0 {
		t.Error("未发送认证开始消息")
	}
	startMsg, err := unpackMessage(transmittedData)
	if err != nil {
		t.Fatalf("解析认证开始消息失败: %v", err)
	}
	if startMsg.MessageType != MsgTypeAuthStart {
		t.Errorf("期望消息类型为%d，实际为%d", MsgTypeAuthStart, startMsg.MessageType)
	}

	// 模拟接收方处理认证开始消息（模拟对端响应）
	// 1. 创建接收方实例
	peerIdentity := &SessionIdentity{SessionID: 100}
	peerHandle, _ := GetInstance(peerIdentity, HCController, callback)
	defer Destroy(&peerHandle)

	// 2. 接收方处理认证开始消息
	err = peerHandle.handleAuthStart(startMsg)
	if err != nil {
		t.Fatalf("接收方处理认证开始消息失败: %v", err)
	}
	// 验证接收方状态变为"认证中"
	if peerHandle.GetState() != StateAuthenticating {
		t.Errorf("接收方期望状态为%d，实际为%d", StateAuthenticating, peerHandle.GetState())
	}
	// 验证接收方发送了挑战消息
	challengeMsg, _ := unpackMessage(transmittedData)
	if challengeMsg.MessageType != MsgTypeAuthChallenge {
		t.Errorf("期望消息类型为%d，实际为%d", MsgTypeAuthChallenge, challengeMsg.MessageType)
	}

	// 模拟发起方处理挑战消息
	err = handle.handleAuthChallenge(challengeMsg)
	if err != nil {
		t.Fatalf("发起方处理挑战消息失败: %v", err)
	}
	// 验证发起方状态变为"认证完成"
	if handle.GetState() != StateCompleted {
		t.Errorf("发起方期望状态为%d，实际为%d", StateCompleted, handle.GetState())
	}
	// 验证会话密钥已设置
	if !sessionKeySet {
		t.Error("会话密钥未被设置")
	}
	// 验证认证结果为成功
	if resultReceived != HCOk {
		t.Errorf("期望认证结果为%d，实际为%d", HCOk, resultReceived)
	}
}

// TestAuthenticationFailure 测试认证失败场景（如无效响应）
func TestAuthenticationFailure(t *testing.T) {
	var resultReceived int32

	// 构建会话和回调
	identity := &SessionIdentity{SessionID: 200}
	callback := &HCCallBack{
		OnTransmit: func(identity *SessionIdentity, data []byte) error {
			return nil
		},
		GetProtocolParams: func(identity *SessionIdentity, operationCode int32) (*ProtocolParams, error) {
			return &ProtocolParams{SelfAuthID: "device_a", PeerAuthID: "device_b"}, nil
		},
		SetSessionKey: func(identity *SessionIdentity, sessionKey *SessionKey) error {
			return nil
		},
		SetServiceResult: func(identity *SessionIdentity, result int32) error {
			resultReceived = result
			return nil
		},
		ConfirmReceiveRequest: func(identity *SessionIdentity, operationCode int32) int32 {
			return HCOk
		},
	}

	handle, _ := GetInstance(identity, HCAccessory, callback)
	defer Destroy(&handle)

	// 构造无效的认证确认消息（结果为失败）
	invalidConfirmMsg := &AuthMessage{
		MessageType: MsgTypeAuthConfirm,
		SessionID:   200,
		Result:      HCAuthFailed,
	}

	// 处理无效消息
	err := handle.handleAuthConfirm(invalidConfirmMsg)
	if err != nil {
		t.Fatalf("处理无效确认消息失败: %v", err)
	}

	// 验证状态变为"认证失败"
	if handle.GetState() != StateFailed {
		t.Errorf("期望状态为%d，实际为%d", StateFailed, handle.GetState())
	}
	// 验证结果为认证失败
	if resultReceived != HCAuthFailed {
		t.Errorf("期望结果为%d，实际为%d", HCAuthFailed, resultReceived)
	}
}

// TestInvalidMessageType 测试处理未知消息类型的场景
func TestInvalidMessageType(t *testing.T) {
	// 构建会话和回调
	identity := &SessionIdentity{SessionID: 300}
	callback := &HCCallBack{
		OnTransmit: func(identity *SessionIdentity, data []byte) error {
			return nil
		},
		GetProtocolParams: func(identity *SessionIdentity, operationCode int32) (*ProtocolParams, error) {
			return &ProtocolParams{}, nil
		},
		SetSessionKey:         func(identity *SessionIdentity, sessionKey *SessionKey) error { return nil },
		SetServiceResult:      func(identity *SessionIdentity, result int32) error { return nil },
		ConfirmReceiveRequest: func(identity *SessionIdentity, operationCode int32) int32 { return HCOk },
	}

	handle, _ := GetInstance(identity, HCAccessory, callback)
	defer Destroy(&handle)

	// 构造未知类型的消息
	invalidMsg := &AuthMessage{
		MessageType: 999, // 不存在的消息类型
		SessionID:   300,
	}
	data, _ := packMessage(invalidMsg)

	// 处理未知消息
	err := handle.ReceiveData(data)
	if err == nil {
		t.Error("处理未知消息类型时应返回错误")
	} else if err.Error() != "未知消息类型：999" {
		t.Errorf("期望错误信息不匹配，实际为：%v", err)
	}
}
