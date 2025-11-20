package bus_center

import (
	"fmt"
	"time"
)

// NetBuilder 网络构建器，处理设备加入/退出
type NetBuilder struct {
	ledger    *NetLedger
	callbacks []LNNEventCallback
}

// NewNetBuilder 创建网络构建器
func NewNetBuilder(ledger *NetLedger) *NetBuilder {
	return &NetBuilder{
		ledger:    ledger,
		callbacks: make([]LNNEventCallback, 0),
	}
}

// RegisterCallback 注册事件回调
func (nb *NetBuilder) RegisterCallback(callback LNNEventCallback) {
	nb.callbacks = append(nb.callbacks, callback)
}

// NotifyNodeOnline 通知节点上线
func (nb *NetBuilder) NotifyNodeOnline(node *NodeInfo) error {
	node.Status = StatusOnline
	node.JoinTime = time.Now()
	node.LastSeen = time.Now()

	nb.ledger.AddNode(node)
	nb.triggerEvent(EventNodeOnline, node)
	return nil
}

// NotifyNodeOffline 通知节点下线
func (nb *NetBuilder) NotifyNodeOffline(networkID string) error {
	node := nb.ledger.GetNode(networkID)
	if node == nil {
		return fmt.Errorf("node not found: %s", networkID)
	}

	node.Status = StatusOffline
	nb.ledger.UpdateNodeStatus(networkID, StatusOffline)
	nb.triggerEvent(EventNodeOffline, node)
	return nil
}

// triggerEvent 触发事件
func (nb *NetBuilder) triggerEvent(event LNNEvent, node *NodeInfo) {
	for _, callback := range nb.callbacks {
		go callback(event, node)
	}
}
