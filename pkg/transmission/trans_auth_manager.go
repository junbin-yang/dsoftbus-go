package transmission

import (
	"encoding/json"
	"fmt"

	"github.com/junbin-yang/dsoftbus-go/pkg/authentication"
	"github.com/junbin-yang/dsoftbus-go/pkg/context"
	"github.com/junbin-yang/dsoftbus-go/pkg/device_auth"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// ============================================================================
// Transmission Auth Manager
// 对应C代码: core/transmission/trans_channel/auth/src/trans_auth_manager.c
// ============================================================================

// TransAuthInit 初始化transmission认证管理器
func TransAuthInit() error {
	// 注册AUTH_CHANNEL监听器
	channelListener := &authentication.AuthChannelListener{
		OnDataReceived: onAuthChannelDataRecv,
		OnDisconnected: onDisconnect,
	}
	if err := authentication.RegAuthChannelListener(authentication.ModuleAuthChannel, channelListener); err != nil {
		return err
	}

	// 注册AUTH_MSG监听器
	msgListener := &authentication.AuthChannelListener{
		OnDataReceived: onAuthMsgDataRecv,
		OnDisconnected: onDisconnect,
	}
	if err := authentication.RegAuthChannelListener(authentication.ModuleAuthMsg, msgListener); err != nil {
		authentication.UnregAuthChannelListener(authentication.ModuleAuthChannel)
		return err
	}

	// 注册AUTH_SDK监听器（HiChain PAKE认证）
	sdkListener := &authentication.AuthChannelListener{
		OnDataReceived: onAuthSdkDataRecv,
		OnDisconnected: onDisconnect,
	}
	if err := authentication.RegAuthChannelListener(authentication.ModuleAuthSdk, sdkListener); err != nil {
		authentication.UnregAuthChannelListener(authentication.ModuleAuthChannel)
		authentication.UnregAuthChannelListener(authentication.ModuleAuthMsg)
		return err
	}

	log.Info("[TRANS_AUTH] Transmission auth manager initialized")
	return nil
}

// TransAuthDeinit 反初始化
func TransAuthDeinit() {
	authentication.UnregAuthChannelListener(authentication.ModuleAuthChannel)
	authentication.UnregAuthChannelListener(authentication.ModuleAuthMsg)
	authentication.UnregAuthChannelListener(authentication.ModuleAuthSdk)
	log.Info("[TRANS_AUTH] Transmission auth manager deinitialized")
}

// ============================================================================
// AUTH_CHANNEL 处理 (通道建立)
// ============================================================================

// onAuthChannelDataRecv 处理AUTH_CHANNEL数据
func onAuthChannelDataRecv(channelId int, data *authentication.AuthChannelData) {
	log.Infof("[TRANS_AUTH] Received AUTH_CHANNEL: channelId=%d, flag=%d, len=%d",
		channelId, data.Flag, data.Len)

	// flag=0 表示请求
	if data.Flag != 0 {
		return
	}

	// 清理并解析请求
	cleanData := cleanJSONData(data.Data)
	log.Infof("[TRANS_AUTH] AUTH_CHANNEL request: %s", string(cleanData))

	var req AuthChannelRequestMsg
	if err := json.Unmarshal(cleanData, &req); err != nil {
		log.Errorf("[TRANS_AUTH] Failed to parse request: %v", err)
		return
	}

	// 获取本地设备信息
	localDevInfo, err := authentication.GetLocalDeviceInfo()
	if err != nil {
		log.Errorf("[TRANS_AUTH] Failed to get local device info: %v", err)
		return
	}

	// 构建响应(交换SRC/DST)
	reply := AuthChannelReplyMsg{
		Code:       req.Code,
		DeviceID:   localDevInfo.UDID,
		PkgName:    req.PkgName,
		SrcBusName: req.DstBusName, // 交换
		DstBusName: req.SrcBusName, // 交换
		ReqID:      req.ReqID,
		MTUSize:    authentication.AuthSocketMaxDataLen,
	}

	replyJSON, _ := json.Marshal(reply)
	replyData := &authentication.AuthChannelData{
		Module: authentication.ModuleAuthChannel,
		Flag:   1, // REPLY
		Seq:    data.Seq,
		Len:    uint32(len(replyJSON)),
		Data:   replyJSON,
	}

	if err := authentication.AuthPostChannelData(channelId, replyData); err != nil {
		log.Errorf("[TRANS_AUTH] Failed to send reply: %v", err)
	} else {
		log.Infof("[TRANS_AUTH] Sent AUTH_CHANNEL reply: %s", string(replyJSON))
	}
}

// ============================================================================
// AUTH_SDK 处理 (HiChain PAKE认证)
// ============================================================================

// onAuthSdkDataRecv 处理AUTH_SDK数据（HiChain PAKE认证）
func onAuthSdkDataRecv(channelId int, data *authentication.AuthChannelData) {
	cleanData := cleanJSONData(data.Data)
	log.Infof("[TRANS_AUTH] ========== Received AUTH_SDK ==========")
	log.Infof("[TRANS_AUTH] channelId: %d", channelId)
	log.Infof("[TRANS_AUTH] seq:       %d", data.Seq)
	log.Infof("[TRANS_AUTH] len:       %d", data.Len)
	log.Infof("[TRANS_AUTH] data:      %s", string(cleanData))
	log.Infof("[TRANS_AUTH] ========================================")

	// 获取device_auth实例
	ga, err := device_auth.GetGaInstance()
	if err != nil {
		log.Errorf("[TRANS_AUTH] Failed to get GA instance: %v", err)
		log.Infof("[TRANS_AUTH] ========== AUTH_SDK Processing END ==========\n")
		return
	}

	// 创建回调（用于发送HiChain响应数据）
	callback := &device_auth.DeviceAuthCallback{
		OnTransmit: func(requestId int64, respData []byte) bool {
			cleanResp := cleanJSONData(respData)
			log.Infof("[TRANS_AUTH] → Sending HiChain response: requestId=%d, len=%d", requestId, len(respData))
			log.Infof("[TRANS_AUTH]   Response data: %s", string(cleanResp))

			// 发送HiChain响应数据（使用与接收相同的模块号）
			respChannelData := &authentication.AuthChannelData{
				Module: data.Module, // 使用接收到的模块号（MODULE_AUTH_MSG）
				Flag:   0,
				Seq:    data.Seq,
				Len:    uint32(len(respData)),
				Data:   respData,
			}

			if err := authentication.AuthPostChannelData(channelId, respChannelData); err != nil {
				log.Errorf("[TRANS_AUTH] Failed to send HiChain response: %v", err)
				return false
			}
			return true
		},
		OnSessionKeyReturned: func(requestId int64, sessionKey []byte) {
			log.Infof("[TRANS_AUTH] Session key derived: requestId=%d, keyLen=%d", requestId, len(sessionKey))
			// TODO: 保存session key到SessionKeyManager
		},
		OnFinish: func(requestId int64, operationCode int32, returnData string) {
			log.Infof("[TRANS_AUTH] HiChain auth finished: requestId=%d, opCode=%d", requestId, operationCode)
		},
		OnError: func(requestId int64, operationCode int32, errorCode int32, errorReturn string) {
			log.Errorf("[TRANS_AUTH] HiChain auth error: requestId=%d, errorCode=%d", requestId, errorCode)
		},
	}

	// 调用device_auth处理数据
	authReqId := int64(channelId)
	if err := ga.ProcessData(authReqId, data.Data, callback); err != nil {
		log.Errorf("[TRANS_AUTH] ProcessData failed: %v", err)
	}
	log.Infof("[TRANS_AUTH] ========== AUTH_SDK Processing END ==========\n")
}

// ============================================================================
// AUTH_MSG 处理 (业务数据)
// ============================================================================

// onAuthMsgDataRecv 处理AUTH_MSG数据
func onAuthMsgDataRecv(channelId int, data *authentication.AuthChannelData) {
	cleanData := cleanJSONData(data.Data)
	log.Infof("[TRANS_AUTH] Received AUTH_MSG: channelId=%d, len=%d, data=%s",
		channelId, data.Len, string(cleanData))

	// 先检查是否是HiChain消息（有"message"字段）
	var msgCheck map[string]interface{}
	if err := json.Unmarshal(cleanData, &msgCheck); err != nil {
		log.Errorf("[TRANS_AUTH] Failed to parse AUTH_MSG: %v", err)
		return
	}

	// 如果有"message"字段，说明是HiChain认证消息，转发到AUTH_SDK处理
	if _, hasMessage := msgCheck["message"]; hasMessage {
		log.Infof("[TRANS_AUTH] Detected HiChain message, forwarding to AUTH_SDK handler")
		onAuthSdkDataRecv(channelId, data)
		return
	}

	// 否则按DM消息处理
	var msg DMNegotiateRequest
	if err := json.Unmarshal(cleanData, &msg); err != nil {
		log.Errorf("[TRANS_AUTH] Failed to parse DM message: %v", err)
		return
	}

	// 根据MSG_TYPE处理
	switch msg.MsgType {
	case 80: // MSG_TYPE_NEGOTIATE
		handleDMNegotiate(channelId, data.Seq, &msg)
	case 100: // MSG_TYPE_REQ_AUTH
		handleDMAuthRequest(channelId, data.Seq, cleanData)
	case 104: // MSG_TYPE_AUTH_ACK (客户端确认认证成功)
		log.Infof("[TRANS_AUTH] Received AUTH_ACK from client: channelId=%d", channelId)
	default:
		log.Warnf("[TRANS_AUTH] Unknown MSG_TYPE: %d", msg.MsgType)
	}
}

// ============================================================================
// 断开处理
// ============================================================================

// onDisconnect 处理断开
func onDisconnect(channelId int) {
	log.Infof("[TRANS_AUTH] Disconnected: channelId=%d", channelId)
	// 清理会话上下文
	context.DeleteAuthSessionContext(channelId)
}

// ============================================================================
// 工具函数
// ============================================================================

// cleanJSONData 清理C字符串的\0结尾符
func cleanJSONData(data []byte) []byte {
	for i, b := range data {
		if b == 0 {
			return data[:i]
		}
	}
	return data
}

// ============================================================================
// Device Manager 协商处理
// ============================================================================

// handleDMAuthRequest 处理DM认证请求(MSG_TYPE 100)
func handleDMAuthRequest(channelId int, seq int64, data []byte) {
	var req DMAuthRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Errorf("[TRANS_AUTH] Failed to parse REQ_AUTH: %v", err)
		return
	}

	log.Infof("[TRANS_AUTH] DM REQ_AUTH: AuthType=%d, Token=%s", req.AuthType, req.Token)

	// 生成PIN码
	pinCode := 888888
	log.Infof("[TRANS_AUTH] ")
	log.Infof("[TRANS_AUTH] ┌─────────────────────────────────┐")
	log.Infof("[TRANS_AUTH] │   请在鸿蒙设备上输入PIN码：       │")
	log.Infof("[TRANS_AUTH] │         %06d                    │", pinCode)
	log.Infof("[TRANS_AUTH] └─────────────────────────────────┘")

	// 获取本地设备信息
	localDevInfo, _ := authentication.GetLocalDeviceInfo()

	// 生成唯一的RequestID（使用channelId作为会话标识）
	requestId := int64(channelId)

	// 保存会话上下文（用于后续HiChain认证时获取pinCode等信息）
	log.Infof("[TRANS_AUTH] 💾 Setting AuthSessionContext: ChannelID=%d, RequestID=%d, LocalDeviceID=%s, PeerDeviceID=%s, PinCode=%d",
		channelId, requestId, localDevInfo.UDID, req.LocalDeviceID, pinCode)
	context.SetAuthSessionContext(channelId, &context.AuthSessionContext{
		ChannelID:     channelId,
		PinCode:       fmt.Sprintf("%d", pinCode),
		RequestID:     requestId,
		LocalDeviceID: localDevInfo.UDID,
		PeerDeviceID:  req.LocalDeviceID,
	})

	// 调用device_auth创建群组并添加设备
	groupId, groupName, err := createDeviceGroup(req.LocalDeviceID, pinCode)
	if err != nil {
		log.Errorf("[TRANS_AUTH] Failed to create group: %v", err)
		return
	}

	// 更新context中的groupId
	if ctx, err := context.GetAuthSessionContext(channelId); err == nil {
		ctx.GroupID = groupId
		context.SetAuthSessionContext(channelId, ctx)
	}

	// 将对端设备添加到群组（预添加，实际认证成功后才生效）
	if err := addDeviceToGroup(groupId, req.LocalDeviceID); err != nil {
		log.Warnf("[TRANS_AUTH] Failed to add device to group: %v", err)
	}

	// 构建响应(MSG_TYPE 200)
	// 根据抓包数据，正确的格式是：
	// groupId: JSON字符串 "{\"groupId\":\"xxx\"}"
	// authToken: JSON字符串 "{\"pinCode\":888888}"
	groupIdJson := fmt.Sprintf(`{\"groupId\":\"%s\"}`, groupId)
	authTokenJson := fmt.Sprintf(`{\"pinCode\":%d}`, pinCode)

	// 使用map构建响应，字段名必须小写开头
	respMap := map[string]interface{}{
		"ITF_VER":   "1.1",
		"MSG_TYPE":  200,
		"REPLY":     0,
		"DEVICEID":  localDevInfo.UDID,
		"TOKEN":     req.Token,
		"NETID":     localDevInfo.UDID,
		"REQUESTID": requestId,
		"groupId":   groupIdJson, // 小写，JSON字符串
		"GROUPNAME": groupName,
		"authToken": authTokenJson, // 小写，JSON字符串
	}

	respJSON, _ := json.Marshal(respMap)
	respData := &authentication.AuthChannelData{
		Module: authentication.ModuleAuthMsg,
		Flag:   0,
		Seq:    seq,
		Len:    uint32(len(respJSON)),
		Data:   respJSON,
	}

	if err := authentication.AuthPostChannelData(channelId, respData); err != nil {
		log.Errorf("[TRANS_AUTH] Failed to send RESP_AUTH: %v", err)
		return
	}
	log.Infof("[TRANS_AUTH] Sent DM RESP_AUTH reply (len=%d): %s", len(respJSON), string(respJSON))
	// 注意：不需要主动发起HiChain认证
	// 鸿蒙设备会建立第二个连接(com.huawei.devicegroupmanage)并发送HiChain START_REQUEST
	// 我们在onAuthSdkDataRecv中通过ProcessData响应即可
}

