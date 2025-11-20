package device_auth

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/junbin-yang/dsoftbus-go/pkg/context"
	"github.com/junbin-yang/dsoftbus-go/pkg/device_auth/hichain"
	log "github.com/junbin-yang/dsoftbus-go/pkg/utils/logger"
)

// ============================================================================
// 真实实现 - GroupAuthManager
// ============================================================================

// realGroupAuthManager 是GroupAuthManager的真实实现，内部使用hichain
type realGroupAuthManager struct {
	hichainInstances map[int64]*hichain.HiChainHandle // authReqId -> HiChain实例
	callbacks        map[int64]*DeviceAuthCallback    // authReqId -> 回调
	authMessages     map[int64]map[string]interface{} // authReqId -> 原始认证消息（用于提取authIdC等）
	dmRequestIdMap   map[int64]int64                  // authReqId -> dmRequestId（用于查找AuthSessionContext）
	mu               sync.RWMutex
}

// ProcessData 处理认证数据（对应C的g_hichain->processData）
func (g *realGroupAuthManager) ProcessData(authReqId int64, data []byte, gaCallback *DeviceAuthCallback) error {
	// 解析消息，提取requestId（用于查找AuthSessionContext）
	var authMsg map[string]interface{}
	isPakeRequest := false
	var dmRequestId int64 = authReqId // 默认使用authReqId

	// 清理JSON数据（去除null字节等控制字符）
	jsonData := hichain.CleanJSONData(data)

	if err := json.Unmarshal(jsonData, &authMsg); err == nil {
		if msgType, ok := authMsg["message"].(float64); ok && int(msgType) == 1 {
			isPakeRequest = true
		}
		// 从HiChain消息中提取DM的requestId
		if reqIdVal, ok := authMsg["requestId"]; ok {
			if reqIdStr, ok := reqIdVal.(string); ok {
				if reqId, err := strconv.ParseInt(reqIdStr, 10, 64); err == nil {
					dmRequestId = reqId
				} else {
					log.Warnf("[DEVICE_AUTH] ⚠️ Failed to parse requestId string: %v", err)
				}
			} else {
				log.Warnf("[DEVICE_AUTH] ⚠️ requestId is not a string, type=%T", reqIdVal)
			}
		} else {
			log.Warnf("[DEVICE_AUTH] ⚠️ No requestId field found in HiChain message")
		}
	} else {
		log.Errorf("[DEVICE_AUTH] ❌ Failed to parse JSON: %v", err)
	}

	g.mu.RLock()
	handle := g.hichainInstances[authReqId]
	g.mu.RUnlock()

	// ⚠️ 关键修复：如果是PAKE_REQUEST且已存在旧实例，先销毁旧实例
	if isPakeRequest && handle != nil {
		log.Warnf("[DEVICE_AUTH] ⚠️ Received PAKE_REQUEST but instance exists, destroying old instance: authReqId=%d", authReqId)
		g.mu.Lock()
		hichain.Destroy(&handle)
		delete(g.hichainInstances, authReqId)
		delete(g.callbacks, authReqId)
		delete(g.dmRequestIdMap, authReqId)
		g.mu.Unlock()
		handle = nil
	}

	if handle == nil {
		// 服务端首次收到数据，需要创建实例
		log.Infof("[DEVICE_AUTH] Creating HiChain instance for server side: authReqId=%d, dmRequestId=%d", authReqId, dmRequestId)

		// 保存authReqId到dmRequestId的映射
		g.mu.Lock()
		if g.dmRequestIdMap == nil {
			g.dmRequestIdMap = make(map[int64]int64)
		}
		g.dmRequestIdMap[authReqId] = dmRequestId
		g.mu.Unlock()

		identity := &hichain.SessionIdentity{
			SessionID:     uint32(authReqId),
			PackageName:   AUTH_APPID,
			ServiceType:   AUTH_APPID,
			OperationCode: hichain.OpCodeAuthenticate,
		}

		// 创建回调转换
		hcCallback := g.createHCCallBack(authReqId, gaCallback)

		var err error
		handle, err = hichain.GetInstance(identity, hichain.HCAccessory, hcCallback)
		if err != nil {
			return fmt.Errorf("failed to create HiChain instance: %w", err)
		}

		g.mu.Lock()
		g.hichainInstances[authReqId] = handle
		g.callbacks[authReqId] = gaCallback
		g.mu.Unlock()
	}

	log.Infof("[DEVICE_AUTH] ProcessData: authReqId=%d, dataLen=%d", authReqId, len(data))

	// 保存原始消息（用于GetProtocolParams提取authIdC等）
	if len(authMsg) > 0 {
		g.mu.Lock()
		if g.authMessages == nil {
			g.authMessages = make(map[int64]map[string]interface{})
		}
		g.authMessages[authReqId] = authMsg
		g.mu.Unlock()
	}

	// 调用HiChain处理数据
	if err := handle.ReceiveData(data); err != nil {
		log.Errorf("[DEVICE_AUTH] HiChain ReceiveData failed: %v", err)
		return err
	}

	return nil
}

