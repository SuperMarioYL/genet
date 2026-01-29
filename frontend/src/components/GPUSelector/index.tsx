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
}

// 根据状态获取槽位样式类
const getSlotClassName = (
  slot: DeviceSlot,
  isSelected: boolean,
  timeSharingEnabled: boolean
): string => {
  const classes = ['gpu-slot'];

  if (isSelected) {
    classes.push('gpu-slot-selected');
  } else if (slot.status === 'free') {
    classes.push('gpu-slot-free');
  } else if (slot.status === 'used') {
    if (timeSharingEnabled) {
      classes.push('gpu-slot-shared'); // 时分复用节点，可选择
    } else {
      classes.push('gpu-slot-occupied'); // 独占节点，不可选择
    }
  }

  return classes.join(' ');
};

// 单个 GPU 槽位组件
const GPUSlot: React.FC<{
  slot: DeviceSlot;
  isSelected: boolean;
  timeSharingEnabled: boolean;
  onClick: (e: React.MouseEvent) => void;
  disabled?: boolean;
}> = ({ slot, isSelected, timeSharingEnabled, onClick, disabled }) => {
  // 判断是否可点击
  const canClick = !disabled && (slot.status === 'free' || (slot.status === 'used' && timeSharingEnabled));

  const tooltipContent = (
    <div className="gpu-slot-tooltip">
      <div className="tooltip-header">
        <Text strong>GPU {slot.index}</Text>
      </div>
      {slot.status === 'used' && slot.pod && (
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
    </div>
  );

  return (
    <Tooltip title={tooltipContent} placement="top">
      <div
        className={getSlotClassName(slot, isSelected, timeSharingEnabled)}
        onClick={canClick ? onClick : undefined}
        style={{ cursor: canClick ? 'pointer' : 'not-allowed' }}
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
}) => {
  const selectedSet = new Set(selectedDevices);
  const [lastClickedIndex, setLastClickedIndex] = useState<number | null>(null);

  // 检查槽位是否可选择
  const isSlotSelectable = (slotIndex: number): boolean => {
    const slot = slots.find(s => s.index === slotIndex);
    if (!slot) return false;
    return slot.status === 'free' || (slot.status === 'used' && timeSharingEnabled);
  };

  const handleSlotClick = (index: number, e: React.MouseEvent) => {
    if (disabled) return;

    const slot = slots.find(s => s.index === index);
    if (!slot) return;

    // 检查是否可选择
    if (slot.status === 'used' && !timeSharingEnabled) {
      return; // 独占节点上已占用的卡不可选择
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
    return slot && slot.status === 'used';
  }).length;

  return (
    <div className="gpu-selector">
      <div className="gpu-selector-header">
        <Text>选择 GPU 卡</Text>
        <Text type="secondary">已选 {selectedDevices.length} 张 · 按住 Shift 可范围选择</Text>
      </div>

      <div className="gpu-selector-grid">
        {slots.map((slot) => (
          <GPUSlot
            key={slot.index}
            slot={slot}
            isSelected={selectedSet.has(slot.index)}
            timeSharingEnabled={timeSharingEnabled}
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
        {timeSharingEnabled && (
          <div className="legend-item">
            <div className="legend-color legend-shared" />
            <span>已使用(可共享)</span>
          </div>
        )}
        {!timeSharingEnabled && (
          <div className="legend-item">
            <div className="legend-color legend-occupied" />
            <span>已使用(不可选)</span>
          </div>
        )}
      </div>

      {sharedCount > 0 && (
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
