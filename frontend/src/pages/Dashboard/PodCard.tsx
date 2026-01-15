import { ClockCircleOutlined, CodeOutlined, CopyOutlined, DeleteOutlined, DesktopOutlined, DownloadOutlined, EyeOutlined } from '@ant-design/icons';
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

  // 检测操作系统
  const detectOS = (): 'windows' | 'mac' | 'linux' => {
    const ua = navigator.userAgent.toLowerCase();
    if (ua.includes('win')) return 'windows';
    if (ua.includes('mac')) return 'mac';
    return 'linux';
  };

  // 打开 VSCode
  const openVSCode = (uri: string) => {
    window.location.href = uri;
    message.info(
      '正在打开 VSCode... 如果覆盖了当前项目，请在设置中将 window.openFoldersInNewWindow 设为 on',
      5
    );
  };

  // 尝试使用 ssh:// 协议打开默认 SSH 客户端
  const openSSHClient = (sshURI: string, sshCmd: string) => {
    const link = document.createElement('a');
    link.href = sshURI;
    link.click();
    
    copyToClipboard(sshCmd, 'SSH 命令');
    message.info('正在尝试打开 SSH 客户端... 如未打开，命令已复制', 4);
  };

  // 下载 Xshell 会话文件（前端直接生成）
  const downloadXshellFile = () => {
    if (!connections) return;
    
    // 生成 Xshell 会话文件内容（标准 INI 格式）
    const xshContent = `[CONNECTION]
Host=${connections.ssh.host}
Port=${connections.ssh.port}
Protocol=SSH

[AUTHENTICATION]
UserName=${connections.ssh.user}
Method=Password
Password=${connections.ssh.password}

[TERMINAL]
Type=xterm
`;
    
    // 创建 Blob 并下载（使用 text/plain 类型）
    const blob = new Blob([xshContent], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${pod.name}.xsh`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    
    message.success('Xshell 会话文件已下载，双击打开即可连接', 3);
  };

  // 复制 SSH 命令到剪贴板
  const copySSHCommand = (sshCmd: string, platform: string) => {
    copyToClipboard(sshCmd, 'SSH 命令');
    if (platform === 'mac') {
      message.info('SSH 命令已复制，请在 Terminal.app 中粘贴运行', 3);
    } else if (platform === 'windows') {
      message.info('SSH 命令已复制，请在 Windows Terminal 中粘贴运行', 3);
    } else {
      message.info('SSH 命令已复制，请在终端中粘贴运行', 3);
    }
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
                <Tooltip title="使用 VSCode 打开（新窗口）">
                  <Button 
                    size="small"
                    type="primary"
                    icon={<CodeOutlined />}
                    onClick={() => openVSCode(connections.apps.vscodeURI)}
                  >
                    VSCode
                  </Button>
                </Tooltip>

                <Tooltip title="下载 Xshell 会话文件">
                  <Button 
                    size="small"
                    icon={<DownloadOutlined />}
                    onClick={downloadXshellFile}
                  >
                    Xshell
                  </Button>
                </Tooltip>
                
                <Tooltip title="尝试打开默认 SSH 客户端">
                  <Button 
                    size="small"
                    icon={<DesktopOutlined />}
                    onClick={() => openSSHClient(
                      connections.apps.xshellURI,
                      connections.apps.sshCommand
                    )}
                  >
                    SSH
                  </Button>
                </Tooltip>

                <Tooltip title="复制 SSH 命令">
                  <Button 
                    size="small"
                    icon={<CopyOutlined />}
                    onClick={() => copySSHCommand(connections.apps.sshCommand, detectOS())}
                  >
                    命令
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
