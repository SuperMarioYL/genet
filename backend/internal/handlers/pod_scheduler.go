package handlers

import (
	"fmt"
	"math"
	"sort"
)

type slotCandidate struct {
	index int
	heat  float64
}

type nodeCandidate struct {
	nodeName string
	score    float64
	devices  []int
}

// selectNodeAndDevicesForSharing 根据热度（SM 利用率/显存利用率）自动选择节点和设备。
// 规则：
// 1. 仅选择可用卡（free/used，排除 full 或达到共享上限的卡）
// 2. 在每个节点内优先选择热度最低的 requested 张卡
// 3. 在节点间选择平均热度最低的节点
func selectNodeAndDevicesForSharing(nodes []NodeInfo, requested int, preferredNode string) (string, []int, error) {
	if requested <= 0 {
		return "", nil, fmt.Errorf("请求的卡数量必须大于 0")
	}

	var best *nodeCandidate
	for _, node := range nodes {
		if preferredNode != "" && node.NodeName != preferredNode {
			continue
		}

		available := make([]slotCandidate, 0, len(node.Slots))
		for _, slot := range node.Slots {
			if !isSlotAvailableForSharing(slot) {
				continue
			}
			available = append(available, slotCandidate{
				index: slot.Index,
				heat:  slotHeat(slot),
			})
		}
		if len(available) < requested {
			continue
		}

		sort.Slice(available, func(i, j int) bool {
			if available[i].heat == available[j].heat {
				return available[i].index < available[j].index
			}
			return available[i].heat < available[j].heat
		})

		chosen := available[:requested]
		totalHeat := 0.0
		devices := make([]int, 0, requested)
		for _, c := range chosen {
			totalHeat += c.heat
			devices = append(devices, c.index)
		}
		sort.Ints(devices)

		candidate := nodeCandidate{
			nodeName: node.NodeName,
			score:    totalHeat / float64(requested),
			devices:  devices,
		}

		if best == nil ||
			candidate.score < best.score ||
			(candidate.score == best.score && candidate.nodeName < best.nodeName) {
			tmp := candidate
			best = &tmp
		}
	}

	if best == nil {
		if preferredNode != "" {
			return "", nil, fmt.Errorf("节点 %s 可用卡不足，无法满足 %d 张卡请求", preferredNode, requested)
		}
		return "", nil, fmt.Errorf("当前无可用节点可满足 %d 张卡请求", requested)
	}

	return best.nodeName, best.devices, nil
}

func isSlotAvailableForSharing(slot DeviceSlot) bool {
	if slot.Status == "full" {
		return false
	}
	if slot.MaxShare > 0 && slot.CurrentShare >= slot.MaxShare {
		return false
	}
	return slot.Status == "free" || slot.Status == "used"
}

func slotHeat(slot DeviceSlot) float64 {
	sm := clampPercent(slot.Utilization)
	mem := 0.0
	if slot.MemoryTotal > 0 {
		mem = clampPercent(slot.MemoryUsed / slot.MemoryTotal * 100)
	}
	if mem > sm {
		return mem
	}
	return sm
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
