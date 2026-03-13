import { ReloadOutlined } from '@ant-design/icons';
import { Button, Empty, Spin, Tabs, Tooltip, Typography } from 'antd';
import React, { memo, useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AcceleratorGroup, DeviceSlot, getGPUOverview, GPUOverviewResponse, NodeGPUInfo, PodInfo } from '../../services/api';
import './AcceleratorHeatmap.css';

const { Text } = Typography;
type MetricsStatus = DeviceSlot['metricsStatus'];

const getPoolLabel = (poolType?: string): string => {
  return poolType === 'exclusive' ? '独占池' : '共享池';
};

const getDisplaySignature = (response: GPUOverviewResponse): string => JSON.stringify({
  summary: response.summary,
  acceleratorGroups: response.acceleratorGroups,
});

interface AcceleratorHeatmapProps {
  refreshInterval?: number;
  onError?: (error: Error) => void;
  onSummaryChange?: (summary: GPUOverviewResponse['summary']) => void;
}

// 根据利用率计算颜色
// 绿色区间小，低利用率时变化明显
// 0% = 绿(120°), 10% = 黄绿(90°), 30% = 黄(60°), 60% = 橙(30°), 100% = 红(0°)
const getUtilizationColor = (utilization: number): string => {
  // 使用平方根函数使低利用率时变化更明显
  // sqrt(utilization/100) * 120 使得：
  // 0% -> 120°(绿), 10% -> 82°(黄绿), 25% -> 60°(黄), 50% -> 35°(橙), 100% -> 0°(红)
  const normalized = Math.sqrt(utilization / 100);
  const hue = 120 - (normalized * 120);
  return `hsl(${Math.max(0, hue)}, 70%, 45%)`;
};

const getMetricsStatusLabel = (metricsStatus: MetricsStatus | undefined): string => {
  switch (metricsStatus) {
    case 'stale':
      return '长时间未更新';
    case 'missing':
      return '未采集';
    case 'fresh':
    default:
      return '正常';
  }
};

const formatMetricsUpdateTime = (metricsUpdatedAt?: string): string => {
  if (!metricsUpdatedAt) return '-';
  return new Date(metricsUpdatedAt).toLocaleString();
};