// AuthDevice 在设备之间发起认证（对应C的g_hichain->authDevice）
func (g *realGroupAuthManager) AuthDevice(osAccountId int32, authReqId int64, authParams string, gaCallback *DeviceAuthCallback) error {
	log.Infof("[DEVICE_AUTH] AuthDevice: osAccountId=%d, authReqId=%d", osAccountId, authReqId)

	// 解析authParams（JSON格式）
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(authParams), &params); err != nil {
		return fmt.Errorf("failed to parse authParams: %w", err)
	}

	// 创建会话标识
	identity := &hichain.SessionIdentity{
		SessionID:     uint32(authReqId),
		PackageName:   AUTH_APPID,
		ServiceType:   AUTH_APPID,
		OperationCode: hichain.OpCodeAuthenticate,
	}

	// 创建回调转换
	hcCallback := g.createHCCallBack(authReqId, gaCallback)

	// 创建HiChain实例
	handle, err := hichain.GetInstance(identity, hichain.HCController, hcCallback)
	if err != nil {
		return fmt.Errorf("failed to create HiChain instance: %w", err)
	}

	// 保存实例和回调
	g.mu.Lock()
	g.hichainInstances[authReqId] = handle
	g.callbacks[authReqId] = gaCallback
	g.mu.Unlock()

	log.Infof("[DEVICE_AUTH] Starting HiChain auth: authReqId=%d", authReqId)

	// 启动认证（异步，通过OnTransmit回调发送数据）
	if err := handle.StartAuth(); err != nil {
		// 启动失败，清理实例
		g.mu.Lock()
		delete(g.hichainInstances, authReqId)
		delete(g.callbacks, authReqId)
		g.mu.Unlock()

		return fmt.Errorf("failed to start auth: %w", err)
	}

	return nil
}

// CancelRequest 取消认证过程（对应C的g_hichain->cancelRequest）
func (g *realGroupAuthManager) CancelRequest(requestId int64, appId string) {
	log.Infof("[DEVICE_AUTH] CancelRequest: requestId=%d, appId=%s", requestId, appId)

	g.mu.Lock()
	defer g.mu.Unlock()

	// 销毁HiChain实例
	if handle, exists := g.hichainInstances[requestId]; exists {
		hichain.Destroy(&handle)
		delete(g.hichainInstances, requestId)
		delete(g.callbacks, requestId)
	}
}

// GetRealInfo 通过假名ID获取真实信息
func (g *realGroupAuthManager) GetRealInfo(osAccountId int32, pseudonymId string) (string, error) {
	log.Infof("[DEVICE_AUTH] GetRealInfo: osAccountId=%d, pseudonymId=%s", osAccountId, pseudonymId)
	return "", fmt.Errorf("not implemented")
}

// GetPseudonymId 通过索引获取假名ID
func (g *realGroupAuthManager) GetPseudonymId(osAccountId int32, indexKey string) (string, error) {
	log.Infof("[DEVICE_AUTH] GetPseudonymId: osAccountId=%d, indexKey=%s", osAccountId, indexKey)
	return "", fmt.Errorf("not implemented")
}

