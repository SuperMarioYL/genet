import React from 'react';
import './StatusBadge.css';

interface StatusBadgeProps {
  status: string;
  size?: 'small' | 'default';
}

const StatusBadge: React.FC<StatusBadgeProps> = ({ status, size = 'default' }) => {
  const getStatusConfig = () => {
    const statusKey = status?.toLowerCase() || '';

    switch (statusKey) {
      case 'running':
        return { color: 'success', text: '运行中', pulse: true };
      case 'pending':
        return { color: 'warning', text: '等待中', pulse: true };
      case 'terminating':
        return { color: 'warning', text: '删除中', pulse: true };
      case 'containercreating':
        return { color: 'info', text: '创建中', pulse: true };
      case 'crashloopbackoff':
        return { color: 'error', text: '崩溃重启', pulse: false };
      case 'imagepullbackoff':
      case 'errimagepull':
        return { color: 'error', text: '镜像拉取失败', pulse: false };
      case 'error':
        return { color: 'error', text: '错误', pulse: false };
      case 'failed':
        return { color: 'error', text: '失败', pulse: false };
      case 'succeeded':
      case 'completed':
        return { color: 'info', text: '已完成', pulse: false };
      case 'oomkilled':
        return { color: 'error', text: '内存溢出', pulse: false };
      default:
        if (statusKey.startsWith('init:')) {
          return { color: 'info', text: status, pulse: true };
        }
        return { color: 'default', text: status || '未知', pulse: false };
    }
  };

  const config = getStatusConfig();

  return (
    <span className={`status-badge status-${config.color} ${size === 'small' ? 'status-small' : ''}`}>
      <span className={`status-dot ${config.pulse ? 'pulse' : ''}`} />
      <span className="status-text">{config.text}</span>
    </span>
  );
};

export default StatusBadge;
