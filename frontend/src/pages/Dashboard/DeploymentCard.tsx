import { DeleteOutlined, DownOutlined, HddOutlined } from '@ant-design/icons';
import { Button, Divider, Modal, Space, Tag, Typography, message } from 'antd';
import dayjs from 'dayjs';
import React, { useMemo, useState } from 'react';
import GlassCard from '../../components/GlassCard';
import StatusBadge from '../../components/StatusBadge';
import { deleteDeployment } from '../../services/api';
import PodCard from './PodCard';
import './StatefulSetCard.css';

const { Text } = Typography;

interface DeploymentCardProps {
  deployment: any;
  onUpdate: () => void;
  cleanupSchedule?: string;
  cleanupTimezone?: string;
}

const DeploymentCard: React.FC<DeploymentCardProps> = ({ deployment, onUpdate, cleanupSchedule, cleanupTimezone }) => {
  const [expanded, setExpanded] = useState(true);
  const [deleting, setDeleting] = useState(false);

  const subtitle = useMemo(() => {
    const parts = [
      deployment.gpuType ? `${deployment.gpuType} ×${deployment.gpuCount}` : (deployment.gpuCount > 0 ? `GPU ×${deployment.gpuCount}` : '无 GPU'),
      `${deployment.cpu || '-'} CPU`,
      deployment.memory || '-',
    ];
    return parts.join(' · ');
  }, [deployment.cpu, deployment.gpuCount, deployment.gpuType, deployment.memory]);

  const handleDelete = () => {
    Modal.confirm({
      title: '确认删除 Deployment？',
      content: (
        <div>
          <p><strong>Deployment:</strong> {deployment.name}</p>
          <p>该操作会删除整个 Deployment 及其副本 Pod。</p>
        </div>
      ),
      okText: '确认删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setDeleting(true);
        try {
          await deleteDeployment(deployment.id);
          message.success('Deployment 删除成功');
          onUpdate();
        } catch (error: any) {
          message.error(`删除失败: ${error.message}`);
        } finally {
          setDeleting(false);
        }
      },
    });
  };

  return (
    <GlassCard className="statefulset-card">
      <div className="statefulset-card-header">
        <div>
          <Space size={8}>
            <HddOutlined />
            <Text strong className="statefulset-card-title">{deployment.name}</Text>
            <StatusBadge status={deployment.status} />
          </Space>
          <div className="statefulset-card-subtitle">{subtitle}</div>
        </div>
        <Space size={8} wrap>
          <Tag color="blue">{deployment.readyReplicas}/{deployment.replicas} Ready</Tag>
          <Text type="secondary">{dayjs(deployment.createdAt).format('MM-DD HH:mm')}</Text>
        </Space>
      </div>

      <div className="statefulset-card-actions">
        <Button size="small" icon={<DownOutlined rotate={expanded ? 180 : 0} />} onClick={() => setExpanded((v) => !v)}>
          {expanded ? '收起副本' : '展开副本'}
        </Button>
        <Button size="small" danger icon={<DeleteOutlined />} onClick={handleDelete} loading={deleting}>
          删除 Deployment
        </Button>
      </div>

      {expanded && (
        <>
          <Divider className="statefulset-card-divider" />
          <div className="statefulset-child-list">
            {(deployment.pods || []).map((pod: any) => (
              <PodCard
                key={pod.id}
                pod={pod}
                onUpdate={onUpdate}
                cleanupSchedule={cleanupSchedule}
                cleanupTimezone={cleanupTimezone}
                allowDelete={false}
              />
            ))}
          </div>
        </>
      )}
    </GlassCard>
  );
};

export default DeploymentCard;