// handleDMNegotiate 处理DM协商请求
func handleDMNegotiate(channelId int, seq int64, req *DMNegotiateRequest) {
	log.Infof("[TRANS_AUTH] DM NEGOTIATE request: ITFVer=%s, AuthType=%d, Reply=%d",
		req.ITFVer, req.AuthType, req.Reply)

	if req.Reply < 0 {
		log.Warnf("[TRANS_AUTH] Peer returned error: %d", req.Reply)
	}

	// 获取本地设备信息
	localDevInfo, err := authentication.GetLocalDeviceInfo()
	if err != nil {
		log.Errorf("[TRANS_AUTH] Failed to get local device info: %v", err)
		return
	}

	// 查询设备是否已认证（检查是否在群组中）
	authed := false
	if req.LocalDeviceID != "" {
		authed, _ = checkDeviceAuthStatus(req.LocalDeviceID)
		if authed {
			log.Infof("[TRANS_AUTH] Device %s already authenticated", req.LocalDeviceID[:8])
		}
	}

	// 构建响应(MSG_TYPE 90)
	resp := DMNegotiateResponse{
		ITFVer:          "1.1",
		MsgType:         90, // RESP_NEGOTIATE
		CryptoSupport:   false,
		AuthType:        req.AuthType,
		Reply:           0, // DM_OK
		LocalDeviceID:   localDevInfo.UDID,
		DMVersion:       "1.1",
		Authed:          authed,
		IsAuthCodeReady: true,
	}

	respJSON, _ := json.Marshal(resp)
	respData := &authentication.AuthChannelData{
		Module: authentication.ModuleAuthMsg,
		Flag:   0,
		Seq:    seq,
		Len:    uint32(len(respJSON)),
		Data:   respJSON,
	}

	if err := authentication.AuthPostChannelData(channelId, respData); err != nil {
		log.Errorf("[TRANS_AUTH] Failed to send DM response: %v", err)
	} else {
		log.Infof("[TRANS_AUTH] Sent DM RESP_NEGOTIATE reply: %s", string(respJSON))
	}
}

