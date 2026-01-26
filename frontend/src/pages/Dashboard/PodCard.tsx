import { CloudServerOutlined, CodeOutlined, CopyOutlined, DeleteOutlined, EyeOutlined } from '@ant-design/icons';
import { Button, Divider, Modal, Space, Tooltip, Typography, message } from 'antd';
import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import GlassCard from '../../components/GlassCard';
import StatusBadge from '../../components/StatusBadge';
import { deletePod } from '../../services/api';
import './PodCard.css';

const { Text } = Typography;

interface PodCardProps {
  pod: any;
  onUpdate: () => void;
}

const PodCard: React.FC<PodCardProps> = ({ pod, onUpdate }) => {
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState(false);

  const copyToClipboard = async (text: string, label: string) => {
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        message.success(`${label} 已复制`);
      } else {
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

  const showVSCodeGuide = () => {
    const namespace = pod.namespace;
    const podName = pod.name;
    const container = pod.container || 'workspace';
    const kubectlCmd = `kubectl exec -it -n ${namespace} ${podName} -c ${container} -- /bin/bash`;

    Modal.info({
      title: (
        <div className="modal-title-custom">
          <CodeOutlined />
          <span>连接到 Pod</span>
        </div>
      ),
      width: 600,
      content: (
        <div className="vscode-guide">
          <div className="guide-section">
            <h4>方法 1: VSCode Kubernetes 插件</h4>
            <ol>
              <li>安装扩展: <Text code className="mono">ms-kubernetes-tools.vscode-kubernetes-tools</Text></li>
              <li>点击 VSCode 左侧 Kubernetes 图标</li>
              <li>展开: 集群 → Namespaces → <Text code>{namespace}</Text> → Pods</li>
              <li>右键 <Text code>{podName}</Text> → <Text strong>"Attach Visual Studio Code"</Text></li>
            </ol>
          </div>

          <Divider />

          <div className="guide-section">
            <h4>方法 2: kubectl exec</h4>
            <div className="command-box">
              <code className="mono">{kubectlCmd}</code>
              <Button
                size="small"
                icon={<CopyOutlined />}
                onClick={() => copyToClipboard(kubectlCmd, 'kubectl 命令')}
              />
            </div>
          </div>

          <Divider />

          <div className="guide-section">
            <h4>Pod 信息</h4>
            <div className="info-grid">
              <div className="info-item">
                <span className="info-label">Namespace</span>
                <Text code copyable className="mono">{namespace}</Text>
              </div>
              <div className="info-item">
                <span className="info-label">Pod</span>
                <Text code copyable className="mono">{podName}</Text>
              </div>
              <div className="info-item">
                <span className="info-label">Container</span>
                <Text code copyable className="mono">{container}</Text>
              </div>
            </div>
          </div>
        </div>
      ),
      okText: '知道了',
    });
  };

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
        <div className="delete-confirm">
          <p><strong>Pod 名称:</strong> {pod.name}</p>
          <p className="delete-warning">
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

  const isRunning = pod.status === 'Running';

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'Running': return 'var(--success)';
      case 'Pending': return 'var(--warning)';
      case 'Failed': return 'var(--error)';
      default: return 'var(--text-muted)';
    }
  };

  return (
    <GlassCard className="pod-card" glow={isRunning}>
      {/* Header */}
      <div className="pod-card-header">
        <div className="pod-status-indicator" style={{ background: getStatusColor(pod.status) }} />
        <div className="pod-title-section">
          <h3 className="pod-name">{pod.name}</h3>
          <StatusBadge status={pod.status} />
        </div>
      </div>

      {/* Specs */}
      <div className="pod-specs">
        <div className="spec-item">
          <span className="spec-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <rect x="4" y="4" width="16" height="16" rx="2"/>
              <rect x="9" y="9" width="6" height="6"/>
            </svg>
          </span>
          <span className="spec-value">{pod.cpu || '-'} 核</span>
        </div>
        <div className="spec-item">
          <span className="spec-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z"/>
              <polyline points="22,6 12,13 2,6"/>
            </svg>
          </span>
          <span className="spec-value">{pod.memory || '-'}</span>
        </div>
        <div className="spec-item">
          <span className="spec-icon gpu">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <rect x="2" y="6" width="20" height="12" rx="2"/>
              <line x1="6" y1="12" x2="6" y2="12"/>
              <line x1="10" y1="12" x2="10" y2="12"/>
              <line x1="14" y1="12" x2="14" y2="12"/>
              <line x1="18" y1="12" x2="18" y2="12"/>
            </svg>
          </span>
          <span className="spec-value">
            {pod.gpuType ? `${pod.gpuType} ×${pod.gpuCount}` : '无 GPU'}
          </span>
        </div>
      </div>

      {/* Warning */}
      <div className="pod-warning">
        <span className="warning-icon">⏰</span>
        <span>今晚 23:00 自动删除</span>
      </div>

      {/* Connection Buttons */}
      {isRunning && (
        <>
          <Divider className="pod-divider" />
          <div className="pod-connections">
            <span className="connections-label">快速连接</span>
            <Space size="small" wrap>
              <Tooltip title="VSCode 连接指南">
                <Button
                  size="small"
                  type="primary"
                  icon={<CodeOutlined />}
                  onClick={showVSCodeGuide}
                >
                  VSCode
                </Button>
              </Tooltip>
              <Tooltip title="复制 kubectl exec 命令">
                <Button
                  size="small"
                  icon={<CopyOutlined />}
                  onClick={copyKubectlExecCommand}
                  className="glass-button"
                >
                  kubectl
                </Button>
              </Tooltip>
              <Tooltip title="复制 Namespace">
                <Button
                  size="small"
                  icon={<CloudServerOutlined />}
                  onClick={() => copyToClipboard(pod.namespace, 'Namespace')}
                  className="glass-button"
                />
              </Tooltip>
            </Space>
          </div>
        </>
      )}

      {/* Actions */}
      <Divider className="pod-divider" />
      <div className="pod-actions">
        <Button
          size="small"
          icon={<EyeOutlined />}
          onClick={() => navigate(`/pods/${pod.id}`)}
          className="glass-button"
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
      </div>
    </GlassCard>
  );
};

export default PodCard;
