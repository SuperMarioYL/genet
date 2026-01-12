import React from 'react';
import { Tag } from 'antd';

interface StatusBadgeProps {
  status: string;
  phase?: string;
}

const StatusBadge: React.FC<StatusBadgeProps> = ({ status, phase }) => {
  const getStatusConfig = () => {
    // 优先使用 phase
    const statusKey = (phase || status).toLowerCase();

    switch (statusKey) {
      case 'running':
        return { color: 'green', text: '运行中', icon: '✓' };
      case 'pending':
        return { color: 'gold', text: '启动中', icon: '⋯' };
      case 'failed':
        return { color: 'red', text: '失败', icon: '✗' };
      case 'succeeded':
        return { color: 'blue', text: '已完成', icon: '✓' };
      case 'terminating':
        return { color: 'default', text: '删除中', icon: '⋯' };
      default:
        return { color: 'default', text: status || phase, icon: '' };
    }
  };

  const config = getStatusConfig();

  return (
    <Tag color={config.color}>
      {config.text}
    </Tag>
  );
};

export default StatusBadge;

