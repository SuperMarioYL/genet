import { CloudServerOutlined, CodeOutlined, CopyOutlined, DeleteOutlined, DownloadOutlined, EyeOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Button, Divider, Modal, Space, Tooltip, Typography, message } from 'antd';
import dayjs from 'dayjs';
import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import GlassCard from '../../components/GlassCard';
import StatusBadge from '../../components/StatusBadge';
import { deletePod, downloadPodYAML, extendPod } from '../../services/api';
import { getNextCleanupTime } from '../../utils/cleanup';
import './PodCard.css';

const { Text } = Typography;

interface PodCardProps {
  pod: any;
  onUpdate: () => void;
  cleanupSchedule?: string;
  cleanupTimezone?: string;
}

const shellSafePattern = /^[A-Za-z0-9_./:-]+$/;

const quoteShellArg = (value: string) => {
  if (shellSafePattern.test(value)) {
    return value;
  }
  return `'${value.replace(/'/g, `'\"'\"'`)}'`;
};

const inferCustomPodName = (pod: any) => {
  const namespace = typeof pod?.namespace === 'string' ? pod.namespace : '';
  const podName = typeof pod?.name === 'string' ? pod.name : '';
  if (!namespace.startsWith('user-') || podName === '') {
    return '';
  }

  const userIdentifier = namespace.slice('user-'.length);
  const prefix = `pod-${userIdentifier}-`;
  if (!podName.startsWith(prefix)) {
    return '';
  }

  const suffix = podName.slice(prefix.length);
  if (suffix === '' || /^\d+$/.test(suffix)) {
    return '';
  }

  return suffix;
};

const buildGenetRunCommand = (pod: any) => {
  const parts = ['genet', 'run'];
  const image = typeof pod?.image === 'string' ? pod.image.trim() : '';
  parts.push(image ? quoteShellArg(image) : '<image>');

  const customName = inferCustomPodName(pod);
  if (customName) {
    parts.push('--name', quoteShellArg(customName));
  }

  const gpuCount = typeof pod?.gpuCount === 'number' ? pod.gpuCount : 0;
  parts.push('--gpus', String(gpuCount));

  const gpuType = typeof pod?.gpuType === 'string' ? pod.gpuType.trim() : '';
  if (gpuCount > 0 && gpuType) {
    parts.push('--gpu-type', quoteShellArg(gpuType));
  }

  const cpu = typeof pod?.cpu === 'string' ? pod.cpu.trim() : '';
  if (cpu) {
    parts.push('--cpu', quoteShellArg(cpu));
  }

  const memory = typeof pod?.memory === 'string' ? pod.memory.trim() : '';
  if (memory) {
    parts.push('--memory', quoteShellArg(memory));
  }

  return parts.join(' ');
};

