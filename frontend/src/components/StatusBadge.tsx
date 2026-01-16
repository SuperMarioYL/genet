import { Tag } from 'antd';
import React from 'react';

interface StatusBadgeProps {
  status: string;
}

const StatusBadge: React.FC<StatusBadgeProps> = ({ status }) => {
  const getStatusConfig = () => {
    // 直接使用后端返回的 status（类似 kubectl get pod 的 STATUS）
    const statusKey = status?.toLowerCase() || '';

    switch (statusKey) {
      case 'running':
        return { color: 'green', text: '运行中' };
      case 'pending':
        return { color: 'gold', text: '等待中' };
      case 'terminating':
        return { color: 'orange', text: '删除中' };
      case 'containercreating':
        return { color: 'blue', text: '创建中' };
      case 'crashloopbackoff':
        return { color: 'red', text: '崩溃重启' };
      case 'imagepullbackoff':
      case 'errimagepull':
        return { color: 'red', text: '镜像拉取失败' };
      case 'error':
        return { color: 'red', text: '错误' };
      case 'failed':
        return { color: 'red', text: '失败' };
      case 'succeeded':
      case 'completed':
        return { color: 'blue', text: '已完成' };
      case 'oomkilled':
        return { color: 'red', text: '内存溢出' };
      default:
        // 处理 Init: 前缀的状态
        if (statusKey.startsWith('init:')) {
          return { color: 'blue', text: status };
        }
        return { color: 'default', text: status || '未知' };
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

