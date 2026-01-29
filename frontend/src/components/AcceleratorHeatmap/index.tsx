import { ReloadOutlined } from '@ant-design/icons';
import { Button, Empty, Spin, Tooltip, Typography } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AcceleratorGroup, DeviceSlot, getGPUOverview, GPUOverviewResponse, NodeGPUInfo } from '../../services/api';
import './AcceleratorHeatmap.css';

const { Text } = Typography;

interface AcceleratorHeatmapProps {
  refreshInterval?: number;
  onError?: (error: Error) => void;
}

// 根据利用率计算颜色
const getUtilizationColor = (utilization: number, status: string): string => {
  if (status === 'free' || utilization === 0) {
    return 'var(--heatmap-free)';
  }
  // 绿(120°) → 黄(60°) → 红(0°) 连续渐变
  const hue = 120 - (utilization * 1.2);
  return `hsl(${Math.max(0, hue)}, 70%, 45%)`;
};

// 格式化运行时长
const formatDuration = (startTime?: string): string => {
  if (!startTime) return '';
  const start = new Date(startTime);
  const now = new Date();
  const diffMs = now.getTime() - start.getTime();
  const hours = Math.floor(diffMs / (1000 * 60 * 60));
  const minutes = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60));
  if (hours > 24) {
    const days = Math.floor(hours / 24);
    return `${days}d ${hours % 24}h`;
  }
  return `${hours}h ${minutes}m`;
};

// 设备单元格组件
const DeviceCell: React.FC<{
  slot: DeviceSlot;
  nodeName: string;
  onPodClick?: (podName: string, namespace: string) => void;
}> = ({ slot, nodeName, onPodClick }) => {
  const color = getUtilizationColor(slot.utilization, slot.status);

  const tooltipContent = (
    <div className="device-tooltip">
      <div className="tooltip-header">
        <Text strong>GPU {slot.index} @ {nodeName}</Text>
      </div>
      <div className="tooltip-divider" />
      <div className="tooltip-row">
        <span className="tooltip-label">利用率:</span>
        <span className="tooltip-value">{slot.utilization.toFixed(0)}%</span>
      </div>
      {slot.pod && (
        <>
          <div className="tooltip-divider" />
          <div className="tooltip-row">
            <span className="tooltip-label">Pod:</span>
            <span className="tooltip-value">{slot.pod.name}</span>
          </div>
          <div className="tooltip-row">
            <span className="tooltip-label">用户:</span>
            <span className="tooltip-value">{slot.pod.user || '-'}</span>
          </div>
          {slot.pod.email && (
            <div className="tooltip-row">
              <span className="tooltip-label">邮箱:</span>
              <span className="tooltip-value">{slot.pod.email}</span>
            </div>
          )}
          {slot.pod.startTime && (
            <div className="tooltip-row">
              <span className="tooltip-label">运行时长:</span>
              <span className="tooltip-value">{formatDuration(slot.pod.startTime)}</span>
            </div>
          )}
        </>
      )}
    </div>
  );

  const handleClick = () => {
    if (slot.pod && onPodClick) {
      onPodClick(slot.pod.name, slot.pod.namespace);
    }
  };

  return (
    <Tooltip title={tooltipContent} placement="top" overlayClassName="heatmap-tooltip">
      <div
        className={`device-cell ${slot.status === 'used' ? 'device-cell-used' : 'device-cell-free'}`}
        style={{ backgroundColor: color }}
        onClick={handleClick}
      />
    </Tooltip>
  );
};

// 节点行组件
const NodeRow: React.FC<{
  node: NodeGPUInfo;
  maxDevices: number;
  onPodClick?: (podName: string, namespace: string) => void;
}> = ({ node, maxDevices, onPodClick }) => {
  return (
    <div className="node-row">
      <div className="node-info">
        <div className="node-name" title={node.nodeName}>{node.nodeName}</div>
        <div className="node-meta">
          {node.deviceType && <span className="device-type">{node.deviceType}</span>}
          <span className="device-count">{node.usedDevices}/{node.totalDevices}</span>
        </div>
      </div>
      <div className="device-slots">
        {node.slots.map((slot) => (
          <DeviceCell
            key={slot.index}
            slot={slot}
            nodeName={node.nodeName}
            onPodClick={onPodClick}
          />
        ))}
        {/* 填充空位以对齐 */}
        {Array.from({ length: maxDevices - node.slots.length }).map((_, i) => (
          <div key={`empty-${i}`} className="device-cell device-cell-empty" />
        ))}
      </div>
    </div>
  );
};