const PodCard: React.FC<PodCardProps> = ({ pod, onUpdate, cleanupSchedule, cleanupTimezone }) => {
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState(false);
  const [extending, setExtending] = useState(false);
  const connections = pod.connections;
  const codeServerURL = connections?.apps?.codeServerURL;
  const codeServerReady = Boolean(connections?.apps?.codeServerReady);
  const codeServerStatus = connections?.apps?.codeServerStatus || 'unavailable';
  const webShellURL = connections?.apps?.webShellURL;
  const webShellReady = Boolean(connections?.apps?.webShellReady);
  const webShellStatus = connections?.apps?.webShellStatus || 'unavailable';

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

  // 可复制代码块组件
  const CodeBlock: React.FC<{ code: string; size?: 'sm' | 'md' | 'lg'; label?: string }> = ({ code, size = 'md', label }) => {
    const sizeClass = size === 'sm' ? 'code-block-sm' : size === 'lg' ? 'code-block-lg' : '';
    return (
      <div className="code-block-wrapper">
        <pre className={`code-block mono ${sizeClass}`}>{code}</pre>
        <Button
          className="code-copy-btn"
          size="small"
          type="text"
          icon={<CopyOutlined />}
          onClick={() => copyToClipboard(code, label || '命令')}
        />
      </div>
    );
  };

  const showVSCodeGuide = () => {
    const namespace = pod.namespace;
    const podName = pod.name;
    const container = pod.container || 'workspace';
    const kubectlCmd = `kubectl exec -it -n ${namespace} ${podName} -c ${container} -- /bin/sh`;

    // 命令定义
    const commands = {
      kubectlMac: 'brew install kubectl@1.23',
      kubectlWin: 'choco install kubernetes-cli --version=1.23.1',
      homebrew: '/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"',
      chocolatey: 'Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString(\'https://community.chocolatey.org/install.ps1\'))',
      k8sPlugin: 'ms-kubernetes-tools.vscode-kubernetes-tools',
      devContainerPlugin: 'ms-vscode-remote.remote-containers',
      kubeconfigMac: '~/.kube/config',
      kubeconfigWin: '%USERPROFILE%\\.kube\\config',
    };

    Modal.info({
      title: (
        <div className="modal-title-custom">
          <CodeOutlined />
          <span>连接到 Pod</span>
        </div>
      ),
      width: 800,
      content: (
        <div className="vscode-guide">
          {/* 环境准备 */}
          <div className="guide-section">
            <h4>📦 环境准备（首次使用）</h4>
            
            <div className="setup-step">
              <div className="step-title">1. 安装 kubectl</div>
              <div className="setup-commands">
                <div className="cmd-group">
                  <span className="cmd-label">macOS:</span>
                  <CodeBlock code={commands.kubectlMac} label="kubectl 安装命令" />
                </div>
                <div className="cmd-hint">
                  未安装 Homebrew？先运行：
                  <CodeBlock code={commands.homebrew} size="sm" label="Homebrew 安装命令" />
                </div>
                <div className="cmd-group">
                  <span className="cmd-label">Windows:</span>
                  <CodeBlock code={commands.kubectlWin} label="kubectl 安装命令" />
                </div>
                <div className="cmd-hint">
                  未安装 Chocolatey？以管理员身份运行 PowerShell：
                  <CodeBlock code={commands.chocolatey} size="sm" label="Chocolatey 安装命令" />
                </div>
              </div>
            </div>

            <div className="setup-step">
              <div className="step-title">2. 安装 VSCode 插件</div>
              <div className="setup-commands">
                <div className="cmd-group">
                  <span className="cmd-label">Kubernetes:</span>
                  <CodeBlock code={commands.k8sPlugin} label="插件 ID" />
                </div>
                <div className="cmd-group">
                  <span className="cmd-label">Dev Containers:</span>
                  <CodeBlock code={commands.devContainerPlugin} label="插件 ID" />
                </div>
              </div>
            </div>

            <div className="setup-step">
              <div className="step-title">3. 配置 Kubeconfig</div>
              <p className="step-desc">
                点击页面顶部 <Text strong>"Kubeconfig"</Text> 按钮下载配置文件，保存到：
              </p>
              <div className="setup-commands">
                <div className="cmd-group">
                  <span className="cmd-label">macOS/Linux:</span>
                  <CodeBlock code={commands.kubeconfigMac} label="路径" />
                </div>
                <div className="cmd-group">
                  <span className="cmd-label">Windows:</span>
                  <CodeBlock code={commands.kubeconfigWin} label="路径" />
                </div>
              </div>
            </div>
          </div>

          <Divider />

          {/* 连接方式 */}
          <div className="guide-section">
            <h4>🔗 连接到 Pod</h4>
            <ol className="connect-steps">
              <li>打开 VSCode，点击左侧 <Text strong>Kubernetes</Text> 图标</li>
              <li>展开集群 → Workloads → Pods</li>
              <li>右键点击 <Text code>{podName}</Text></li>
              <li>选择 <Text strong>"Attach Visual Studio Code"</Text></li>
            </ol>
          </div>

          <Divider />

          {/* 命令行方式 */}
          <div className="guide-section">
            <h4>💻 命令行连接</h4>
            <CodeBlock code={kubectlCmd} size="lg" label="kubectl 命令" />
          </div>

          <Divider />

          {/* Pod 信息 */}
          <div className="guide-section">
            <h4>📋 Pod 信息</h4>
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
    const cmd = `kubectl exec -it -n ${namespace} ${podName} -c ${container} -- /bin/sh`;
    copyToClipboard(cmd, 'kubectl exec 命令');
  };

  const copyGenetRunCommand = () => {
    copyToClipboard(buildGenetRunCommand(pod), 'genet 命令');
  };

  const openInNewTab = (href: string) => {
    window.open(href, '_blank', 'noopener,noreferrer');
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

  const handleDownloadYAML = () => {
    downloadPodYAML(pod.id);
  };

  const isRunning = pod.status === 'Running';

  // 处理延长保护
  const handleExtend = async () => {
    setExtending(true);
    try {
      const response = await extendPod(pod.id);
      message.success(`Pod 已保护至 ${dayjs(response.protectedUntil).format('MM-DD HH:mm')}`);
      onUpdate();
    } catch (error: any) {
      message.error(`延长保护失败: ${error.message}`);
    } finally {
      setExtending(false);
    }
  };

  // 获取保护状态信息
  const getProtectionInfo = () => {
    const protectedUntil = pod.protectedUntil ? dayjs(pod.protectedUntil) : null;
    const now = dayjs();
    const cleanupInfo = getNextCleanupTime(cleanupSchedule || '0 23 * * *', cleanupTimezone);
    const cleanupTime = cleanupInfo?.nextTime || now.hour(23).minute(0).second(0);
    const cleanupLabel = cleanupInfo?.label || '今天 23:00';

    if (!protectedUntil || protectedUntil.isBefore(now)) {
      // 未保护或保护已过期
      return {
        type: 'warning' as const,
        icon: '⏰',
        text: `${cleanupLabel} 自动清理`,
        canExtend: true,
      };
    }

    // 判断保护时间是否在下次清理之前（即会被清理）
    if (protectedUntil.isBefore(cleanupTime)) {
      return {
        type: 'warning' as const,
        icon: '⚠️',
        text: `${cleanupLabel} 将清理（可再次延长）`,
        canExtend: true,
      };
    }

    // 保护到清理时间之后，显示保护状态
    return {
      type: 'success' as const,
      icon: '✅',
      text: `已保护至 ${protectedUntil.format('MM-DD HH:mm')}`,
      canExtend: true,
    };
  };

  const protectionInfo = getProtectionInfo();

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
              <rect x="3" y="6" width="18" height="12" rx="1"/>
              <line x1="7" y1="10" x2="7" y2="14"/>
              <line x1="11" y1="10" x2="11" y2="14"/>
              <line x1="15" y1="10" x2="15" y2="14"/>
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
            {pod.gpuType ? `${pod.gpuType} ×${pod.gpuCount}` : (pod.gpuCount > 0 ? `GPU ×${pod.gpuCount}` : '无 GPU')}
          </span>
        </div>
      </div>

      {/* Protection Status */}
      <div className={`pod-protection pod-protection-${protectionInfo.type}`}>
        <div className="protection-info">
          <span className="protection-icon">{protectionInfo.icon}</span>
          <span className="protection-text">{protectionInfo.text}</span>
        </div>
        {protectionInfo.canExtend && (
          <Tooltip title="延长保护至明天 22:59">
                  <Button 
                    size="small" 
              type="link"
              icon={<SafetyCertificateOutlined />}
              onClick={handleExtend}
              loading={extending}
              className="extend-btn"
            >
              延长
            </Button>
                </Tooltip>
        )}
            </div>

      {/* Connection Buttons */}
      {isRunning && (
        <>
          <Divider className="pod-divider" />
          <div className="pod-connections">
            <span className="connections-label">快速连接</span>
            <Space size="small" wrap>
              {codeServerURL && (
                <Tooltip title={codeServerReady ? '在浏览器中打开 code-server' : 'code-server 启动中，请稍后重试'}>
                  <Button
                    size="small"
                    type="primary"
                    icon={<CloudServerOutlined />}
                    onClick={() => openInNewTab(codeServerURL)}
                    disabled={!codeServerReady}
                  >
                    code-server
                  </Button>
                </Tooltip>
              )}
              {webShellURL && (
                <Tooltip title={webShellReady ? '打开浏览器终端' : 'Web Shell 当前不可用'}>
                  <Button
                    size="small"
                    type="primary"
                    icon={<CodeOutlined />}
                    onClick={() => openInNewTab(webShellURL)}
                    disabled={!webShellReady}
                  >
                    Web Shell
                  </Button>
                </Tooltip>
              )}
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
              <Tooltip title="复制 genet run 命令">
                  <Button
                    size="small"
                    icon={<CopyOutlined />}
                    onClick={copyGenetRunCommand}
                    className="glass-button"
                  >
                    genet
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
            {codeServerURL && !codeServerReady && (
              <Text type="secondary" className="connections-hint">
                {codeServerStatus === 'starting' ? '启动中' : '不可用'}
              </Text>
            )}
            {webShellURL && !webShellReady && (
              <Text type="secondary" className="connections-hint">
                {webShellStatus === 'enabled' ? '启动中' : '不可用'}
              </Text>
            )}
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
          icon={<DownloadOutlined />}
          onClick={handleDownloadYAML}
          className="glass-button"
        >
          YAML
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