// createHCCallBack 创建HiChain回调，转换为DeviceAuthCallback
func (g *realGroupAuthManager) createHCCallBack(authReqId int64, gaCallback *DeviceAuthCallback) *hichain.HCCallBack {
	return &hichain.HCCallBack{
		// OnTransmit: HiChain需要发送数据时调用
		OnTransmit: func(identity *hichain.SessionIdentity, data []byte) error {
			log.Infof("[DEVICE_AUTH] HiChain OnTransmit: sessionId=%d, dataLen=%d",
				identity.SessionID, len(data))

			if gaCallback != nil && gaCallback.OnTransmit != nil {
				success := gaCallback.OnTransmit(authReqId, data)
				if !success {
					return fmt.Errorf("OnTransmit returned false")
				}
			}
			return nil
		},

		// GetProtocolParams: HiChain需要PIN码和设备ID时调用
		GetProtocolParams: func(identity *hichain.SessionIdentity, operationCode int32) (*hichain.ProtocolParams, error) {
			log.Infof("[DEVICE_AUTH] HiChain GetProtocolParams: sessionId=%d, authReqId=%d", identity.SessionID, authReqId)

			// 从AuthSessionContext获取（客户端场景会预先设置）
			pinCode := "888888" // 默认值
			selfAuthID := ""
			peerAuthID := "" // 服务端场景为空，由HiChainHandle从EXCHANGE消息解析

			// 获取dmRequestId（用于查找AuthSessionContext）
			g.mu.RLock()
			dmRequestId, hasDmRequestId := g.dmRequestIdMap[authReqId]
			g.mu.RUnlock()

			if !hasDmRequestId {
				dmRequestId = authReqId // 回退到使用authReqId
			}

			ctx := context.FindAuthSessionContextByRequestId(dmRequestId)
			if ctx != nil {
				if ctx.PinCode != "" {
					pinCode = ctx.PinCode
					log.Infof("[DEVICE_AUTH] Using pinCode from AuthSessionContext: %s", pinCode)
				}
				if ctx.LocalDeviceID != "" {
					selfAuthID = ctx.LocalDeviceID
					log.Infof("[DEVICE_AUTH] Using selfAuthID from AuthSessionContext: %s", selfAuthID)
				}
				if ctx.PeerDeviceID != "" {
					peerAuthID = ctx.PeerDeviceID
					log.Infof("[DEVICE_AUTH] Using peerAuthID from AuthSessionContext: %s (client scenario)", peerAuthID)
				}
			} else {
				log.Warnf("[DEVICE_AUTH] ⚠️ AuthSessionContext not found for authReqId=%d", authReqId)
			}

			return &hichain.ProtocolParams{
				KeyLength:  hichain.SessionKeyLength,
				SelfAuthID: selfAuthID,
				PeerAuthID: peerAuthID, // 客户端场景有值，服务端场景为空
				PinCode:    pinCode,
			}, nil
		},

		// SetSessionKey: HiChain派生出会话密钥时调用
		SetSessionKey: func(identity *hichain.SessionIdentity, sessionKey *hichain.SessionKey) error {
			log.Infof("[DEVICE_AUTH] HiChain SetSessionKey: sessionId=%d, keyLen=%d",
				identity.SessionID, sessionKey.Length)

			if gaCallback != nil && gaCallback.OnSessionKeyReturned != nil {
				gaCallback.OnSessionKeyReturned(authReqId, sessionKey.Key)
			}
			return nil
		},

		// SetServiceResult: HiChain认证完成（成功或失败）时调用
		SetServiceResult: func(identity *hichain.SessionIdentity, result int32) error {
			log.Infof("[DEVICE_AUTH] HiChain SetServiceResult: sessionId=%d, result=%d",
				identity.SessionID, result)

			if result == hichain.HCOk {
				// 认证成功
				if gaCallback != nil && gaCallback.OnFinish != nil {
					gaCallback.OnFinish(authReqId, int32(identity.OperationCode), "{}")
				}
			} else {
				// 认证失败
				if gaCallback != nil && gaCallback.OnError != nil {
					gaCallback.OnError(authReqId, int32(identity.OperationCode), result, "auth failed")
				}
			}

			// 认证完成后清理实例
			g.mu.Lock()
			if handle, exists := g.hichainInstances[authReqId]; exists {
				hichain.Destroy(&handle)
				delete(g.hichainInstances, authReqId)
				delete(g.callbacks, authReqId)
			}
			g.mu.Unlock()

			return nil
		},

		// ConfirmReceiveRequest: HiChain需要确认接收请求时调用
		ConfirmReceiveRequest: func(identity *hichain.SessionIdentity, operationCode int32) int32 {
			log.Infof("[DEVICE_AUTH] HiChain ConfirmReceiveRequest: sessionId=%d", identity.SessionID)

			// 如果有OnRequest回调，调用它
			if gaCallback != nil && gaCallback.OnRequest != nil {
				response := gaCallback.OnRequest(authReqId, operationCode, "{}")
				if response != "" {
					return hichain.HCOk
				}
			}

			// 默认接受请求
			return hichain.HCOk
		},
	}
}

