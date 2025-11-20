package bus_center

import "time"

// NodeStatus 节点状态
type NodeStatus int

const (
	StatusOffline NodeStatus = iota
	StatusOnline
)

// NodeInfo 设备节点信息
type NodeInfo struct {
	NetworkID    string
	DeviceID     string
	DeviceName   string
	DeviceType   int
	Status       NodeStatus
	AuthSeq      int64
	DiscoveryType string
	ConnectAddr  string
	JoinTime     time.Time
	LastSeen     time.Time
}

// LNNEvent LNN事件类型
type LNNEvent int

const (
	EventNodeOnline LNNEvent = iota
	EventNodeOffline
	EventNodeInfoChanged
)

// LNNEventCallback LNN事件回调
type LNNEventCallback func(event LNNEvent, nodeInfo *NodeInfo)
