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

// 根据状态获取槽位样式类
const getSlotClassName = (
  slot: DeviceSlot,
  isSelected: boolean,
  timeSharingEnabled: boolean,
  isSharing: boolean
): string => {
  const classes = ['gpu-slot'];

  if (isSelected) {
    classes.push('gpu-slot-selected');
  } else if (slot.status === 'free') {
    classes.push('gpu-slot-free');
  } else if (slot.status === 'full') {
    classes.push('gpu-slot-full'); // 共享模式已满，不可选择
  } else if (slot.status === 'used') {
    if (isSharing || timeSharingEnabled) {
      classes.push('gpu-slot-shared'); // 共享模式或时分复用节点，可选择
    } else {
      classes.push('gpu-slot-occupied'); // 独占模式，不可选择
    }
  }

  return classes.join(' ');
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

  const tooltipContent = (
    <div className="gpu-slot-tooltip">
      <div className="tooltip-header">
        <Text strong>GPU {slot.index}</Text>
        {isSharing && maxPodsPerGPU > 0 && (
          <Text type="secondary" style={{ marginLeft: 8 }}>
            ({slot.currentShare}/{maxPodsPerGPU} 已用)
          </Text>
        )}
      </div>
      {/* 共享模式：显示所有共享的 Pod */}
      {isSharing && slot.sharedPods && slot.sharedPods.length > 0 && (
        <>
          <div className="tooltip-divider" />
          <div className="tooltip-row">
            <span className="tooltip-label">共享 Pod:</span>
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
      {slot.status === 'free' && (
        <div className="tooltip-row">
          <span className="tooltip-label">状态:</span>
          <span className="tooltip-value">空闲</span>
        </div>
      )}
      {slot.status === 'full' && (
        <div className="tooltip-row">
          <span className="tooltip-label">状态:</span>
          <span className="tooltip-value" style={{ color: '#ff4d4f' }}>已满 (不可选)</span>
        </div>
      )}
    </div>
  );

  return (
    <Tooltip title={tooltipContent} placement="top">
      <div
        className={getSlotClassName(slot, isSelected, timeSharingEnabled, isSharing)}
        onClick={canClick ? onClick : undefined}
        style={{ cursor: canClick ? 'pointer' : 'not-allowed' }}
      >
        <span className="gpu-slot-index">{slot.index}</span>
        {isSharing && slot.currentShare > 0 && !isSelected && (
          <span className="gpu-slot-share-count">{slot.currentShare}</span>
        )}
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
        <div className="legend-item">
          <div className="legend-color legend-free" />
          <span>空闲</span>
        </div>
        <div className="legend-item">
          <div className="legend-color legend-selected" />
          <span>已选择</span>
        </div>
        {(isSharing || timeSharingEnabled) && (
          <div className="legend-item">
            <div className="legend-color legend-shared" />
            <span>已使用(可共享)</span>
          </div>
        )}
        {isSharing && (
          <div className="legend-item">
            <div className="legend-color legend-full" />
            <span>已满(不可选)</span>
          </div>
        )}
        {!isSharing && !timeSharingEnabled && (
          <div className="legend-item">
            <div className="legend-color legend-occupied" />
            <span>已使用(不可选)</span>
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