// ============================================================================
// Stub实现 - DeviceGroupManager（保持不变）
// ============================================================================

// stubDeviceGroupManager 是DeviceGroupManager的stub实现
type stubDeviceGroupManager struct {
	callbacks map[string]*DeviceAuthCallback
	listeners map[string]*DataChangeListener
	groups    map[string]*GroupInfo // groupId -> GroupInfo
	mu        sync.RWMutex
}

// GroupInfo 群组信息
type GroupInfo struct {
	GroupID     string
	GroupName   string
	GroupType   int32
	Visibility  int32
	OwnerUserID string
	CreateTime  int64
	Members     map[string]*DeviceMemberInfo // deviceId -> member info
}

// DeviceMemberInfo 设备成员信息
type DeviceMemberInfo struct {
	DeviceID   string
	UDID       string
	AuthID     string
	UserType   int32
	Credential string
	JoinTime   int64
}

// RegCallback 注册业务回调（stub实现）
func (d *stubDeviceGroupManager) RegCallback(appId string, callback *DeviceAuthCallback) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Infof("[DEVICE_AUTH] RegCallback: appId=%s", appId)
	if d.callbacks == nil {
		d.callbacks = make(map[string]*DeviceAuthCallback)
	}
	d.callbacks[appId] = callback
	return nil
}

// UnRegCallback 注销业务回调（stub实现）
func (d *stubDeviceGroupManager) UnRegCallback(appId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Infof("[DEVICE_AUTH] UnRegCallback: appId=%s", appId)
	delete(d.callbacks, appId)
	return nil
}

// RegDataChangeListener 注册数据变更监听回调（stub实现）
func (d *stubDeviceGroupManager) RegDataChangeListener(appId string, listener *DataChangeListener) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Infof("[DEVICE_AUTH] RegDataChangeListener: appId=%s", appId)
	if d.listeners == nil {
		d.listeners = make(map[string]*DataChangeListener)
	}
	d.listeners[appId] = listener
	return nil
}

// UnRegDataChangeListener 注销数据变更监听回调（stub实现）
func (d *stubDeviceGroupManager) UnRegDataChangeListener(appId string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Infof("[DEVICE_AUTH] UnRegDataChangeListener: appId=%s", appId)
	delete(d.listeners, appId)
	return nil
}

// CreateGroup 创建可信组
func (d *stubDeviceGroupManager) CreateGroup(osAccountId int32, requestId int64, appId string, createParams string) error {
	log.Infof("[DEVICE_AUTH] CreateGroup: osAccountId=%d, requestId=%d, appId=%s", osAccountId, requestId, appId)

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(createParams), &params); err != nil {
		return fmt.Errorf("invalid createParams: %w", err)
	}

	groupId, _ := params["groupId"].(string)
	groupName, _ := params["groupName"].(string)
	groupType, _ := params["groupType"].(float64)
	visibility, _ := params["groupVisibility"].(float64)

	if groupId == "" {
		return fmt.Errorf("groupId is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.groups == nil {
		d.groups = make(map[string]*GroupInfo)
	}

	d.groups[groupId] = &GroupInfo{
		GroupID:    groupId,
		GroupName:  groupName,
		GroupType:  int32(groupType),
		Visibility: int32(visibility),
		CreateTime: requestId,
		Members:    make(map[string]*DeviceMemberInfo),
	}

	log.Infof("[DEVICE_AUTH] Group created: groupId=%s, groupName=%s", groupId, groupName)

	// 触发回调
	if listener, ok := d.listeners[appId]; ok && listener.OnGroupCreated != nil {
		groupInfo := fmt.Sprintf(`{"groupId":"%s","groupName":"%s","groupType":%d}`, groupId, groupName, int32(groupType))
		listener.OnGroupCreated(groupInfo)
	}

	return nil
}