// 加速卡分组组件
const AcceleratorGroupView: React.FC<{
  group: AcceleratorGroup;
  onPodClick?: (podName: string, namespace: string) => void;
}> = ({ group, onPodClick }) => {
  // 找出最大设备数用于对齐
  const maxDevices = Math.max(...group.nodes.map(n => n.totalDevices), 8);

  return (
    <div className="accelerator-group">
      <div className="group-header">
        <span className="group-label">{group.label}</span>
        <span className="group-stats">{group.usedDevices}/{group.totalDevices}</span>
      </div>
      <div className="group-content">
        {/* 列标题 */}
        <div className="column-headers">
          <div className="node-info-placeholder" />
          <div className="column-indices">
            {Array.from({ length: maxDevices }).map((_, i) => (
              <div key={i} className="column-index">{i}</div>
            ))}
          </div>
        </div>
        {/* 节点行 */}
        {group.nodes.map((node) => (
          <NodeRow
            key={node.nodeName}
            node={node}
            maxDevices={maxDevices}
            onPodClick={onPodClick}
          />
        ))}
      </div>
    </div>
  );
};

// 主组件
const AcceleratorHeatmap: React.FC<AcceleratorHeatmapProps> = ({
  refreshInterval = 30000,
  onError,
}) => {
  const navigate = useNavigate();
  const [data, setData] = useState<GPUOverviewResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null);

  const loadData = useCallback(async () => {
    try {
      const response = await getGPUOverview();
      setData(response);
      setLastUpdate(new Date());
    } catch (error: any) {
      onError?.(error);
    } finally {
      setLoading(false);
    }
  }, [onError]);

  useEffect(() => {
    loadData();
    const timer = setInterval(loadData, refreshInterval);
    return () => clearInterval(timer);
  }, [loadData, refreshInterval]);

  const handlePodClick = (podName: string, namespace: string) => {
    // 导航到 Pod 详情页
    navigate(`/pod/${namespace}/${podName}`);
  };

  const handleRefresh = () => {
    setLoading(true);
    loadData();
  };

  if (loading && !data) {
    return (
      <div className="heatmap-loading">
        <Spin size="large" />
        <Text type="secondary">加载中...</Text>
      </div>
    );
  }

  if (!data || data.acceleratorGroups.length === 0) {
    return (
      <div className="heatmap-empty">
        <Empty
          description="暂无加速卡数据"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
        />
      </div>
    );
  }

  return (
    <div className="accelerator-heatmap">
      <div className="heatmap-header">
        <div className="heatmap-title">
          <span className="title-icon">🖥️</span>
          <span>加速卡概览</span>
        </div>
        <div className="heatmap-summary">
          <span className="summary-stat">
            <span className="summary-value">{data.summary.totalDevices}</span>
            <span className="summary-label">总计</span>
          </span>
          <span className="summary-divider">|</span>
          <span className="summary-stat">
            <span className="summary-value">{data.summary.usedDevices}</span>
            <span className="summary-label">使用中</span>
          </span>
          <Button
            type="text"
            icon={<ReloadOutlined spin={loading} />}
            onClick={handleRefresh}
            className="refresh-btn"
          />
        </div>
      </div>

      <div className="heatmap-body">
        {data.acceleratorGroups.map((group) => (
          <AcceleratorGroupView
            key={group.type}
            group={group}
            onPodClick={handlePodClick}
          />
        ))}
      </div>

      <div className="heatmap-footer">
        <div className="heatmap-legend">
          <div className="legend-item">
            <div className="legend-color" style={{ backgroundColor: 'var(--heatmap-free)' }} />
            <span>空闲</span>
          </div>
          <div className="legend-item">
            <div className="legend-color" style={{ backgroundColor: 'hsl(90, 70%, 45%)' }} />
            <span>&lt;25%</span>
          </div>
          <div className="legend-item">
            <div className="legend-color" style={{ backgroundColor: 'hsl(60, 70%, 45%)' }} />
            <span>50%</span>
          </div>
          <div className="legend-item">
            <div className="legend-color" style={{ backgroundColor: 'hsl(30, 70%, 45%)' }} />
            <span>75%</span>
          </div>
          <div className="legend-item">
            <div className="legend-color" style={{ backgroundColor: 'hsl(0, 70%, 45%)' }} />
            <span>100%</span>
          </div>
        </div>
        {lastUpdate && (
          <Text type="secondary" className="last-update">
            更新于 {lastUpdate.toLocaleTimeString()}
          </Text>
        )}
      </div>
    </div>
  );
};

export default AcceleratorHeatmap;
