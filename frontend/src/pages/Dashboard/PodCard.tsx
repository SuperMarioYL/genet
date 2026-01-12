import React, { useState } from 'react';
import { Card, Space, Button, Modal, message, Divider, Tooltip } from 'antd';
import { ClockCircleOutlined, DeleteOutlined, EyeOutlined, CopyOutlined, CodeOutlined, WindowsOutlined, DesktopOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import StatusBadge from '../../components/StatusBadge';
import CountdownTimer from '../../components/CountdownTimer';
import { deletePod, extendPod } from '../../services/api';
import './PodCard.css';

interface PodCardProps {
  pod: any;
  onUpdate: () => void;
}

const PodCard: React.FC<PodCardProps> = ({ pod, onUpdate }) => {
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState(false);

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    message.success(`${label} 已复制`);
  };

  const handleExtend = () => {
    Modal.confirm({
      title: '延长 Pod 生命周期',
      content: (
        <div>
          <p>选择延长时间：</p>
          <Space direction="vertical" style={{ width: '100%' }}>
            <Button block onClick={() => doExtend(2)}>延长 2 小时</Button>
            <Button block onClick={() => doExtend(4)}>延长 4 小时</Button>
            <Button block onClick={() => doExtend(8)}>延长 8 小时</Button>
          </Space>
        </div>
      ),
      okButtonProps: { style: { display: 'none' } },
      cancelText: '取消',
    });
  };

  const doExtend = async (hours: number) => {
    try {
      await extendPod(pod.id, hours);
      message.success(`已延长 ${hours} 小时`);
      onUpdate();
      Modal.destroyAll();
    } catch (error: any) {
      message.error(`延长失败: ${error.message}`);
    }
  };

  const handleDelete = () => {
    Modal.confirm({
      title: '确认删除 Pod？',
      content: (
        <div>
          <p><strong>Pod 名称:</strong> {pod.name}</p>
          <p style={{ color: '#ff4d4f' }}>
            此操作无法撤销，Pod 将被删除（工作区存储保留）
          </p>
        </div>
      ),
      okText: '确认删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setDeleting(true);
        try {
          await deletePod(pod.id);
          message.success('Pod 删除成功');
          onUpdate();
        } catch (error: any) {
          message.error(`删除失败: ${error.message}`);
        } finally {
          setDeleting(false);
        }
      },
    });
  };

  const connections = pod.connections;
  const hasConnections = connections?.ssh?.host && connections?.ssh?.port;

  return (
    <Card className="pod-card" hoverable>
      <div className="pod-header">
        <Space>
          <StatusBadge status={pod.status} phase={pod.phase} />
          <span className="pod-name">{pod.name}</span>
        </Space>
      </div>

      <div className="pod-info">
        <div className="info-item">
          GPU: {pod.gpuType || '无'} x{pod.gpuCount}
        </div>
        <div className="info-item">
          <CountdownTimer expiresAt={pod.expiresAt} />
        </div>
        <div className="info-item warning-text">
          将在今晚11点自动删除
        </div>
      </div>

      <Divider />

      {hasConnections && (
        <>
          <div className="connection-info">
            <div className="connection-title">连接信息</div>
            
            <div className="connection-item">
              <div className="connection-label">SSH:</div>
              <div className="connection-value">
                <code style={{ fontSize: '12px' }}>{connections.apps.sshCommand}</code>
                <Tooltip title="复制 SSH 命令">
                  <Button 
                    size="small" 
                    icon={<CopyOutlined />} 
                    onClick={() => copyToClipboard(connections.apps.sshCommand, 'SSH 命令')}
                    style={{ marginLeft: 8 }}
                  />
                </Tooltip>
              </div>
            </div>

            <div className="connection-item" style={{ marginTop: 8 }}>
              <Space wrap>
                <Tooltip title="使用 VSCode 打开">
                  <Button 
                    size="small"
                    type="primary"
                    icon={<CodeOutlined />}
                    onClick={() => window.open(connections.apps.vscodeURI, '_blank')}
                  >
                    VSCode
                  </Button>
                </Tooltip>
                <Tooltip title="使用 Xshell 打开 (Windows)">
                  <Button 
                    size="small"
                    icon={<WindowsOutlined />}
                    onClick={() => window.open(connections.apps.xshellURI, '_blank')}
                  >
                    Xshell
                  </Button>
                </Tooltip>
                <Tooltip title="使用终端打开 (macOS/Linux)">
                  <Button 
                    size="small"
                    icon={<DesktopOutlined />}
                    onClick={() => window.open(connections.apps.terminalURI, '_blank')}
                  >
                    Terminal
                  </Button>
                </Tooltip>
                <Tooltip title="复制密码">
                  <Button 
                    size="small"
                    icon={<CopyOutlined />}
                    onClick={() => copyToClipboard(connections.ssh.password, '密码')}
                  >
                    密码
                  </Button>
                </Tooltip>
              </Space>
            </div>
          </div>
          <Divider />
        </>
      )}

      <Space className="pod-actions">
        <Button
          size="small"
          icon={<ClockCircleOutlined />}
          onClick={handleExtend}
        >
          延长
        </Button>
        <Button
          size="small"
          icon={<EyeOutlined />}
          onClick={() => navigate(`/pods/${pod.id}`)}
        >
          详情
        </Button>
        <Button
          size="small"
          danger
          icon={<DeleteOutlined />}
          onClick={handleDelete}
          loading={deleting}
        >
          删除
        </Button>
      </Space>
    </Card>
  );
};

export default PodCard;
