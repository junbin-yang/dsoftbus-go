package context

import (
	"fmt"
	"sync"
)

// AuthSessionContext 认证会话上下文，用于在传输层和认证层之间共享信息
type AuthSessionContext struct {
	ChannelID     int
	PinCode       string
	GroupID       string
	RequestID     int64
	LocalDeviceID string
	PeerDeviceID  string
}

var (
	sessionContexts = make(map[int]*AuthSessionContext)
	contextMu       sync.RWMutex
)

// SetAuthSessionContext 设置会话上下文
func SetAuthSessionContext(channelID int, ctx *AuthSessionContext) {
	contextMu.Lock()
	defer contextMu.Unlock()
	sessionContexts[channelID] = ctx
}

// GetAuthSessionContext 获取会话上下文
func GetAuthSessionContext(channelID int) (*AuthSessionContext, error) {
	contextMu.RLock()
	defer contextMu.RUnlock()
	ctx, ok := sessionContexts[channelID]
	if !ok {
		return nil, fmt.Errorf("session context not found for channel %d", channelID)
	}
	return ctx, nil
}

// DeleteAuthSessionContext 删除会话上下文
func DeleteAuthSessionContext(channelID int) {
	contextMu.Lock()
	defer contextMu.Unlock()
	delete(sessionContexts, channelID)
}

// FindAuthSessionContextByRequestId 根据RequestID查找会话上下文
func FindAuthSessionContextByRequestId(requestID int64) *AuthSessionContext {
	contextMu.RLock()
	defer contextMu.RUnlock()

	for _, ctx := range sessionContexts {
		if ctx.RequestID == requestID {
			return ctx
		}
	}
	return nil
}
