import { ArrowLeftOutlined, CodeOutlined, CopyOutlined, DesktopOutlined, DownloadOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons';
import { Alert, Button, Descriptions, Input, Layout, message, Modal, Progress, Skeleton, Space, Switch, Table, Tabs, Tag, Tooltip, Typography } from 'antd';
import dayjs from 'dayjs';
import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import GlassCard from '../../components/GlassCard';
import StatusBadge from '../../components/StatusBadge';
import ThemeToggle from '../../components/ThemeToggle';
import { commitImage, CommitStatus, getCommitLogs, getCommitStatus, getPod, getPodDescribe, getPodEvents, getPodLogs, getSharedGPUPods, SharedGPUPod } from '../../services/api';
import './index.css';

const { Header, Content } = Layout;
const { Text } = Typography;

const PodDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [pod, setPod] = useState<any>(null);
  const [logs, setLogs] = useState<string>('');
  const [events, setEvents] = useState<any[]>([]);
  const [describe, setDescribe] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [logsLoading, setLogsLoading] = useState(false);
  const [eventsLoading, setEventsLoading] = useState(false);
  const [describeLoading, setDescribeLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [commitModalVisible, setCommitModalVisible] = useState(false);
  const [commitImageName, setCommitImageName] = useState('');
  const [commitStatus, setCommitStatus] = useState<CommitStatus | null>(null);
  const [commitLogs, setCommitLogs] = useState<string>('');
  const [commitSubmitting, setCommitSubmitting] = useState(false);
  const commitPollRef = useRef<NodeJS.Timeout | null>(null);
  const [sharedGPUPods, setSharedGPUPods] = useState<SharedGPUPod[]>([]);
  const [sharedGPULoading, setSharedGPULoading] = useState(false);
  const [autoRefreshLogs, setAutoRefreshLogs] = useState(false);
  const [activeTab, setActiveTab] = useState('overview');
  const logRefreshRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    if (id) {
      loadPod();
      loadCommitStatus();
      loadSharedGPUPods();
    }
    return () => {
      if (commitPollRef.current) {
        clearInterval(commitPollRef.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  // 日志自动刷新
  useEffect(() => {
    if (autoRefreshLogs && activeTab === 'logs') {
      logRefreshRef.current = setInterval(() => {
        loadLogs();
      }, 3000);
    }
    return () => {
      if (logRefreshRef.current) {
        clearInterval(logRefreshRef.current);
        logRefreshRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoRefreshLogs, activeTab]);

  const loadPod = async () => {
    setLoading(true);
    try {
      const data = await getPod(id!);
      setPod(data);
    } catch (error: any) {
      message.error(`加载 Pod 详情失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const loadLogs = async () => {
    setLogsLoading(true);
    try {
      const data: any = await getPodLogs(id!);
      setLogs(data.logs || '暂无日志');
    } catch (error: any) {
      message.error(`加载日志失败: ${error.message}`);
      setLogs('加载日志失败');
    } finally {
      setLogsLoading(false);
    }
  };

  const loadEvents = async () => {
    setEventsLoading(true);
    try {
      const data: any = await getPodEvents(id!);
      setEvents(data.events || []);
    } catch (error: any) {
      message.error(`加载事件失败: ${error.message}`);
    } finally {
      setEventsLoading(false);
    }
  };

  const loadDescribe = async () => {
    setDescribeLoading(true);
    try {
      const data: any = await getPodDescribe(id!);
      setDescribe(data);
    } catch (error: any) {
      message.error(`加载详情失败: ${error.message}`);
    } finally {
      setDescribeLoading(false);
    }
  };

  const loadSharedGPUPods = async () => {
    setSharedGPULoading(true);
    try {
      const data = await getSharedGPUPods(id!);
      setSharedGPUPods(data.pods || []);
    } catch (error: any) {
      // 静默失败，不显示错误消息
      console.error('Failed to load shared GPU pods:', error);
    } finally {
      setSharedGPULoading(false);
    }
  };

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
      message.error(`复制失败`);
    }
  };

  const openVSCode = (uri: string) => {
    window.location.href = uri;
    message.info('正在打开 VSCode...', 5);
  };

  const openSSHClient = (sshURI: string, sshCmd: string) => {
    const link = document.createElement('a');
    link.href = sshURI;
    link.click();
    copyToClipboard(sshCmd, 'SSH 命令');
    message.info('正在尝试打开 SSH 客户端...', 4);
  };

  const downloadXshellFile = () => {
    if (!pod || !connections) return;
    const xshContent = `[CONNECTION]\nHost=${connections.ssh.host}\nPort=${connections.ssh.port}\nProtocol=SSH\n\n[AUTHENTICATION]\nUserName=${connections.ssh.user}\nMethod=Password\nPassword=${connections.ssh.password}\n\n[TERMINAL]\nType=xterm\n`;
    const blob = new Blob([xshContent], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${pod.name}.xsh`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    message.success('Xshell 会话文件已下载', 3);
  };

  const loadCommitStatus = async () => {
    try {
      const status = await getCommitStatus(id!);
      setCommitStatus(status);
      if (status.hasJob && (status.status === 'Pending' || status.status === 'Running')) {
        startCommitPolling();
      }
    } catch (error) {}
  };

  const startCommitPolling = () => {
    if (commitPollRef.current) {
      clearInterval(commitPollRef.current);
    }
    commitPollRef.current = setInterval(async () => {
      try {
        const status = await getCommitStatus(id!);
        setCommitStatus(status);
        try {
          const logsData = await getCommitLogs(id!);
          setCommitLogs(logsData.logs || '');
        } catch (e) {}
        if (status.status === 'Succeeded' || status.status === 'Failed') {
          if (commitPollRef.current) {
            clearInterval(commitPollRef.current);
            commitPollRef.current = null;
          }
          if (status.status === 'Succeeded') {
            message.success('镜像保存成功！');
          } else {
            message.error('镜像保存失败');
          }
        }
      } catch (error) {}
    }, 3000);
  };

  const handleCommit = async () => {
    if (!commitImageName.trim()) {
      message.error('请输入目标镜像名称');
      return;
    }
    setCommitSubmitting(true);
    try {
      await commitImage(id!, commitImageName.trim());
      message.success('镜像保存任务已创建');
      setCommitModalVisible(false);
      setCommitImageName('');
      loadCommitStatus();
      startCommitPolling();
    } catch (error: any) {
      message.error(`创建任务失败: ${error.message}`);
    } finally {
      setCommitSubmitting(false);
    }
  };

  const loadCommitLogs = async () => {
    try {
      const logsData = await getCommitLogs(id!);
      setCommitLogs(logsData.logs || '暂无日志');
    } catch (error: any) {
      message.error(`获取日志失败: ${error.message}`);
    }
  };

  const getCommitProgress = () => {
    if (!commitStatus?.hasJob) return 0;
    switch (commitStatus.status) {
      case 'Pending': return 10;
      case 'Running': return 50;
      case 'Succeeded': return 100;
      case 'Failed': return 100;
      default: return 0;
    }
  };

  if (loading || !pod) {
    return (
      <Layout className="pod-detail-layout">
        <Header className="pod-detail-header glass-header">
          <div className="header-content">
            <Space size="middle">
              <Skeleton.Button active style={{ width: 80 }} />
              <div className="header-title">
                <Skeleton.Input active style={{ width: 200 }} />
              </div>
            </Space>
            <Skeleton.Button active style={{ width: 40 }} />
          </div>
        </Header>
        <Content className="pod-detail-content">
          <GlassCard hover={false} className="detail-card animate-slide-up">
            <Skeleton active paragraph={{ rows: 8 }} />
          </GlassCard>
        </Content>
      </Layout>
    );
  }

  const connections = pod.connections;
  const hasConnections = connections?.ssh?.host && connections?.ssh?.port;

  const tabItems = [
    {
      key: 'overview',
      label: '概览',
      children: (
        <div className="tab-content">
          <GlassCard hover={false} className="info-card">
            <Descriptions bordered column={{ xs: 1, sm: 2 }} size="small">
              <Descriptions.Item label="Pod 名称">{pod.name}</Descriptions.Item>
              <Descriptions.Item label="状态"><StatusBadge status={pod.status} /></Descriptions.Item>
              <Descriptions.Item label="镜像"><Text code className="mono">{pod.image}</Text></Descriptions.Item>
              <Descriptions.Item label="GPU">{pod.gpuType ? `${pod.gpuType} ×${pod.gpuCount}` : '无'}</Descriptions.Item>
              <Descriptions.Item label="CPU / 内存">{pod.cpu || '-'} 核 / {pod.memory || '-'}</Descriptions.Item>
              <Descriptions.Item label="节点 IP">{pod.nodeIP || '-'}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{dayjs(pod.createdAt).format('YYYY-MM-DD HH:mm:ss')}</Descriptions.Item>
            </Descriptions>
          </GlassCard>

          {/* 共用 GPU 的 Pod 信息 */}
          {sharedGPUPods.length > 0 && (
            <GlassCard
              hover={false}
              className="info-card"
              title={
                <Space>
                  <span>共用 GPU 的 Pod</span>
                  <Tag color="warning">{sharedGPUPods.length} 个</Tag>
                </Space>
              }
              extra={
                <Button
                  icon={<ReloadOutlined />}
                  size="small"
                  onClick={loadSharedGPUPods}
                  loading={sharedGPULoading}
                >
                  刷新
                </Button>
              }
              style={{ marginTop: 16 }}
            >
              <Alert
                message="以下 Pod 与当前 Pod 共用 GPU 卡（时分复用模式）"
                type="warning"
                showIcon
                style={{ marginBottom: 16 }}
              />
              <Table
                dataSource={sharedGPUPods}
                rowKey="name"
                pagination={false}
                size="small"
                columns={[
                  {
                    title: 'Pod 名称',
                    dataIndex: 'name',
                    key: 'name',
                    render: (name: string, record: SharedGPUPod) => (
                      <Button type="link" size="small" style={{ padding: 0 }} onClick={() => navigate(`/pod/${record.namespace}/${name}`)}>{name}</Button>
                    ),
                  },
                  {
                    title: '用户',
                    dataIndex: 'user',
                    key: 'user',
                  },
                  {
                    title: '共用的 GPU',
                    dataIndex: 'sharedWith',
                    key: 'sharedWith',
                    render: (gpus: number[]) => (
                      <Space size={4}>
                        {gpus.map(g => (
                          <Tag key={g} color="orange">GPU {g}</Tag>
                        ))}
                      </Space>
                    ),
                  },
                  {
                    title: '创建时间',
                    dataIndex: 'createdAt',
                    key: 'createdAt',
                    width: 150,
                    render: (time: string) => time ? dayjs(time).format('MM-DD HH:mm') : '-',
                  },
                ]}
              />
            </GlassCard>
          )}
        </div>
      ),
    },
    {
      key: 'logs',
      label: '实时日志',
      children: (
        <div className="tab-content">
          <div className="tab-actions">
            <Space>
              <Switch
                checked={autoRefreshLogs}
                onChange={setAutoRefreshLogs}
                checkedChildren="自动刷新"
                unCheckedChildren="手动刷新"
              />
              <Button onClick={loadLogs} loading={logsLoading} icon={<ReloadOutlined />}>刷新日志</Button>
            </Space>
          </div>
          <div className="logs-container">
            <pre className="mono">{logs || '点击刷新按钮加载日志'}</pre>
          </div>
        </div>
      ),
    },
    {
      key: 'describe',
      label: 'Pod 状态',
      children: (
        <div className="tab-content">
          <div className="tab-actions">
            <Button onClick={loadDescribe} loading={describeLoading} icon={<ReloadOutlined />}>刷新状态</Button>
          </div>
          {describe ? (
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <GlassCard hover={false} title="基本信息" size="small">
                <Descriptions bordered column={{ xs: 1, sm: 2 }} size="small">
                  <Descriptions.Item label="名称">{describe.name}</Descriptions.Item>
                  <Descriptions.Item label="命名空间">{describe.namespace}</Descriptions.Item>
                  <Descriptions.Item label="节点">{describe.node || '-'}</Descriptions.Item>
                  <Descriptions.Item label="状态">{describe.status}</Descriptions.Item>
                  <Descriptions.Item label="Pod IP">{describe.ip || '-'}</Descriptions.Item>
                  <Descriptions.Item label="Host IP">{describe.hostIP || '-'}</Descriptions.Item>
                </Descriptions>
              </GlassCard>
              <GlassCard hover={false} title="容器状态" size="small">
                <Table dataSource={describe.containers || []} rowKey="name" pagination={false} size="small"
                  columns={[
                    { title: '名称', dataIndex: 'name', key: 'name' },
                    { title: '状态', dataIndex: 'state', key: 'state', render: (state: string) => <Tag color={state === 'Running' ? 'green' : state === 'Waiting' ? 'orange' : 'red'}>{state}</Tag> },
                    { title: '就绪', dataIndex: 'ready', key: 'ready', render: (ready: boolean) => ready ? 'Yes' : 'No' },
                    { title: '重启', dataIndex: 'restartCount', key: 'restartCount' },
                  ]}
                />
              </GlassCard>
            </Space>
          ) : (
            <Text type="secondary">点击刷新按钮加载 Pod 状态</Text>
          )}
        </div>
      ),
    },
    {
      key: 'commit',
      label: '镜像保存',
      children: (
        <div className="tab-content">
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <Alert message="将当前 Pod 保存为镜像" description="此功能会将 Pod 当前状态打包为一个新镜像，并推送到镜像仓库。" type="info" showIcon />
            <Button type="primary" icon={<SaveOutlined />} onClick={() => setCommitModalVisible(true)}
              disabled={pod.status !== 'Running' || (commitStatus?.hasJob && (commitStatus.status === 'Pending' || commitStatus.status === 'Running'))} size="large">
              保存为镜像
            </Button>
            {pod.status !== 'Running' && <Text type="warning">只有运行中的 Pod 才能保存为镜像</Text>}
          </Space>
          {commitStatus?.hasJob && (
            <GlassCard hover={false} title="当前任务状态" style={{ marginTop: 24 }}>
              <Space direction="vertical" style={{ width: '100%' }}>
                <Descriptions bordered column={2} size="small">
                  <Descriptions.Item label="任务名称">{commitStatus.jobName}</Descriptions.Item>
                  <Descriptions.Item label="状态">
                    <Tag color={commitStatus.status === 'Succeeded' ? 'green' : commitStatus.status === 'Failed' ? 'red' : commitStatus.status === 'Running' ? 'blue' : 'default'}>{commitStatus.status}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="消息" span={2}>{commitStatus.message}</Descriptions.Item>
                </Descriptions>
                <Progress percent={getCommitProgress()} status={commitStatus.status === 'Failed' ? 'exception' : commitStatus.status === 'Succeeded' ? 'success' : 'active'} />
              </Space>
            </GlassCard>
          )}
          {commitStatus?.hasJob && (
            <GlassCard hover={false} title="构建日志" extra={<Button onClick={loadCommitLogs} icon={<ReloadOutlined />} size="small">刷新</Button>} style={{ marginTop: 16 }}>
              <div className="logs-container"><pre className="mono">{commitLogs || '暂无日志'}</pre></div>
            </GlassCard>
          )}
        </div>
      ),
    },
    {
      key: 'events',
      label: '事件',
      children: (
        <div className="tab-content">
          <div className="tab-actions">
            <Button onClick={loadEvents} loading={eventsLoading} icon={<ReloadOutlined />}>刷新事件</Button>
          </div>
          {events.length > 0 ? (
            <Table dataSource={events} rowKey={(record, index) => `${record.reason}-${index}`} pagination={false} size="small"
              columns={[
                { title: '类型', dataIndex: 'type', key: 'type', width: 80, render: (type: string) => <Tag color={type === 'Normal' ? 'blue' : 'red'}>{type}</Tag> },
                { title: '原因', dataIndex: 'reason', key: 'reason', width: 120 },
                { title: '消息', dataIndex: 'message', key: 'message' },
                { title: '次数', dataIndex: 'count', key: 'count', width: 60 },
                { title: '最后发生', dataIndex: 'lastTime', key: 'lastTime', width: 150, render: (time: string) => time ? dayjs(time).format('MM-DD HH:mm:ss') : '-' },
              ]}
            />
          ) : (
            <Text type="secondary">{eventsLoading ? '加载中...' : '点击刷新按钮加载事件'}</Text>
          )}
        </div>
      ),
    },
    {
      key: 'connection',
      label: '连接信息',
      children: (
        <div className="tab-content">
          {!hasConnections ? (
            <div className="connection-empty">
              <Text type="secondary">连接信息暂不可用，请等待 Pod 完全启动后刷新页面</Text>
              <Button onClick={loadPod} style={{ marginTop: 16 }}>刷新</Button>
            </div>
          ) : (
            <Space direction="vertical" size="middle" style={{ width: '100%', maxWidth: 800 }}>
              <GlassCard hover={false} title="SSH 连接信息">
                <Descriptions bordered column={2} size="small">
                  <Descriptions.Item label="主机">{connections.ssh.host}</Descriptions.Item>
                  <Descriptions.Item label="端口">{connections.ssh.port}</Descriptions.Item>
                  <Descriptions.Item label="用户">{connections.ssh.user}</Descriptions.Item>
                  <Descriptions.Item label="密码">
                    <Space>
                      <Input.Password value={connections.ssh.password} visibilityToggle={{ visible: showPassword, onVisibleChange: setShowPassword }} style={{ width: 150 }} readOnly />
                      <Button icon={<CopyOutlined />} size="small" onClick={() => copyToClipboard(connections.ssh.password, '密码')}>复制</Button>
                    </Space>
                  </Descriptions.Item>
                </Descriptions>
              </GlassCard>
              <GlassCard hover={false} title="SSH 命令">
                <div className="command-box">
                  <code className="mono">{connections.apps.sshCommand}</code>
                  <Button icon={<CopyOutlined />} onClick={() => copyToClipboard(connections.apps.sshCommand, 'SSH 命令')}>复制</Button>
                </div>
              </GlassCard>
              <GlassCard hover={false} title="快捷打开">
                <Alert message="提示：如果 VSCode 覆盖了当前项目，请在设置中将 window.openFoldersInNewWindow 设为 on" type="info" showIcon style={{ marginBottom: 16 }} />
                <Space size="large" wrap>
                  <Tooltip title="VSCode Remote SSH"><Button type="primary" icon={<CodeOutlined />} size="large" onClick={() => openVSCode(connections.apps.vscodeURI)}>VSCode</Button></Tooltip>
                  <Tooltip title="下载 Xshell 会话文件"><Button icon={<DownloadOutlined />} size="large" onClick={downloadXshellFile}>Xshell</Button></Tooltip>
                  <Tooltip title="打开 SSH 客户端"><Button icon={<DesktopOutlined />} size="large" onClick={() => openSSHClient(connections.apps.xshellURI, connections.apps.sshCommand)}>SSH 客户端</Button></Tooltip>
                </Space>
              </GlassCard>
            </Space>
          )}
        </div>
      ),
    },
  ];

  return (
    <Layout className="pod-detail-layout">
      <Header className="pod-detail-header glass-header">
        <div className="header-content">
          <Space size="middle">
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/')} className="glass-button">返回</Button>
            <div className="header-title">
              <h2>{pod.name}</h2>
              <StatusBadge status={pod.status} />
            </div>
          </Space>
          <ThemeToggle />
        </div>
      </Header>

      <Content className="pod-detail-content">
        <GlassCard hover={false} className="detail-card animate-slide-up">
          <Tabs defaultActiveKey="overview" items={tabItems} onChange={setActiveTab} />
        </GlassCard>

        <Modal title="保存为镜像" open={commitModalVisible} onCancel={() => setCommitModalVisible(false)} onOk={handleCommit} confirmLoading={commitSubmitting} okText="开始保存" cancelText="取消">
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <div><Text strong>当前镜像：</Text><Text code className="mono">{pod.image}</Text></div>
            <div>
              <Text strong>目标镜像名称：</Text>
              <Input placeholder="registry.example.com/namespace/image:tag" value={commitImageName} onChange={(e) => setCommitImageName(e.target.value)} style={{ marginTop: 8 }} />
              <Text type="secondary" style={{ display: 'block', marginTop: 4 }}>请输入完整的镜像名称，包括仓库地址和标签</Text>
            </div>
            <Alert message="注意" description={<ul style={{ margin: 0, paddingLeft: 20 }}><li>保存过程可能需要几分钟</li><li>确保镜像仓库配置正确且有推送权限</li></ul>} type="warning" showIcon />
          </Space>
        </Modal>
      </Content>
    </Layout>
  );
};

export default PodDetail;



