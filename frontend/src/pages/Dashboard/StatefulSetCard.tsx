import { DeleteOutlined, DownOutlined, HddOutlined } from '@ant-design/icons';
import { Button, Divider, Modal, Space, Tag, Typography, message } from 'antd';
import dayjs from 'dayjs';
import React, { useMemo, useState } from 'react';
import GlassCard from '../../components/GlassCard';
import StatusBadge from '../../components/StatusBadge';
import { deleteStatefulSet, resumeStatefulSet } from '../../services/api';
import PodCard from './PodCard';
import './StatefulSetCard.css';

const { Text } = Typography;

interface StatefulSetCardProps {
  statefulSet: any;
  onUpdate: () => void;
  cleanupSchedule?: string;
  cleanupTimezone?: string;
}

const StatefulSetCard: React.FC<StatefulSetCardProps> = ({ statefulSet, onUpdate, cleanupSchedule, cleanupTimezone }) => {
  const [expanded, setExpanded] = useState(true);
  const [deleting, setDeleting] = useState(false);
  const [resuming, setResuming] = useState(false);
  const isSuspended = statefulSet.suspended || statefulSet.status === 'Suspended';
  const isManaged = statefulSet.managed !== false;

  const subtitle = useMemo(() => {
    const parts = [
      statefulSet.gpuType ? `${statefulSet.gpuType} ×${statefulSet.gpuCount}` : (statefulSet.gpuCount > 0 ? `GPU ×${statefulSet.gpuCount}` : '无 GPU'),
      `${statefulSet.cpu || '-'} CPU`,
      statefulSet.memory || '-',
    ];
    return parts.join(' · ');
  }, [statefulSet.cpu, statefulSet.gpuCount, statefulSet.gpuType, statefulSet.memory]);

  const handleDelete = () => {
    Modal.confirm({
      title: '确认删除 StatefulSet？',
      content: (
        <div>
          <p><strong>StatefulSet:</strong> {statefulSet.name}</p>
          <p>该操作会删除整个 StatefulSet 及其副本 Pod。</p>
        </div>
      ),
      okText: '确认删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setDeleting(true);
        try {
          await deleteStatefulSet(statefulSet.id);
          message.success('StatefulSet 删除成功');
          onUpdate();
        } catch (error: any) {
          message.error(`删除失败: ${error.message}`);
        } finally {
          setDeleting(false);
        }
      },
    });
  };

  const handleResume = async () => {
    setResuming(true);
    try {
      await resumeStatefulSet(statefulSet.id);
      message.success('StatefulSet 已恢复');
      onUpdate();
    } catch (error: any) {
      message.error(`恢复失败: ${error.message}`);
    } finally {
      setResuming(false);
    }
  };

  return (
    <GlassCard className="statefulset-card">
      <div className="statefulset-card-header">
        <div>
          <Space size={8}>
            <HddOutlined />
            <Text strong className="statefulset-card-title">{statefulSet.name}</Text>
            <StatusBadge status={statefulSet.status} />
          </Space>
          <div className="statefulset-card-subtitle">{subtitle}</div>
        </div>
        <Space size={8} wrap>
          {!isManaged && <Tag>外部</Tag>}
          {isSuspended ? <Tag color="gold">挂起</Tag> : <Tag color="blue">{statefulSet.readyReplicas}/{statefulSet.replicas} Ready</Tag>}
          <Tag>Service: {statefulSet.serviceName || '-'}</Tag>
          <Text type="secondary">{dayjs(statefulSet.createdAt).format('MM-DD HH:mm')}</Text>
        </Space>
      </div>

      <div className="statefulset-card-actions">
        <Button size="small" icon={<DownOutlined rotate={expanded ? 180 : 0} />} onClick={() => setExpanded((v) => !v)}>
          {expanded ? '收起副本' : '展开副本'}
        </Button>
        {isManaged && isSuspended && (
          <Button size="small" type="primary" onClick={handleResume} loading={resuming}>
            恢复
          </Button>
        )}
        {isManaged ? (
          <Button size="small" danger icon={<DeleteOutlined />} onClick={handleDelete} loading={deleting}>
            删除 StatefulSet
          </Button>
        ) : (
          <Text type="secondary">只读展示</Text>
        )}
      </div>

      {expanded && (
        <>
          <Divider className="statefulset-card-divider" />
          <div className="statefulset-child-list">
            {isSuspended ? (
              <Text type="secondary">已挂起，无运行副本</Text>
            ) : (
              (statefulSet.pods || []).map((pod: any) => (
                <PodCard
                  key={pod.id}
                  pod={pod}
                  onUpdate={onUpdate}
                  cleanupSchedule={cleanupSchedule}
                  cleanupTimezone={cleanupTimezone}
                  allowDelete={false}
                />
              ))
            )}
          </div>
        </>
      )}
    </GlassCard>
  );
};

export default StatefulSetCard;
