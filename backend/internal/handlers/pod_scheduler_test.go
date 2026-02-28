package handlers

import (
	"reflect"
	"testing"
)

func TestSelectNodeAndDevicesForSharing_PicksLowestHeatNode(t *testing.T) {
	nodes := []NodeInfo{
		{
			NodeName: "node-hot",
			Slots: []DeviceSlot{
				{Index: 0, Status: "free", Utilization: 80, MemoryUsed: 8000, MemoryTotal: 10000},
				{Index: 1, Status: "free", Utilization: 70, MemoryUsed: 7000, MemoryTotal: 10000},
			},
		},
		{
			NodeName: "node-cool",
			Slots: []DeviceSlot{
				{Index: 0, Status: "free", Utilization: 15, MemoryUsed: 2000, MemoryTotal: 10000},
				{Index: 1, Status: "used", Utilization: 20, MemoryUsed: 2500, MemoryTotal: 10000},
			},
		},
	}

	nodeName, devices, err := selectNodeAndDevicesForSharing(nodes, 2, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeName != "node-cool" {
		t.Fatalf("expected node-cool, got %s", nodeName)
	}
	wantDevices := []int{0, 1}
	if !reflect.DeepEqual(devices, wantDevices) {
		t.Fatalf("expected devices %v, got %v", wantDevices, devices)
	}
}

func TestSelectNodeAndDevicesForSharing_UsesPreferredNode(t *testing.T) {
	nodes := []NodeInfo{
		{
			NodeName: "node-a",
			Slots: []DeviceSlot{
				{Index: 0, Status: "full", Utilization: 10, MemoryUsed: 1000, MemoryTotal: 10000},
				{Index: 1, Status: "used", Utilization: 35, MemoryUsed: 4500, MemoryTotal: 10000},
				{Index: 2, Status: "free", Utilization: 10, MemoryUsed: 500, MemoryTotal: 10000},
			},
		},
		{
			NodeName: "node-b",
			Slots: []DeviceSlot{
				{Index: 0, Status: "free", Utilization: 5, MemoryUsed: 500, MemoryTotal: 10000},
				{Index: 1, Status: "free", Utilization: 6, MemoryUsed: 600, MemoryTotal: 10000},
			},
		},
	}

	nodeName, devices, err := selectNodeAndDevicesForSharing(nodes, 2, "node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeName != "node-a" {
		t.Fatalf("expected node-a, got %s", nodeName)
	}
	wantDevices := []int{1, 2}
	if !reflect.DeepEqual(devices, wantDevices) {
		t.Fatalf("expected devices %v, got %v", wantDevices, devices)
	}
}

func TestSelectNodeAndDevicesForSharing_FailsWhenInsufficient(t *testing.T) {
	nodes := []NodeInfo{
		{
			NodeName: "node-a",
			Slots: []DeviceSlot{
				{Index: 0, Status: "full", Utilization: 10, MemoryUsed: 1000, MemoryTotal: 10000},
				{Index: 1, Status: "used", Utilization: 20, MemoryUsed: 2000, MemoryTotal: 10000, CurrentShare: 4, MaxShare: 4},
			},
		},
	}

	_, _, err := selectNodeAndDevicesForSharing(nodes, 1, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