// 格式化显存 (MiB -> GB/MB)
const formatMemory = (mib?: number): string => {
  if (!mib || mib === 0) return '0';
  if (mib >= 1024) {
    return `${(mib / 1024).toFixed(1)}GB`;
  }
  return `${mib.toFixed(0)}MB`;
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
}> = memo(({ slot, nodeName, onPodClick }) => {
  const metricsStatus = slot.metricsStatus ?? 'fresh';
  const metricsUnavailable = metricsStatus === 'stale' || metricsStatus === 'missing';
  // 计算显存利用率
  const memoryUtilization = (slot.memoryTotal && slot.memoryTotal > 0)
    ? (slot.memoryUsed || 0) / slot.memoryTotal * 100
    : 0;
  const podDetails = slot.sharedPods && slot.sharedPods.length > 0
    ? slot.sharedPods
    : (slot.pod ? [slot.pod] : []);
  const hasMultiplePods = podDetails.length > 1;
  const primaryPod = podDetails[0];

  // 取最大值决定颜色（利用率和显存利用率取大）
  const maxUtil = Math.max(slot.utilization, memoryUtilization);
  const color = metricsUnavailable ? undefined : getUtilizationColor(maxUtil);

  const renderPodDetails = (pod: PodInfo) => (
    <div className="device-tooltip-pod-detail">
      <div className="tooltip-row">
        <span className="tooltip-label">Pod:</span>
        <span className="tooltip-value">{pod.name}</span>
      </div>
      <div className="tooltip-row">
        <span className="tooltip-label">User:</span>
        <span className="tooltip-value">{pod.user || '-'}</span>
      </div>
      {pod.email && (
        <div className="tooltip-row">
          <span className="tooltip-label">Email:</span>
          <span className="tooltip-value">{pod.email}</span>
        </div>
      )}
      {pod.startTime && (
        <div className="tooltip-row">
          <span className="tooltip-label">Duration:</span>
          <span className="tooltip-value">{formatDuration(pod.startTime)}</span>
        </div>
      )}
    </div>
  );

  const tooltipContent = (
    <div className="device-tooltip">
      <div className="tooltip-header">
        <Text strong>GPU {slot.index} @ {nodeName}</Text>
      </div>
      <div className="tooltip-divider" />
      <div className="tooltip-row">
        <span className="tooltip-label">Utilization:</span>
        <span className="tooltip-value">{slot.utilization.toFixed(0)}%</span>
      </div>
      <div className="tooltip-row">
        <span className="tooltip-label">指标状态:</span>
        <span className="tooltip-value">{getMetricsStatusLabel(metricsStatus)}</span>
      </div>
      {metricsUnavailable && (
        <div className="tooltip-row">
          <span className="tooltip-label">最后采集时间:</span>
          <span className="tooltip-value">{formatMetricsUpdateTime(slot.metricsUpdatedAt)}</span>
        </div>
      )}
      {slot.memoryTotal && slot.memoryTotal > 0 && (
        <div className="tooltip-row">
          <span className="tooltip-label">Memory:</span>
          <span className="tooltip-value">
            {formatMemory(slot.memoryUsed)} / {formatMemory(slot.memoryTotal)}
            {' '}({memoryUtilization.toFixed(0)}%)
          </span>
        </div>
      )}
      {primaryPod && (
        <>
          <div className="tooltip-divider" />
          {hasMultiplePods ? (
            <Tabs
              defaultActiveKey="pod-0"
              size="small"
              className="device-tooltip-tabs"
              items={podDetails.map((pod, index) => ({
                key: `pod-${index}`,
                label: <span className="device-tooltip-tab-label">{pod.name}</span>,
                children: renderPodDetails(pod),
              }))}
            />
          ) : (
            renderPodDetails(primaryPod)
          )}
        </>
      )}
    </div>
  );

  const handleClick = () => {
    if (!hasMultiplePods && primaryPod && onPodClick) {
      onPodClick(primaryPod.name, primaryPod.namespace);
    }
  };

  return (
    <Tooltip title={tooltipContent} placement="top" classNames={{ root: 'heatmap-tooltip' }}>
      <div
        className={[
          'device-cell',
          slot.status === 'free' ? 'device-cell-free' : 'device-cell-used',
          metricsUnavailable ? 'device-cell-metrics-unavailable' : '',
        ].filter(Boolean).join(' ')}
        style={{ backgroundColor: color }}
        onClick={handleClick}
      />
    </Tooltip>
  );
});
DeviceCell.displayName = 'DeviceCell';

// 节点行组件
const NodeRow: React.FC<{
  node: NodeGPUInfo;
  maxDevices: number;
  onPodClick?: (podName: string, namespace: string) => void;
}> = memo(({ node, maxDevices, onPodClick }) => {
  const poolType = node.poolType === 'exclusive' ? 'exclusive' : 'shared';

  return (
    <div className="node-row">
      <div className="node-info-line">
        <span className="node-name" title={node.nodeName}>{node.nodeName}</span>
        <span className="node-separator">|</span>
        <span className="node-ip">{node.nodeIP || '-'}</span>
        <span className="node-separator">|</span>
        <span className="device-type">{node.deviceType || '-'}</span>
        <span className={`pool-badge pool-badge-${poolType}`}>{getPoolLabel(poolType)}</span>
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
});
NodeRow.displayName = 'NodeRow';

// 加速卡分组组件
const AcceleratorGroupView: React.FC<{
  group: AcceleratorGroup;
  onPodClick?: (podName: string, namespace: string) => void;
}> = memo(({ group, onPodClick }) => {
  // 找出最大设备数用于对齐
  const maxDevices = Math.max(...group.nodes.map(n => n.totalDevices), 8);
  const sharedNodes = group.nodes.filter((node) => node.poolType !== 'exclusive');
  const exclusiveNodes = group.nodes.filter((node) => node.poolType === 'exclusive');

  const renderPoolSection = (title: string, nodes: NodeGPUInfo[], poolType: 'shared' | 'exclusive') => {
    if (nodes.length === 0) return null;
    const total = nodes.reduce((sum, node) => sum + node.totalDevices, 0);
    const used = nodes.reduce((sum, node) => sum + node.usedDevices, 0);

    return (
      <div className="pool-section">
        <div className="pool-section-header">
          <span className={`pool-badge pool-badge-${poolType}`}>{title}</span>
          <span className="pool-section-stats">{used}/{total}</span>
        </div>
        {nodes.map((node) => (
          <NodeRow
            key={node.nodeName}
            node={node}
            maxDevices={maxDevices}
            onPodClick={onPodClick}
          />
        ))}
      </div>
    );
  };

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
        {renderPoolSection('共享池', sharedNodes, 'shared')}
        {renderPoolSection('独占池', exclusiveNodes, 'exclusive')}
      </div>
    </div>
  );
});
AcceleratorGroupView.displayName = 'AcceleratorGroupView';