// ============================================================================
// 消息结构体
// ============================================================================

// AuthChannelRequestMsg AUTH_CHANNEL请求消息
type AuthChannelRequestMsg struct {
	Code       int    `json:"CODE"`
	DeviceID   string `json:"DEVICE_ID"`
	PkgName    string `json:"PKG_NAME"`
	SrcBusName string `json:"SRC_BUS_NAME"`
	DstBusName string `json:"DST_BUS_NAME"`
	ReqID      string `json:"REQ_ID"`
	MTUSize    int    `json:"MTU_SIZE"`
}

// AuthChannelReplyMsg AUTH_CHANNEL回复消息
type AuthChannelReplyMsg struct {
	Code       int    `json:"CODE"`
	DeviceID   string `json:"DEVICE_ID"`
	PkgName    string `json:"PKG_NAME"`
	SrcBusName string `json:"SRC_BUS_NAME"`
	DstBusName string `json:"DST_BUS_NAME"`
	ReqID      string `json:"REQ_ID"`
	MTUSize    int    `json:"MTU_SIZE"`
}

// DMNegotiateRequest DM协商请求(MSG_TYPE 80)
type DMNegotiateRequest struct {
	MsgType        int    `json:"MSG_TYPE"`
	ITFVer         string `json:"ITF_VER"`
	LocalDeviceID  string `json:"LOCALDEVICEID"`
	AuthType       int    `json:"AUTHTYPE"`
	Reply          int    `json:"REPLY"`
	Authed         bool   `json:"authed"`
	HaveCredential bool   `json:"haveCredential"`
	DMVersion      string `json:"dmVersion"`
	CryptoSupport  bool   `json:"CRYPTOSUPPORT"`
}

