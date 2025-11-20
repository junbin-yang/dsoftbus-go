package bus_center

import (
	"fmt"
	"sync"
	"time"

	"github.com/junbin-yang/dsoftbus-go/pkg/authentication"
)

// LocalDeviceInfo 本地设备信息
type LocalDeviceInfo struct {
	UDID       string
	UUID       string
	DeviceID   string
	DeviceName string
	DeviceType string
	AuthPort   int
}

// BusCenter 总线中心，统一管理组网
type BusCenter struct {
	ledger         *NetLedger
	netBuilder     *NetBuilder
	localDevInfo   *LocalDeviceInfo
	authCallbacks  []AuthCallback
	mu             sync.RWMutex
	started        bool
}

// AuthCallback 认证回调
type AuthCallback struct {
	OnAuthSuccess func(requestId uint32, authId int64, deviceInfo *NodeInfo)
	OnAuthFailed  func(requestId uint32, reason int32)
}

var (
	instance *BusCenter
	once     sync.Once
)

// GetInstance 获取BusCenter单例
func GetInstance() *BusCenter {
	once.Do(func() {
		ledger := NewNetLedger()
		netBuilder := NewNetBuilder(ledger)
		instance = &BusCenter{
			ledger:     ledger,
			netBuilder: netBuilder,
		}
	})
	return instance
}

// Start 启动总线中心
func (bc *BusCenter) Start() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.started {
		return fmt.Errorf("bus center already started")
	}

	bc.started = true
	return nil
}

// Stop 停止总线中心
func (bc *BusCenter) Stop() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if !bc.started {
		return fmt.Errorf("bus center not started")
	}

	bc.started = false
	return nil
}

// RegisterEventCallback 注册事件回调
func (bc *BusCenter) RegisterEventCallback(callback LNNEventCallback) {
	bc.netBuilder.RegisterCallback(callback)
}

// OnDeviceOnline 设备上线
func (bc *BusCenter) OnDeviceOnline(node *NodeInfo) error {
	return bc.netBuilder.NotifyNodeOnline(node)
}

// OnDeviceOffline 设备下线
func (bc *BusCenter) OnDeviceOffline(networkID string) error {
	return bc.netBuilder.NotifyNodeOffline(networkID)
}

// GetNodeInfo 获取节点信息
func (bc *BusCenter) GetNodeInfo(networkID string) *NodeInfo {
	return bc.ledger.GetNode(networkID)
}

// GetAllNodes 获取所有节点
func (bc *BusCenter) GetAllNodes() []*NodeInfo {
	return bc.ledger.GetAllNodes()
}

// GetOnlineNodes 获取在线节点
func (bc *BusCenter) GetOnlineNodes() []*NodeInfo {
	return bc.ledger.GetOnlineNodes()
}

// SetLocalDeviceInfo 设置本地设备信息
func (bc *BusCenter) SetLocalDeviceInfo(info *LocalDeviceInfo) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.localDevInfo = info
}

// GetLocalDeviceInfo 获取本地设备信息
func (bc *BusCenter) GetLocalDeviceInfo() *LocalDeviceInfo {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.localDevInfo
}

// UpdateAuthPort 更新认证端口
func (bc *BusCenter) UpdateAuthPort(port int) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if bc.localDevInfo != nil {
		bc.localDevInfo.AuthPort = port
	}
}

// RegisterAuthCallback 注册认证回调
func (bc *BusCenter) RegisterAuthCallback(callback AuthCallback) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.authCallbacks = append(bc.authCallbacks, callback)
}

// NotifyAuthSuccess 通知认证成功
func (bc *BusCenter) NotifyAuthSuccess(requestId uint32, authId int64, node *NodeInfo) {
	// 设备上线
	bc.OnDeviceOnline(node)

	// 触发认证成功回调
	bc.mu.RLock()
	callbacks := bc.authCallbacks
	bc.mu.RUnlock()

	for _, cb := range callbacks {
		if cb.OnAuthSuccess != nil {
			go cb.OnAuthSuccess(requestId, authId, node)
		}
	}
}

// NotifyAuthFailed 通知认证失败
func (bc *BusCenter) NotifyAuthFailed(requestId uint32, reason int32) {
	bc.mu.RLock()
	callbacks := bc.authCallbacks
	bc.mu.RUnlock()

	for _, cb := range callbacks {
		if cb.OnAuthFailed != nil {
			go cb.OnAuthFailed(requestId, reason)
		}
	}
}

// JoinLNNCallback 加入LNN回调
type JoinLNNCallback func(networkId string, retCode int32)

// LeaveLNNCallback 离开LNN回调
type LeaveLNNCallback func(networkId string, retCode int32)

// JoinLNN 加入局域网络（触发认证流程）
// 对应C代码: int32_t JoinLNN(const char *pkgName, ConnectionAddr *target, OnJoinLNNResult cb)
func (bc *BusCenter) JoinLNN(pkgName string, ip string, port int, callback JoinLNNCallback) error {
	// 注册临时认证回调
	requestId := uint32(time.Now().Unix())

	bc.RegisterAuthCallback(AuthCallback{
		OnAuthSuccess: func(reqId uint32, authId int64, node *NodeInfo) {
			if reqId == requestId && callback != nil {
				callback(node.NetworkID, 0)
			}
		},
		OnAuthFailed: func(reqId uint32, reason int32) {
			if reqId == requestId && callback != nil {
				callback("", reason)
			}
		},
	})

	// 构建连接信息
	connInfo := &authentication.AuthConnInfo{
		Type: authentication.AuthLinkTypeWifi,
		Ip:   ip,
		Port: port,
	}

	// 触发认证流程
	return authentication.AuthOpenConnection(connInfo, requestId)
}

// LeaveLNN 离开局域网络
// 对应C代码: int32_t LeaveLNN(const char *pkgName, const char *networkId, OnLeaveLNNResult cb)
func (bc *BusCenter) LeaveLNN(pkgName string, networkId string, callback LeaveLNNCallback) error {
	// 设备下线
	if err := bc.OnDeviceOffline(networkId); err != nil {
		return err
	}

	// 回调通知
	if callback != nil {
		go callback(networkId, 0)
	}

	return nil
}
