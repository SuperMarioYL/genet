import { Tooltip, Typography } from 'antd';
import React, { useState } from 'react';
import { DeviceSlot } from '../../services/api';
import './GPUSelector.css';

const { Text } = Typography;

export interface GPUSelectorProps {
  slots: DeviceSlot[];
  selectedDevices: number[];
  timeSharingEnabled: boolean;
  onChange: (devices: number[]) => void;
  disabled?: boolean;
  // 新增：共享模式相关
  schedulingMode?: 'sharing' | 'exclusive';
  maxPodsPerGPU?: number;
}

// 热力图颜色计算函数（与 AcceleratorHeatmap 一致）
const getUtilizationColor = (utilization: number): string => {
  const normalized = Math.sqrt(utilization / 100);
  const hue = 120 - (normalized * 120);
  return `hsl(${Math.max(0, hue)}, 70%, 45%)`;
};

// 格式化显存大小
const formatMemory = (mib: number | undefined): string => {
  if (mib === undefined) return '-';
  if (mib >= 1024) {
    return `${(mib / 1024).toFixed(1)} GiB`;
  }
  return `${mib} MiB`;
};

// 单个 GPU 槽位组件
const GPUSlot: React.FC<{
  slot: DeviceSlot;
  isSelected: boolean;
  timeSharingEnabled: boolean;
  isSharing: boolean;
  maxPodsPerGPU: number;
  onClick: (e: React.MouseEvent) => void;
  disabled?: boolean;
}> = ({ slot, isSelected, timeSharingEnabled, isSharing, maxPodsPerGPU, onClick, disabled }) => {
  // 判断是否可点击
  // 共享模式：free 和 used 可选，full 不可选
  // 独占模式或时分复用：原有逻辑
  const canClick = !disabled && (
    slot.status === 'free' ||
    (slot.status === 'used' && (isSharing || timeSharingEnabled))
  );

  // 计算显存利用率
  const memoryUtilization = (slot.memoryTotal && slot.memoryTotal > 0)
    ? (slot.memoryUsed || 0) / slot.memoryTotal * 100
    : 0;

  // 计算 Pod 占用率
  const podUtilization = (maxPodsPerGPU > 0)
    ? (slot.currentShare / maxPodsPerGPU) * 100
    : 0;

  // 取三者最大值决定颜色
  const smUtilization = slot.utilization || 0;
  const maxUtil = Math.max(smUtilization, memoryUtilization, podUtilization);

  // 决定背景色：选中蓝色 > 已满红色 > 利用率渐变色
  const backgroundColor = isSelected
    ? '#1890ff'
    : slot.status === 'full'
      ? '#ff7875'
      : getUtilizationColor(maxUtil);

  const tooltipContent = (
    <div className="gpu-slot-tooltip">
      <div className="tooltip-header">
        <Text strong>GPU {slot.index}</Text>
        {slot.status === 'full' && (
          <Text type="danger" style={{ marginLeft: 8 }}>(已满)</Text>
        )}
      </div>
      <div className="tooltip-divider" />

      {/* SM 利用率 */}
      <div className="tooltip-row">
        <span className="tooltip-label">SM 利用率:</span>
        <span className="tooltip-value">{smUtilization.toFixed(0)}%</span>
      </div>

      {/* 显存利用率 */}
      {slot.memoryTotal && slot.memoryTotal > 0 && (
        <div className="tooltip-row">
          <span className="tooltip-label">显存:</span>
          <span className="tooltip-value">
            {formatMemory(slot.memoryUsed)} / {formatMemory(slot.memoryTotal)}
            ({memoryUtilization.toFixed(0)}%)
          </span>
        </div>
      )}

      {/* Pod 占用（共享模式） */}
      {isSharing && maxPodsPerGPU > 0 && (
        <div className="tooltip-row">
          <span className="tooltip-label">Pod 占用:</span>
          <span className="tooltip-value">
            {slot.currentShare} / {maxPodsPerGPU} ({podUtilization.toFixed(0)}%)
          </span>
        </div>
      )}

      {/* 共享 Pod 列表 */}
      {slot.sharedPods && slot.sharedPods.length > 0 && (
        <>
          <div className="tooltip-divider" />
          <div className="tooltip-row">
            <span className="tooltip-label">占用 Pod:</span>
          </div>
          {slot.sharedPods.map((pod, idx) => (
            <div key={idx} className="tooltip-row" style={{ paddingLeft: 8 }}>
              <span className="tooltip-value">• {pod.user || pod.name}</span>
            </div>
          ))}
        </>
      )}

      {/* 独占模式或时分复用：显示单个占用者 */}
      {!isSharing && slot.status === 'used' && slot.pod && (
        <>
          <div className="tooltip-divider" />
          <div className="tooltip-row">
            <span className="tooltip-label">占用者:</span>
            <span className="tooltip-value">{slot.pod.user || slot.pod.name}</span>
          </div>
          {timeSharingEnabled && (
            <div className="tooltip-row">
              <span className="tooltip-label">模式:</span>
              <span className="tooltip-value">时分复用</span>
            </div>
          )}
        </>
      )}
    </div>
  );

  return (
    <Tooltip title={tooltipContent} placement="top">
      <div
        className={`gpu-slot ${isSelected ? 'gpu-slot-selected' : ''} ${slot.status === 'full' ? 'gpu-slot-full' : ''}`}
        onClick={canClick ? onClick : undefined}
        style={{
          backgroundColor,
          cursor: canClick ? 'pointer' : 'not-allowed',
        }}
      >
        <span className="gpu-slot-index">{slot.index}</span>
        {isSelected && <span className="gpu-slot-check">✓</span>}
      </div>
    </Tooltip>
  );
};

