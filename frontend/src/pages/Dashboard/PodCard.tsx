import { ClockCircleOutlined, CloudServerOutlined, CodeOutlined, CopyOutlined, DeleteOutlined, EyeOutlined } from '@ant-design/icons';
import { Button, Card, Divider, Modal, Space, Tooltip, message } from 'antd';
import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import CountdownTimer from '../../components/CountdownTimer';
import StatusBadge from '../../components/StatusBadge';
import { deletePod, extendPod } from '../../services/api';
import './PodCard.css';

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

  // 使用 VSCode Kubernetes 插件附加到 Pod（新窗口）
  const attachVSCodeK8s = () => {
    const namespace = pod.namespace;
    const podName = pod.name;
    const container = pod.container || 'workspace';
    
    // 构建 VSCode Kubernetes 插件的附加 URI
    // 添加 windowId=_blank 参数强制在新窗口打开
    // 文档: https://github.com/vscode-kubernetes-tools/vscode-kubernetes-tools
    const uri = `vscode://ms-kubernetes-tools.vscode-kubernetes-tools/attach?namespace=${encodeURIComponent(namespace)}&pod=${encodeURIComponent(podName)}&container=${encodeURIComponent(container)}&windowId=_blank`;
    
    // 使用 window.open 尝试在新窗口打开
    const newWindow = window.open(uri, '_blank');
    
    // 如果 window.open 被阻止，回退到 location.href
    if (!newWindow) {
      window.location.href = uri;
    }
    
    message.info(
      '正在打开 VSCode Kubernetes 插件（新窗口）... 请确保已安装 Kubernetes 插件',
      5
    );
  };

  // 复制 kubectl exec 命令
  const copyKubectlExecCommand = () => {
    const namespace = pod.namespace;
    const podName = pod.name;
    const container = pod.container || 'workspace';
    
    const cmd = `kubectl exec -it -n ${namespace} ${podName} -c ${container} -- /bin/bash`;
    copyToClipboard(cmd, 'kubectl exec 命令');
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

      {/* 连接按钮区域 */}
      {isRunning && (
        <>
          <div className="connection-info">
            <div className="connection-title">连接方式</div>
            <div className="connection-item" style={{ marginTop: 8 }}>
              <Space wrap>
                <Tooltip title="使用 VSCode Kubernetes 插件附加到 Pod（需要安装插件）">
                  <Button 
                    size="small"
                    type="primary"
                    icon={<CodeOutlined />}
                    onClick={attachVSCodeK8s}
                  >
                    VSCode K8s
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
