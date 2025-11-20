package bus_center

import (
	"sync"
	"time"
)

// NetLedger 网络账本，管理所有节点信息
type NetLedger struct {
	mu    sync.RWMutex
	nodes map[string]*NodeInfo // key: NetworkID
}

// NewNetLedger 创建网络账本
func NewNetLedger() *NetLedger {
	return &NetLedger{
		nodes: make(map[string]*NodeInfo),
	}
}

// AddNode 添加节点
func (nl *NetLedger) AddNode(node *NodeInfo) {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	nl.nodes[node.NetworkID] = node
}

// RemoveNode 移除节点
func (nl *NetLedger) RemoveNode(networkID string) {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	delete(nl.nodes, networkID)
}

// GetNode 获取节点信息
func (nl *NetLedger) GetNode(networkID string) *NodeInfo {
	nl.mu.RLock()
	defer nl.mu.RUnlock()
	return nl.nodes[networkID]
}

// GetAllNodes 获取所有节点
func (nl *NetLedger) GetAllNodes() []*NodeInfo {
	nl.mu.RLock()
	defer nl.mu.RUnlock()
	nodes := make([]*NodeInfo, 0, len(nl.nodes))
	for _, node := range nl.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetOnlineNodes 获取在线节点
func (nl *NetLedger) GetOnlineNodes() []*NodeInfo {
	nl.mu.RLock()
	defer nl.mu.RUnlock()
	nodes := make([]*NodeInfo, 0)
	for _, node := range nl.nodes {
		if node.Status == StatusOnline {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// UpdateNodeStatus 更新节点状态
func (nl *NetLedger) UpdateNodeStatus(networkID string, status NodeStatus) {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	if node, ok := nl.nodes[networkID]; ok {
		node.Status = status
		node.LastSeen = time.Now()
	}
}
