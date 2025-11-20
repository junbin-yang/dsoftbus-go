package bus_center

import (
	"testing"
)

func TestNetLedgerAddRemove(t *testing.T) {
	ledger := NewNetLedger()

	node := &NodeInfo{
		NetworkID:  "test-id",
		DeviceName: "TestDevice",
		Status:     StatusOnline,
	}

	ledger.AddNode(node)

	retrieved := ledger.GetNode("test-id")
	if retrieved == nil {
		t.Fatal("Node not found after adding")
	}

	if retrieved.DeviceName != "TestDevice" {
		t.Errorf("Expected DeviceName 'TestDevice', got '%s'", retrieved.DeviceName)
	}

	ledger.RemoveNode("test-id")
	retrieved = ledger.GetNode("test-id")
	if retrieved != nil {
		t.Error("Node should be removed")
	}
}

func TestNetLedgerGetOnlineNodes(t *testing.T) {
	ledger := NewNetLedger()

	node1 := &NodeInfo{NetworkID: "node1", Status: StatusOnline}
	node2 := &NodeInfo{NetworkID: "node2", Status: StatusOffline}
	node3 := &NodeInfo{NetworkID: "node3", Status: StatusOnline}

	ledger.AddNode(node1)
	ledger.AddNode(node2)
	ledger.AddNode(node3)

	onlineNodes := ledger.GetOnlineNodes()
	if len(onlineNodes) != 2 {
		t.Errorf("Expected 2 online nodes, got %d", len(onlineNodes))
	}
}

func TestNetLedgerUpdateStatus(t *testing.T) {
	ledger := NewNetLedger()

	node := &NodeInfo{
		NetworkID: "test-id",
		Status:    StatusOnline,
	}

	ledger.AddNode(node)
	ledger.UpdateNodeStatus("test-id", StatusOffline)

	retrieved := ledger.GetNode("test-id")
	if retrieved.Status != StatusOffline {
		t.Errorf("Expected status Offline, got %v", retrieved.Status)
	}
}