// 主组件
const AcceleratorHeatmap: React.FC<AcceleratorHeatmapProps> = ({
  refreshInterval = 30000,
  onError,
  onSummaryChange,
}) => {
  const navigate = useNavigate();
  const [data, setData] = useState<GPUOverviewResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null);
  const displaySignatureRef = useRef<string | null>(null);
  const onErrorRef = useRef(onError);
  const onSummaryChangeRef = useRef(onSummaryChange);

  useEffect(() => {
    onErrorRef.current = onError;
  }, [onError]);

  useEffect(() => {
    onSummaryChangeRef.current = onSummaryChange;
  }, [onSummaryChange]);

  const loadData = useCallback(async () => {
    try {
      const response = await getGPUOverview();
      const nextSignature = getDisplaySignature(response);

      if (displaySignatureRef.current !== nextSignature) {
        displaySignatureRef.current = nextSignature;
        setData(response);
        onSummaryChangeRef.current?.(response.summary);
      }

      setLastUpdate(new Date());
    } catch (error: any) {
      onErrorRef.current?.(error);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
    const timer = setInterval(loadData, refreshInterval);
    return () => clearInterval(timer);
  }, [loadData, refreshInterval]);

  const handlePodClick = useCallback((podName: string, namespace: string) => {
    // 导航到 Pod 详情页
    navigate(`/pod/${namespace}/${podName}`);
  }, [navigate]);

  const handleRefresh = () => {
    setLoading(true);
    loadData();
  };

  if (loading && !data) {
    return (
      <div className="heatmap-loading">
        <Spin size="large" />
        <Text type="secondary">Loading...</Text>
      </div>
    );
  }

  if (!data || data.acceleratorGroups.length === 0) {
    return (
      <div className="heatmap-empty">
        <Empty
          description="No accelerator data"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
        />
      </div>
    );
  }

  return (
    <div className="accelerator-heatmap">
      <div className="heatmap-header">
        <div className="heatmap-header-summary">
          <span className="heatmap-summary-stat">
            <span className="heatmap-summary-label">总卡数量</span>
            <span className="heatmap-summary-value">{data.summary.totalDevices}</span>
          </span>
          <span className="heatmap-summary-stat">
            <span className="heatmap-summary-label">占用量</span>
            <span className="heatmap-summary-value">{data.summary.usedDevices}/{data.summary.totalDevices}</span>
          </span>
        </div>
        <Button
          type="text"
          icon={<ReloadOutlined spin={loading} />}
          onClick={handleRefresh}
          className="refresh-btn"
        />
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
            <span className="legend-color legend-color-stale" />
            <span>指标缺失/长时间未更新</span>
          </div>
          <div className="legend-gradient-section">
            <span className="legend-gradient-label">0%</span>
            <div className="legend-gradient-bar" />
            <span className="legend-gradient-label">100%</span>
          </div>
        </div>
        {lastUpdate && (
          <Text type="secondary" className="last-update">
            Updated {lastUpdate.toLocaleTimeString()}
          </Text>
        )}
      </div>
    </div>
  );
};

export default AcceleratorHeatmap;