// DMNegotiateResponse DM协商响应(MSG_TYPE 90)
type DMNegotiateResponse struct {
	ITFVer          string `json:"ITF_VER"`
	MsgType         int    `json:"MSG_TYPE"`
	CryptoSupport   bool   `json:"CRYPTOSUPPORT"`
	AuthType        int    `json:"AUTHTYPE"`
	Reply           int    `json:"REPLY"`
	LocalDeviceID   string `json:"LOCALDEVICEID"`
	DMVersion       string `json:"dmVersion"`
	Authed          bool   `json:"authed"`
	IsAuthCodeReady bool   `json:"IS_AUTH_CODE_READY"`
}

// DMAuthRequest DM认证请求(MSG_TYPE 100)
type DMAuthRequest struct {
	MsgType       int    `json:"MSG_TYPE"`
	ITFVer        string `json:"ITF_VER"`
	LocalDeviceID string `json:"LOCALDEVICEID"`
	AuthType      int    `json:"AUTHTYPE"`
	Token         string `json:"TOKEN"`
	IsShowDialog  bool   `json:"IS_SHOW_DIALOG"`
	Target        string `json:"TARGET"`
	Visibility    int    `json:"VISIBILITY"`
	Index         int    `json:"INDEX"`
	SliceNum      int    `json:"SLICE"`
}

