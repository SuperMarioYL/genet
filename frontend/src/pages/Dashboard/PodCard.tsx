import { CloudServerOutlined, CodeOutlined, CopyOutlined, DeleteOutlined, EyeOutlined } from '@ant-design/icons';
import { Button, Card, Divider, Modal, Space, Tooltip, Typography, message } from 'antd';
import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import StatusBadge from '../../components/StatusBadge';
import { deletePod } from '../../services/api';
import './PodCard.css';

const { Text, Paragraph } = Typography;

interface PodCardProps {
  pod: any;
  onUpdate: () => void;
}

const PodCard: React.FC<PodCardProps> = ({ pod, onUpdate }) => {
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState(false);

  const copyToClipboard = async (text: string, label: string) => {
    try {
      // 优先使用 Clipboard API
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        message.success(`${label} 已复制`);
      } else {
        // 降级方案：使用 execCommand
        const textArea = document.createElement('textarea');
        textArea.value = text;
        textArea.style.position = 'fixed';
        textArea.style.left = '-9999px';
        document.body.appendChild(textArea);
        textArea.select();
        document.execCommand('copy');
        document.body.removeChild(textArea);
        message.success(`${label} 已复制`);
      }
    } catch (err) {
      // 如果都失败，使用降级方案
      const textArea = document.createElement('textarea');
      textArea.value = text;
      textArea.style.position = 'fixed';
      textArea.style.left = '-9999px';
      document.body.appendChild(textArea);
      textArea.select();
      try {
        document.execCommand('copy');
        message.success(`${label} 已复制`);
      } catch (e) {
        message.error(`复制失败，请手动复制: ${text}`);
      }
      document.body.removeChild(textArea);
    }
  };

  // 显示 VSCode 连接指南
  const showVSCodeGuide = () => {
    const namespace = pod.namespace;
    const podName = pod.name;
    const container = pod.container || 'workspace';
    const kubectlCmd = `kubectl exec -it -n ${namespace} ${podName} -c ${container} -- /bin/bash`;

    Modal.info({
      title: '使用 VSCode 连接到 Pod',
      width: 600,
      content: (
        <div style={{ marginTop: 16 }}>
          <Paragraph>
            <Text strong>方法 1: 使用 Kubernetes 插件（推荐）</Text>
          </Paragraph>
          <ol style={{ paddingLeft: 20 }}>
            <li>安装 VSCode 扩展: <Text code>ms-kubernetes-tools.vscode-kubernetes-tools</Text></li>
            <li>在 VSCode 左侧边栏点击 Kubernetes 图标</li>
            <li>展开集群 → Namespaces → <Text code>{namespace}</Text> → Pods</li>
            <li>右键点击 <Text code>{podName}</Text></li>
            <li>选择 <Text strong>"Attach Visual Studio Code"</Text></li>
          </ol>

          <Divider />

          <Paragraph>
            <Text strong>方法 2: 使用 kubectl exec</Text>
          </Paragraph>
          <Paragraph copyable={{ text: kubectlCmd }}>
            <Text code style={{ wordBreak: 'break-all' }}>{kubectlCmd}</Text>
          </Paragraph>

          <Divider />

          <Paragraph>
            <Text strong>Pod 信息</Text>
          </Paragraph>
          <ul style={{ paddingLeft: 20 }}>
            <li>Namespace: <Text code copyable>{namespace}</Text></li>
            <li>Pod: <Text code copyable>{podName}</Text></li>
            <li>Container: <Text code copyable>{container}</Text></li>
          </ul>
        </div>
      ),
      okText: '知道了',
    });
  };

  // 复制 kubectl exec 命令
  const copyKubectlExecCommand = () => {
    const namespace = pod.namespace;
    const podName = pod.name;
    const container = pod.container || 'workspace';
    
    const cmd = `kubectl exec -it -n ${namespace} ${podName} -c ${container} -- /bin/bash`;
    copyToClipboard(cmd, 'kubectl exec 命令');
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

  // Pod 是否处于运行状态
  const isRunning = pod.status === 'Running';

  return (
    <Card className="pod-card" hoverable>
      <div className="pod-header">
        <Space>
          <StatusBadge status={pod.status} />
          <span className="pod-name">{pod.name}</span>
        </Space>
      </div>

      <div className="pod-info">
        <div className="info-item">
          CPU: {pod.cpu || '-'} 核 | 内存: {pod.memory || '-'}
        </div>
        <div className="info-item">
          GPU: {pod.gpuType || '无'} x{pod.gpuCount}
        </div>
        <div className="info-item warning-text">
          将在今晚 23:00 自动删除
        </div>
      </div>

      <Divider />

      {/* 连接按钮区域 */}
      {isRunning && (
        <>
          <div className="connection-info">
            <div className="connection-title">连接方式</div>
            <div className="connection-item" style={{ marginTop: 8 }}>
              <Space wrap>
                <Tooltip title="查看 VSCode 连接指南">
                  <Button 
                    size="small"
                    type="primary"
                    icon={<CodeOutlined />}
                    onClick={showVSCodeGuide}
                  >
                    VSCode 连接
                  </Button>
                </Tooltip>

                <Tooltip title="复制 kubectl exec 命令">
                  <Button 
                    size="small"
                    icon={<CopyOutlined />}
                    onClick={copyKubectlExecCommand}
                  >
                    kubectl exec
                  </Button>
                </Tooltip>

                <Tooltip title="复制 Namespace">
                  <Button 
                    size="small"
                    icon={<CloudServerOutlined />}
                    onClick={() => copyToClipboard(pod.namespace, 'Namespace')}
                  >
                    Namespace
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