// DeleteGroup 删除可信组
func (d *stubDeviceGroupManager) DeleteGroup(osAccountId int32, requestId int64, appId string, disbandParams string) error {
	log.Infof("[DEVICE_AUTH] DeleteGroup: osAccountId=%d, requestId=%d, appId=%s", osAccountId, requestId, appId)

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(disbandParams), &params); err != nil {
		return fmt.Errorf("invalid disbandParams: %w", err)
	}

	groupId, _ := params["groupId"].(string)
	if groupId == "" {
		return fmt.Errorf("groupId is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	group, exists := d.groups[groupId]
	if !exists {
		return fmt.Errorf("group not found: %s", groupId)
	}

	delete(d.groups, groupId)
	log.Infof("[DEVICE_AUTH] Group deleted: groupId=%s", groupId)

	// 触发回调
	if listener, ok := d.listeners[appId]; ok && listener.OnGroupDeleted != nil {
		groupInfo := fmt.Sprintf(`{"groupId":"%s","groupName":"%s"}`, group.GroupID, group.GroupName)
		listener.OnGroupDeleted(groupInfo)
	}

	return nil
}

// AddMemberToGroup 将可信设备添加到可信组
func (d *stubDeviceGroupManager) AddMemberToGroup(osAccountId int32, requestId int64, appId string, addParams string) error {
	log.Infof("[DEVICE_AUTH] AddMemberToGroup: osAccountId=%d, requestId=%d, appId=%s", osAccountId, requestId, appId)

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(addParams), &params); err != nil {
		return fmt.Errorf("invalid addParams: %w", err)
	}

	groupId, _ := params["groupId"].(string)
	deviceId, _ := params["deviceId"].(string)
	if groupId == "" || deviceId == "" {
		return fmt.Errorf("groupId and deviceId are required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	group, exists := d.groups[groupId]
	if !exists {
		return fmt.Errorf("group not found: %s", groupId)
	}

	if group.Members == nil {
		group.Members = make(map[string]*DeviceMemberInfo)
	}

	member := &DeviceMemberInfo{
		DeviceID: deviceId,
		UDID:     deviceId,
		JoinTime: requestId,
	}
	if udid, ok := params["udid"].(string); ok {
		member.UDID = udid
	}
	if authId, ok := params["authId"].(string); ok {
		member.AuthID = authId
	}

	group.Members[deviceId] = member
	log.Infof("[DEVICE_AUTH] Member added: groupId=%s, deviceId=%s", groupId, deviceId)

	// 触发回调
	if listener, ok := d.listeners[appId]; ok && listener.OnDeviceBound != nil {
		groupInfo := fmt.Sprintf(`{"groupId":"%s","groupName":"%s"}`, group.GroupID, group.GroupName)
		listener.OnDeviceBound(member.UDID, groupInfo)
	}

	return nil
}

// DeleteMemberFromGroup 从可信组删除可信设备
func (d *stubDeviceGroupManager) DeleteMemberFromGroup(osAccountId int32, requestId int64, appId string, deleteParams string) error {
	log.Infof("[DEVICE_AUTH] DeleteMemberFromGroup: osAccountId=%d, requestId=%d, appId=%s", osAccountId, requestId, appId)

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(deleteParams), &params); err != nil {
		return fmt.Errorf("invalid deleteParams: %w", err)
	}

	groupId, _ := params["groupId"].(string)
	deviceId, _ := params["deviceId"].(string)
	if groupId == "" || deviceId == "" {
		return fmt.Errorf("groupId and deviceId are required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	group, exists := d.groups[groupId]
	if !exists {
		return fmt.Errorf("group not found: %s", groupId)
	}

	member, exists := group.Members[deviceId]
	if !exists {
		return fmt.Errorf("device not found in group: %s", deviceId)
	}

	delete(group.Members, deviceId)
	log.Infof("[DEVICE_AUTH] Member deleted: groupId=%s, deviceId=%s", groupId, deviceId)

	// 触发回调
	if listener, ok := d.listeners[appId]; ok && listener.OnDeviceUnBound != nil {
		groupInfo := fmt.Sprintf(`{"groupId":"%s","groupName":"%s"}`, group.GroupID, group.GroupName)
		listener.OnDeviceUnBound(member.UDID, groupInfo)
	}

	return nil
}

// ProcessData 处理绑定或解绑设备的数据（stub实现）
func (d *stubDeviceGroupManager) ProcessData(requestId int64, data []byte) error {
	log.Infof("[DEVICE_AUTH] ProcessData: requestId=%d, dataLen=%d", requestId, len(data))
	return fmt.Errorf("not implemented")
}

// AddMultiMembersToGroup 批量添加具有账户关系的可信设备（stub实现）
func (d *stubDeviceGroupManager) AddMultiMembersToGroup(osAccountId int32, appId string, addParams string) error {
	log.Infof("[DEVICE_AUTH] AddMultiMembersToGroup: osAccountId=%d, appId=%s", osAccountId, appId)
	return fmt.Errorf("not implemented")
}

// DelMultiMembersFromGroup 批量删除具有账户关系的可信设备（stub实现）
func (d *stubDeviceGroupManager) DelMultiMembersFromGroup(osAccountId int32, appId string, deleteParams string) error {
	log.Infof("[DEVICE_AUTH] DelMultiMembersFromGroup: osAccountId=%d, appId=%s", osAccountId, appId)
	return fmt.Errorf("not implemented")
}

// GetRegisterInfo 获取本地设备的注册信息（stub实现）
func (d *stubDeviceGroupManager) GetRegisterInfo(reqJsonStr string) (string, error) {
	log.Infof("[DEVICE_AUTH] GetRegisterInfo called")
	return "{}", nil
}

// CheckAccessToGroup 检查指定应用是否具有组的访问权限（stub实现）
func (d *stubDeviceGroupManager) CheckAccessToGroup(osAccountId int32, appId string, groupId string) error {
	log.Infof("[DEVICE_AUTH] CheckAccessToGroup: osAccountId=%d, appId=%s, groupId=%s", osAccountId, appId, groupId)
	// Stub实现：总是允许访问
	return nil
}

// GetPkInfoList 获取与设备相关的所有公钥信息（stub实现）
func (d *stubDeviceGroupManager) GetPkInfoList(osAccountId int32, appId string, queryParams string) ([]string, error) {
	log.Infof("[DEVICE_AUTH] GetPkInfoList: osAccountId=%d, appId=%s", osAccountId, appId)
	return []string{}, nil
}

// GetGroupInfoById 获取组的组信息
func (d *stubDeviceGroupManager) GetGroupInfoById(osAccountId int32, appId string, groupId string) (string, error) {
	log.Infof("[DEVICE_AUTH] GetGroupInfoById: osAccountId=%d, appId=%s, groupId=%s", osAccountId, appId, groupId)

	d.mu.RLock()
	defer d.mu.RUnlock()

	group, exists := d.groups[groupId]
	if !exists {
		return "", fmt.Errorf("group not found: %s", groupId)
	}

	result := fmt.Sprintf(`{"groupId":"%s","groupName":"%s","groupType":%d,"groupVisibility":%d}`,
		group.GroupID, group.GroupName, group.GroupType, group.Visibility)
	return result, nil
}

// GetGroupInfo 获取满足查询参数的组的组信息（stub实现）
func (d *stubDeviceGroupManager) GetGroupInfo(osAccountId int32, appId string, queryParams string) ([]string, error) {
	log.Infof("[DEVICE_AUTH] GetGroupInfo: osAccountId=%d, appId=%s", osAccountId, appId)
	return []string{}, nil
}

// GetJoinedGroups 获取特定组类型的所有组信息
func (d *stubDeviceGroupManager) GetJoinedGroups(osAccountId int32, appId string, groupType GroupType) ([]string, error) {
	log.Infof("[DEVICE_AUTH] GetJoinedGroups: osAccountId=%d, appId=%s, groupType=%d", osAccountId, appId, groupType)

	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []string
	for _, group := range d.groups {
		if groupType == AllGroup || GroupType(group.GroupType) == groupType {
			groupInfo := fmt.Sprintf(`{"groupId":"%s","groupName":"%s","groupType":%d}`,
				group.GroupID, group.GroupName, group.GroupType)
			result = append(result, groupInfo)
		}
	}

	return result, nil
}

// GetRelatedGroups 获取与某个设备相关的所有组信息
func (d *stubDeviceGroupManager) GetRelatedGroups(osAccountId int32, appId string, peerDeviceId string) ([]string, error) {
	log.Infof("[DEVICE_AUTH] GetRelatedGroups: osAccountId=%d, appId=%s, peerDeviceId=%s", osAccountId, appId, peerDeviceId)

	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []string
	for _, group := range d.groups {
		if _, exists := group.Members[peerDeviceId]; exists {
			groupInfo := fmt.Sprintf(`{"groupId":"%s","groupName":"%s","groupType":%d}`,
				group.GroupID, group.GroupName, group.GroupType)
			result = append(result, groupInfo)
		}
	}

	return result, nil
}

// GetDeviceInfoById 获取可信设备的信息
func (d *stubDeviceGroupManager) GetDeviceInfoById(osAccountId int32, appId string, deviceId string, groupId string) (string, error) {
	log.Infof("[DEVICE_AUTH] GetDeviceInfoById: osAccountId=%d, appId=%s, deviceId=%s, groupId=%s",
		osAccountId, appId, deviceId, groupId)

	d.mu.RLock()
	defer d.mu.RUnlock()

	group, exists := d.groups[groupId]
	if !exists {
		return "", fmt.Errorf("group not found: %s", groupId)
	}

	member, exists := group.Members[deviceId]
	if !exists {
		return "", fmt.Errorf("device not found: %s", deviceId)
	}

	result := fmt.Sprintf(`{"deviceId":"%s","udid":"%s","authId":"%s"}`,
		member.DeviceID, member.UDID, member.AuthID)
	return result, nil
}

// GetTrustedDevices 获取组中的所有可信设备信息
func (d *stubDeviceGroupManager) GetTrustedDevices(osAccountId int32, appId string, groupId string) ([]string, error) {
	log.Infof("[DEVICE_AUTH] GetTrustedDevices: osAccountId=%d, appId=%s, groupId=%s", osAccountId, appId, groupId)

	d.mu.RLock()
	defer d.mu.RUnlock()

	group, exists := d.groups[groupId]
	if !exists {
		return nil, fmt.Errorf("group not found: %s", groupId)
	}

	var result []string
	for _, member := range group.Members {
		deviceInfo := fmt.Sprintf(`{"deviceId":"%s","udid":"%s","authId":"%s"}`,
			member.DeviceID, member.UDID, member.AuthID)
		result = append(result, deviceInfo)
	}

	return result, nil
}

// IsDeviceInGroup 查询组中是否存在指定设备
func (d *stubDeviceGroupManager) IsDeviceInGroup(osAccountId int32, appId string, groupId string, deviceId string) bool {
	log.Infof("[DEVICE_AUTH] IsDeviceInGroup: osAccountId=%d, appId=%s, groupId=%s, deviceId=%s",
		osAccountId, appId, groupId, deviceId)

	d.mu.RLock()
	defer d.mu.RUnlock()

	group, exists := d.groups[groupId]
	if !exists {
		return false
	}

	_, exists = group.Members[deviceId]
	return exists
}

// CancelRequest 取消绑定或解绑过程（stub实现）
func (d *stubDeviceGroupManager) CancelRequest(requestId int64, appId string) {
	log.Infof("[DEVICE_AUTH] CancelRequest: requestId=%d, appId=%s", requestId, appId)
	// Stub实现：什么也不做
}

// DestroyInfo 销毁内部分配的内存返回的信息（stub实现）
func (d *stubDeviceGroupManager) DestroyInfo(returnInfo *string) {
	// Stub实现：Go有垃圾回收，不需要手动释放
	if returnInfo != nil {
		*returnInfo = ""
	}
}

// ============================================================================
// 全局变量和服务管理
// ============================================================================

var (
	gaInstance         GroupAuthManager
	gmInstance         DeviceGroupManager
	serviceInitialized bool
	serviceMu          sync.RWMutex
)

// InitDeviceAuthService 初始化设备认证服务
func InitDeviceAuthService() error {
	serviceMu.Lock()
	defer serviceMu.Unlock()

	if serviceInitialized {
		log.Warn("[DEVICE_AUTH] Device auth service already initialized")
		return nil
	}

	log.Info("[DEVICE_AUTH] Initializing device auth service with HiChain")

	// 创建真实实现（使用hichain）
	gaInstance = &realGroupAuthManager{
		hichainInstances: make(map[int64]*hichain.HiChainHandle),
		callbacks:        make(map[int64]*DeviceAuthCallback),
		authMessages:     make(map[int64]map[string]interface{}),
	}

	gmInstance = &stubDeviceGroupManager{
		callbacks: make(map[string]*DeviceAuthCallback),
		listeners: make(map[string]*DataChangeListener),
		groups:    make(map[string]*GroupInfo),
	}

	serviceInitialized = true
	log.Info("[DEVICE_AUTH] Device auth service initialized successfully")
	return nil
}

// DestroyDeviceAuthService 销毁设备认证服务
func DestroyDeviceAuthService() {
	serviceMu.Lock()
	defer serviceMu.Unlock()

	if !serviceInitialized {
		return
	}

	log.Info("[DEVICE_AUTH] Destroying device auth service")

	// 清理HiChain实例
	if ga, ok := gaInstance.(*realGroupAuthManager); ok {
		ga.mu.Lock()
		for authReqId, handle := range ga.hichainInstances {
			log.Infof("[DEVICE_AUTH] Destroying HiChain instance: authReqId=%d", authReqId)
			hichain.Destroy(&handle)
		}
		ga.hichainInstances = make(map[int64]*hichain.HiChainHandle)
		ga.callbacks = make(map[int64]*DeviceAuthCallback)
		ga.mu.Unlock()
	}

	gaInstance = nil
	gmInstance = nil
	serviceInitialized = false

	log.Info("[DEVICE_AUTH] Device auth service destroyed")
}

// GetGaInstance 获取组认证实例
// 必须先调用InitDeviceAuthService
func GetGaInstance() (GroupAuthManager, error) {
	serviceMu.RLock()
	defer serviceMu.RUnlock()

	if !serviceInitialized {
		return nil, fmt.Errorf("device auth service not initialized")
	}

	if gaInstance == nil {
		return nil, fmt.Errorf("group auth manager not available")
	}

	return gaInstance, nil
}

// GetGmInstance 获取组管理实例
// 必须先调用InitDeviceAuthService
func GetGmInstance() (DeviceGroupManager, error) {
	serviceMu.RLock()
	defer serviceMu.RUnlock()

	if !serviceInitialized {
		return nil, fmt.Errorf("device auth service not initialized")
	}

	if gmInstance == nil {
		return nil, fmt.Errorf("device group manager not available")
	}

	return gmInstance, nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// ProcessCredential 处理凭证数据
func ProcessCredential(operationCode int32, requestParams string) (string, error) {
	log.Infof("[DEVICE_AUTH] ProcessCredential: operationCode=%d", operationCode)
	// Stub实现：返回空结果
	return "{}", nil
}

// StartAuthDevice 开始设备认证
func StartAuthDevice(requestId int64, authParams string, callback *DeviceAuthCallback) error {
	log.Infof("[DEVICE_AUTH] StartAuthDevice: requestId=%d", requestId)

	serviceMu.RLock()
	ga := gaInstance
	serviceMu.RUnlock()

	if ga == nil {
		return fmt.Errorf("group auth manager not initialized")
	}

	// 调用GroupAuthManager的AuthDevice
	return ga.AuthDevice(AnyOsAccount, requestId, authParams, callback)
}

// ProcessAuthDevice 处理认证设备数据
func ProcessAuthDevice(requestId int64, authParams string, callback *DeviceAuthCallback) error {
	log.Infof("[DEVICE_AUTH] ProcessAuthDevice: requestId=%d", requestId)

	serviceMu.RLock()
	ga := gaInstance
	serviceMu.RUnlock()

	if ga == nil {
		return fmt.Errorf("group auth manager not initialized")
	}

	// 解析authParams获取data（简化实现，实际应该从JSON中提取）
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(authParams), &params); err != nil {
		return fmt.Errorf("failed to parse authParams: %w", err)
	}

	// 这里应该从params中获取data字段
	// 暂时返回not implemented
	return fmt.Errorf("not implemented")
}

// CancelAuthRequest 取消认证设备请求
func CancelAuthRequest(requestId int64, authParams string) error {
	log.Infof("[DEVICE_AUTH] CancelAuthRequest: requestId=%d", requestId)

	serviceMu.RLock()
	ga := gaInstance
	serviceMu.RUnlock()

	if ga == nil {
		return fmt.Errorf("group auth manager not initialized")
	}

	ga.CancelRequest(requestId, "")
	return nil
}

// ============================================================================
// 辅助常量
// ============================================================================

const (
	// AUTH_APPID 软总线认证应用ID
	AUTH_APPID = "softbus_auth"
)