// 主组件
const GPUSelector: React.FC<GPUSelectorProps> = ({
  slots,
  selectedDevices,
  timeSharingEnabled,
  onChange,
  disabled = false,
  schedulingMode = 'exclusive',
  maxPodsPerGPU = 0,
}) => {
  const selectedSet = new Set(selectedDevices);
  const [lastClickedIndex, setLastClickedIndex] = useState<number | null>(null);
  const isSharing = schedulingMode === 'sharing';

  // 检查槽位是否可选择
  const isSlotSelectable = (slotIndex: number): boolean => {
    const slot = slots.find(s => s.index === slotIndex);
    if (!slot) return false;
    // 共享模式：free 和 used 可选，full 不可选
    if (isSharing) {
      return slot.status === 'free' || slot.status === 'used';
    }
    // 独占模式或时分复用
    return slot.status === 'free' || (slot.status === 'used' && timeSharingEnabled);
  };

  const handleSlotClick = (index: number, e: React.MouseEvent) => {
    if (disabled) return;

    const slot = slots.find(s => s.index === index);
    if (!slot) return;

    // 检查是否可选择
    if (!isSlotSelectable(index)) {
      return;
    }

    // Shift+Click 范围选择
    if (e.shiftKey && lastClickedIndex !== null) {
      const start = Math.min(lastClickedIndex, index);
      const end = Math.max(lastClickedIndex, index);
      const rangeIndices: number[] = [];

      // 收集范围内所有可选择的槽位
      for (let i = start; i <= end; i++) {
        if (isSlotSelectable(i)) {
          rangeIndices.push(i);
        }
      }

      // 合并已选择的
      const newSelection = Array.from(new Set([...selectedDevices, ...rangeIndices]));
      onChange(newSelection.sort((a, b) => a - b));
    } else {
      // 普通点击: 切换选择
      const newSelected = new Set(selectedSet);
      if (newSelected.has(index)) {
        newSelected.delete(index);
      } else {
        newSelected.add(index);
      }
      onChange(Array.from(newSelected).sort((a, b) => a - b));
    }

    setLastClickedIndex(index);
  };

  // 统计选中的卡中有多少是已被占用的
  const sharedCount = selectedDevices.filter(d => {
    const slot = slots.find(s => s.index === d);
    return slot && (slot.status === 'used' || slot.currentShare > 0);
  }).length;

  return (
    <div className="gpu-selector">
      <div className="gpu-selector-header">
        <Text>选择 GPU 卡</Text>
        <Text type="secondary">
          已选 {selectedDevices.length} 张 · 按住 Shift 可范围选择
          {isSharing && maxPodsPerGPU > 0 && ` · 每卡最多 ${maxPodsPerGPU} Pod`}
        </Text>
      </div>

      <div className="gpu-selector-grid">
        {slots.map((slot) => (
          <GPUSlot
            key={slot.index}
            slot={slot}
            isSelected={selectedSet.has(slot.index)}
            timeSharingEnabled={timeSharingEnabled}
            isSharing={isSharing}
            maxPodsPerGPU={maxPodsPerGPU}
            onClick={(e) => handleSlotClick(slot.index, e)}
            disabled={disabled}
          />
        ))}
      </div>

      <div className="gpu-selector-legend">
        {/* 渐变色图例 */}
        <div className="legend-gradient">
          <span className="legend-gradient-label">利用率</span>
          <div className="legend-gradient-bar" />
          <span className="legend-gradient-range">0% → 100%</span>
        </div>
        {/* 特殊状态图例 */}
        <div className="legend-item">
          <div className="legend-color legend-selected" />
          <span>已选择</span>
        </div>
        {isSharing && (
          <div className="legend-item">
            <div className="legend-color legend-full" />
            <span>已满</span>
          </div>
        )}
      </div>

      {sharedCount > 0 && !isSharing && (
        <div className="gpu-selector-warning">
          <Text type="warning">
            注意: 选择了 {sharedCount} 张已被使用的 GPU，将启用时分复用模式
          </Text>
        </div>
      )}
    </div>
  );
};

export default GPUSelector;