// DMAuthResponse DM认证响应(MSG_TYPE 200)
type DMAuthResponse struct {
	ITFVer    string `json:"ITF_VER"`
	MsgType   int    `json:"MSG_TYPE"`
	Reply     int    `json:"REPLY"`
	DeviceID  string `json:"DEVICEID"`
	Token     string `json:"TOKEN"`
	NetID     string `json:"NETID"`
	RequestID int64  `json:"REQUESTID"`
	GroupID   string `json:"GROUPID"`
	GroupName string `json:"GROUPNAME"`
	AuthToken string `json:"AUTHTOKEN"`
}

// ============================================================================
// Device Auth 群组管理集成
// ============================================================================

// checkDeviceAuthStatus 检查设备认证状态
func checkDeviceAuthStatus(peerDeviceId string) (authed bool, haveCredential bool) {
	gm, err := device_auth.GetGmInstance()
	if err != nil {
		return false, false
	}

	// 查询与该设备相关的所有群组
	groups, err := gm.GetRelatedGroups(device_auth.AnyOsAccount, "softbus", peerDeviceId)
	if err != nil || len(groups) == 0 {
		return false, false
	}

	// 如果设备在任何群组中，说明已认证
	return true, true
}

// addDeviceToGroup 将设备添加到群组
func addDeviceToGroup(groupId string, deviceId string) error {
	gm, err := device_auth.GetGmInstance()
	if err != nil {
		return fmt.Errorf("failed to get GM instance: %w", err)
	}

	addParams := fmt.Sprintf(`{
		"groupId": "%s",
		"deviceId": "%s",
		"udid": "%s"
	}`, groupId, deviceId, deviceId)

	requestId := int64(2000)
	appId := "softbus"
	if err := gm.AddMemberToGroup(device_auth.AnyOsAccount, requestId, appId, addParams); err != nil {
		return fmt.Errorf("failed to add member: %w", err)
	}

	log.Infof("[TRANS_AUTH] Device added to group: groupId=%s, deviceId=%s", groupId, deviceId[:8])
	return nil
}

// createDeviceGroup 创建device_auth群组
func createDeviceGroup(peerDeviceId string, pinCode int) (groupId string, groupName string, err error) {
	gm, err := device_auth.GetGmInstance()
	if err != nil {
		return "", "", fmt.Errorf("failed to get GM instance: %w", err)
	}

	// 生成群组ID和名称
	groupId = "SOFTBUS_GROUP_" + peerDeviceId[:16]
	groupName = "SoftBusGroup"

	// 构建创建群组参数(JSON格式)
	createParams := fmt.Sprintf(`{
		"groupType": 256,
		"groupName": "%s",
		"groupId": "%s",
		"groupVisibility": 0,
		"userType": 0,
		"expireTime": -1
	}`, groupName, groupId)

	// 创建群组
	requestId := int64(1000)
	appId := "softbus"
	if err := gm.CreateGroup(device_auth.AnyOsAccount, requestId, appId, createParams); err != nil {
		log.Warnf("[TRANS_AUTH] CreateGroup returned: %v (may already exist)", err)
	}

	log.Infof("[TRANS_AUTH] Device group created: groupId=%s", groupId)
	return groupId, groupName, nil
}
