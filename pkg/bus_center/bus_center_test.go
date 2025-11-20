package bus_center

import (
	"testing"
	"time"
)

func TestBusCenterSingleton(t *testing.T) {
	bc1 := GetInstance()
	bc2 := GetInstance()

	if bc1 != bc2 {
		t.Error("BusCenter should be singleton")
	}
}

func TestBusCenterStartStop(t *testing.T) {
	bc := GetInstance()

	if err := bc.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := bc.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestNodeOnlineOffline(t *testing.T) {
	bc := GetInstance()
	bc.Start()
	defer bc.Stop()

	node := &NodeInfo{
		NetworkID:     "test-network-id",
		DeviceID:      "test-device-id",
		DeviceName:    "TestDevice",
		Status:        StatusOffline,
		DiscoveryType: "CoAP",
	}

	if err := bc.OnDeviceOnline(node); err != nil {
		t.Fatalf("OnDeviceOnline failed: %v", err)
	}

	retrievedNode := bc.GetNodeInfo("test-network-id")
	if retrievedNode == nil {
		t.Fatal("Node not found after online")
	}

	if retrievedNode.Status != StatusOnline {
		t.Errorf("Expected status Online, got %v", retrievedNode.Status)
	}

	if err := bc.OnDeviceOffline("test-network-id"); err != nil {
		t.Fatalf("OnDeviceOffline failed: %v", err)
	}

	retrievedNode = bc.GetNodeInfo("test-network-id")
	if retrievedNode.Status != StatusOffline {
		t.Errorf("Expected status Offline, got %v", retrievedNode.Status)
	}
}

func TestGetAllNodes(t *testing.T) {
	bc := GetInstance()
	bc.Start()
	defer bc.Stop()

	node1 := &NodeInfo{NetworkID: "node1", DeviceName: "Device1"}
	node2 := &NodeInfo{NetworkID: "node2", DeviceName: "Device2"}

	bc.OnDeviceOnline(node1)
	bc.OnDeviceOnline(node2)

	nodes := bc.GetAllNodes()
	if len(nodes) < 2 {
		t.Errorf("Expected at least 2 nodes, got %d", len(nodes))
	}
}

func TestEventCallback(t *testing.T) {
	bc := GetInstance()
	bc.Start()
	defer bc.Stop()

	eventReceived := false
	bc.RegisterEventCallback(func(event LNNEvent, node *NodeInfo) {
		if event == EventNodeOnline && node.NetworkID == "test-event-node" {
			eventReceived = true
		}
	})

	node := &NodeInfo{
		NetworkID:  "test-event-node",
		DeviceName: "EventTestDevice",
	}

	bc.OnDeviceOnline(node)
	time.Sleep(100 * time.Millisecond)

	if !eventReceived {
		t.Error("Event callback was not triggered")
	}
}
