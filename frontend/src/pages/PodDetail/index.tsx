import React, { useState, useEffect, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Layout, Card, Tabs, Button, Space, Descriptions, message, Input, Typography, Divider, Tooltip, Table, Tag, Modal, Progress, Alert } from 'antd';
import { ArrowLeftOutlined, CopyOutlined, EyeOutlined, EyeInvisibleOutlined, CodeOutlined, WindowsOutlined, AppleOutlined, DesktopOutlined, ReloadOutlined, SaveOutlined, CloudUploadOutlined } from '@ant-design/icons';
import { getPod, getPodLogs, getPodEvents, getPodDescribe, commitImage, getCommitStatus, getCommitLogs, CommitStatus } from '../../services/api';
import StatusBadge from '../../components/StatusBadge';
import dayjs from 'dayjs';
import './index.css';

const { Header, Content } = Layout;
const { TabPane } = Tabs;
const { Text, Title } = Typography;

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

  // 镜像 Commit 相关状态
  const [commitModalVisible, setCommitModalVisible] = useState(false);
  const [commitImageName, setCommitImageName] = useState('');
  const [commitStatus, setCommitStatus] = useState<CommitStatus | null>(null);
  const [commitLogs, setCommitLogs] = useState<string>('');
  const [commitSubmitting, setCommitSubmitting] = useState(false);
  const commitPollRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    if (id) {
      loadPod();
      loadCommitStatus();
    }
    return () => {
      // 清理轮询
      if (commitPollRef.current) {
        clearInterval(commitPollRef.current);
      }
    };
  }, [id]);

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

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    message.success(`${label} 已复制`);
  };

  // 检测操作系统
  const detectOS = (): 'windows' | 'mac' | 'linux' => {
    const ua = navigator.userAgent.toLowerCase();
    if (ua.includes('win')) return 'windows';
    if (ua.includes('mac')) return 'mac';
    return 'linux';
  };

  const openApp = (uri: string, appName: string) => {
    window.open(uri, '_blank');
    message.info(`正在打开 ${appName}...`);
  };

  // 打开 Xshell（Windows）
  const openXshell = (xshellURI: string) => {
    // 尝试打开 Xshell 协议
    const link = document.createElement('a');
    link.href = xshellURI;
    link.click();
    message.info('正在打开 Xshell...');
  };

  // 复制 SSH 命令到剪贴板（Mac/Windows Terminal）
  const copySSHCommand = (sshCmd: string, platform: string) => {
    copyToClipboard(sshCmd, 'SSH 命令');
    if (platform === 'mac') {
      message.info('SSH 命令已复制，请在 Terminal.app 中粘贴运行', 3);
    } else if (platform === 'windows') {
      message.info('SSH 命令已复制，请在 PowerShell 或 CMD 中粘贴运行', 3);
    } else {
      message.info('SSH 命令已复制，请在终端中粘贴运行', 3);
    }
  };

  // 加载 commit 状态
  const loadCommitStatus = async () => {
    try {
      const status = await getCommitStatus(id!);
      setCommitStatus(status);
      
      // 如果有进行中的任务，开始轮询
      if (status.hasJob && (status.status === 'Pending' || status.status === 'Running')) {
        startCommitPolling();
      }
    } catch (error) {
      // 忽略错误
    }
  };

  // 开始轮询 commit 状态
  const startCommitPolling = () => {
    if (commitPollRef.current) {
      clearInterval(commitPollRef.current);
    }
    
    commitPollRef.current = setInterval(async () => {
      try {
        const status = await getCommitStatus(id!);
        setCommitStatus(status);
        
        // 同时获取日志
        try {
          const logsData = await getCommitLogs(id!);
          setCommitLogs(logsData.logs || '');
        } catch (e) {
          // 忽略日志错误
        }
        
        // 如果完成或失败，停止轮询
        if (status.status === 'Succeeded' || status.status === 'Failed') {
          if (commitPollRef.current) {
            clearInterval(commitPollRef.current);
            commitPollRef.current = null;
          }
          if (status.status === 'Succeeded') {
            message.success('镜像保存成功！');
          } else {
            message.error('镜像保存失败，请查看日志');
          }
        }
      } catch (error) {
        // 忽略轮询错误
      }
    }, 3000); // 每 3 秒轮询一次
  };

  // 提交 commit 请求
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
      
      // 开始轮询状态
      loadCommitStatus();
      startCommitPolling();
    } catch (error: any) {
      message.error(`创建任务失败: ${error.message}`);
    } finally {
      setCommitSubmitting(false);
    }
  };

  // 刷新 commit 日志
  const loadCommitLogs = async () => {
    try {
      const logsData = await getCommitLogs(id!);
      setCommitLogs(logsData.logs || '暂无日志');
    } catch (error: any) {
      message.error(`获取日志失败: ${error.message}`);
    }
  };

  // 获取 commit 状态对应的 Progress 百分比
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

  // 获取 commit 状态对应的颜色
  const getCommitStatusColor = () => {
    if (!commitStatus?.hasJob) return undefined;
    switch (commitStatus.status) {
      case 'Succeeded': return '#52c41a';
      case 'Failed': return '#ff4d4f';
      default: return undefined;
    }
  };

  if (loading || !pod) {
    return <div style={{ padding: 24 }}>加载中...</div>;
  }

  const connections = pod.connections;
  const hasConnections = connections?.ssh?.host && connections?.ssh?.port;

  return (
    <Layout className="pod-detail-layout">
      <Header className="pod-detail-header">
        <Space>
          <Button
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate('/')}
          >
            返回
          </Button>
          <h2>{pod.name}</h2>
          <StatusBadge status={pod.status} phase={pod.phase} />
        </Space>
      </Header>

      <Content className="pod-detail-content">
        <Card>
          <Tabs defaultActiveKey="overview">
            <TabPane tab="概览" key="overview">
              <Descriptions bordered column={2}>
                <Descriptions.Item label="Pod 名称">{pod.name}</Descriptions.Item>
                <Descriptions.Item label="状态">
                  <StatusBadge status={pod.status} phase={pod.phase} />
                </Descriptions.Item>
                <Descriptions.Item label="镜像">{pod.image}</Descriptions.Item>
                <Descriptions.Item label="GPU 类型">{pod.gpuType || '无 (CPU Only)'}</Descriptions.Item>
                <Descriptions.Item label="GPU 数量">{pod.gpuCount}</Descriptions.Item>
                <Descriptions.Item label="创建时间">
                  {dayjs(pod.createdAt).format('YYYY-MM-DD HH:mm:ss')}
                </Descriptions.Item>
                <Descriptions.Item label="过期时间">
                  {dayjs(pod.expiresAt).format('YYYY-MM-DD HH:mm:ss')}
                </Descriptions.Item>
                <Descriptions.Item label="节点 IP">{pod.nodeIP || '-'}</Descriptions.Item>
              </Descriptions>
            </TabPane>

            <TabPane tab="实时日志" key="logs">
              <div style={{ marginBottom: 16 }}>
                <Button onClick={loadLogs} loading={logsLoading} icon={<ReloadOutlined />}>
                  刷新日志
                </Button>
              </div>
              <div className="logs-container">
                <pre>{logs || '点击刷新按钮加载日志'}</pre>
              </div>
            </TabPane>

            <TabPane tab="Pod 状态" key="describe">
              <div style={{ marginBottom: 16 }}>
                <Button onClick={loadDescribe} loading={describeLoading} icon={<ReloadOutlined />}>
                  刷新状态
                </Button>
              </div>
              {describe ? (
                <div>
                  <Card title="基本信息" style={{ marginBottom: 16 }}>
                    <Descriptions bordered column={2} size="small">
                      <Descriptions.Item label="名称">{describe.name}</Descriptions.Item>
                      <Descriptions.Item label="命名空间">{describe.namespace}</Descriptions.Item>
                      <Descriptions.Item label="节点">{describe.node || '-'}</Descriptions.Item>
                      <Descriptions.Item label="状态">{describe.status}</Descriptions.Item>
                      <Descriptions.Item label="Pod IP">{describe.ip || '-'}</Descriptions.Item>
                      <Descriptions.Item label="Host IP">{describe.hostIP || '-'}</Descriptions.Item>
                      <Descriptions.Item label="启动时间" span={2}>
                        {describe.startTime ? dayjs(describe.startTime).format('YYYY-MM-DD HH:mm:ss') : '-'}
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>

                  <Card title="容器状态" style={{ marginBottom: 16 }}>
                    <Table
                      dataSource={describe.containers || []}
                      rowKey="name"
                      pagination={false}
                      size="small"
                      columns={[
                        { title: '名称', dataIndex: 'name', key: 'name' },
                        { title: '状态', dataIndex: 'state', key: 'state',
                          render: (state: string) => (
                            <Tag color={state === 'Running' ? 'green' : state === 'Waiting' ? 'orange' : 'red'}>
                              {state}
                            </Tag>
                          )
                        },
                        { title: '就绪', dataIndex: 'ready', key: 'ready',
                          render: (ready: boolean) => ready ? 'Yes' : 'No'
                        },
                        { title: '重启次数', dataIndex: 'restartCount', key: 'restartCount' },
                        { title: '原因', dataIndex: 'reason', key: 'reason', render: (v: string) => v || '-' },
                        { title: '消息', dataIndex: 'message', key: 'message', render: (v: string) => v || '-' },
                      ]}
                    />
                  </Card>

                  <Card title="Conditions">
                    <Table
                      dataSource={describe.conditions || []}
                      rowKey="type"
                      pagination={false}
                      size="small"
                      columns={[
                        { title: '类型', dataIndex: 'type', key: 'type' },
                        { title: '状态', dataIndex: 'status', key: 'status',
                          render: (status: string) => (
                            <Tag color={status === 'True' ? 'green' : 'red'}>{status}</Tag>
                          )
                        },
                        { title: '原因', dataIndex: 'reason', key: 'reason', render: (v: string) => v || '-' },
                        { title: '消息', dataIndex: 'message', key: 'message', render: (v: string) => v || '-' },
                      ]}
                    />
                  </Card>
                </div>
              ) : (
                <Text type="secondary">点击刷新按钮加载 Pod 状态</Text>
              )}
            </TabPane>

            <TabPane tab="镜像保存" key="commit">
              <div style={{ marginBottom: 24 }}>
                <Space direction="vertical" style={{ width: '100%' }}>
                  <Alert
                    message="将当前 Pod 保存为镜像"
                    description="此功能会将 Pod 当前状态（包括已安装的软件、配置等）打包为一个新镜像，并推送到镜像仓库。下次可以使用这个镜像创建新的 Pod。"
                    type="info"
                    showIcon
                  />
                  
                  <Button
                    type="primary"
                    icon={<SaveOutlined />}
                    onClick={() => setCommitModalVisible(true)}
                    disabled={pod.status !== 'Running' || (commitStatus?.hasJob && (commitStatus.status === 'Pending' || commitStatus.status === 'Running'))}
                    size="large"
                  >
                    保存为镜像
                  </Button>

                  {pod.status !== 'Running' && (
                    <Text type="warning">只有运行中的 Pod 才能保存为镜像</Text>
                  )}
                </Space>
              </div>

              {/* 当前任务状态 */}
              {commitStatus?.hasJob && (
                <Card title="当前任务状态" style={{ marginBottom: 16 }}>
                  <Space direction="vertical" style={{ width: '100%' }}>
                    <Descriptions bordered column={2} size="small">
                      <Descriptions.Item label="任务名称">{commitStatus.jobName}</Descriptions.Item>
                      <Descriptions.Item label="状态">
                        <Tag color={
                          commitStatus.status === 'Succeeded' ? 'green' :
                          commitStatus.status === 'Failed' ? 'red' :
                          commitStatus.status === 'Running' ? 'blue' : 'default'
                        }>
                          {commitStatus.status}
                        </Tag>
                      </Descriptions.Item>
                      <Descriptions.Item label="消息" span={2}>{commitStatus.message}</Descriptions.Item>
                      <Descriptions.Item label="开始时间">
                        {commitStatus.startTime ? dayjs(commitStatus.startTime).format('YYYY-MM-DD HH:mm:ss') : '-'}
                      </Descriptions.Item>
                      <Descriptions.Item label="结束时间">
                        {commitStatus.endTime ? dayjs(commitStatus.endTime).format('YYYY-MM-DD HH:mm:ss') : '-'}
                      </Descriptions.Item>
                    </Descriptions>
                    
                    <Progress 
                      percent={getCommitProgress()} 
                      status={commitStatus.status === 'Failed' ? 'exception' : commitStatus.status === 'Succeeded' ? 'success' : 'active'}
                      strokeColor={getCommitStatusColor()}
                    />
                  </Space>
                </Card>
              )}

              {/* 任务日志 */}
              {commitStatus?.hasJob && (
                <Card 
                  title="构建日志" 
                  extra={
                    <Button onClick={loadCommitLogs} icon={<ReloadOutlined />} size="small">
                      刷新日志
                    </Button>
                  }
                >
                  <div className="logs-container">
                    <pre>{commitLogs || '暂无日志，点击刷新按钮加载'}</pre>
                  </div>
                </Card>
              )}
            </TabPane>

            <TabPane tab="事件" key="events">
              <div style={{ marginBottom: 16 }}>
                <Button onClick={loadEvents} loading={eventsLoading} icon={<ReloadOutlined />}>
                  刷新事件
                </Button>
              </div>
              {events.length > 0 ? (
                <Table
                  dataSource={events}
                  rowKey={(record, index) => `${record.reason}-${index}`}
                  pagination={false}
                  size="small"
                  columns={[
                    { title: '类型', dataIndex: 'type', key: 'type', width: 80,
                      render: (type: string) => (
                        <Tag color={type === 'Normal' ? 'blue' : 'red'}>{type}</Tag>
                      )
                    },
                    { title: '原因', dataIndex: 'reason', key: 'reason', width: 120 },
                    { title: '消息', dataIndex: 'message', key: 'message' },
                    { title: '次数', dataIndex: 'count', key: 'count', width: 60 },
                    { title: '最后发生', dataIndex: 'lastTime', key: 'lastTime', width: 180,
                      render: (time: string) => time ? dayjs(time).format('MM-DD HH:mm:ss') : '-'
                    },
                  ]}
                />
              ) : (
                <Text type="secondary">
                  {eventsLoading ? '加载中...' : '点击刷新按钮加载事件，或暂无事件'}
                </Text>
              )}
            </TabPane>

            <TabPane tab="连接信息" key="connection">
              {!hasConnections ? (
                <div style={{ padding: 24, textAlign: 'center' }}>
                  <Text type="secondary">
                    连接信息暂不可用，请等待 Pod 完全启动后刷新页面
                  </Text>
                  <br />
                  <Button onClick={loadPod} style={{ marginTop: 16 }}>
                    刷新
                  </Button>
                </div>
              ) : (
                <div className="connection-section">
                  {/* SSH 连接信息 */}
                  <Card title="连接信息" style={{ marginBottom: 16 }}>
                    <Descriptions bordered column={2} size="small">
                      <Descriptions.Item label="主机">{connections.ssh.host}</Descriptions.Item>
                      <Descriptions.Item label="端口">{connections.ssh.port}</Descriptions.Item>
                      <Descriptions.Item label="用户">{connections.ssh.user}</Descriptions.Item>
                      <Descriptions.Item label="密码">
                        <Space>
                          <Input.Password
                            value={connections.ssh.password}
                            visibilityToggle={{
                              visible: showPassword,
                              onVisibleChange: setShowPassword,
                            }}
                            style={{ width: 150 }}
                            readOnly
                          />
                          <Button
                            icon={<CopyOutlined />}
                            size="small"
                            onClick={() => copyToClipboard(connections.ssh.password, '密码')}
                          >
                            复制
                          </Button>
                        </Space>
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>

                  {/* SSH 命令 */}
                  <Card title="SSH 命令" style={{ marginBottom: 16 }}>
                    <div className="command-block">
                      <code style={{ 
                        display: 'block', 
                        padding: '12px', 
                        background: '#f5f5f5', 
                        borderRadius: '4px',
                        fontFamily: 'monospace'
                      }}>
                        {connections.apps.sshCommand}
                      </code>
                      <div style={{ marginTop: 8, textAlign: 'right' }}>
                        <Button
                          icon={<CopyOutlined />}
                          onClick={() => copyToClipboard(connections.apps.sshCommand, 'SSH 命令')}
                        >
                          复制命令
                        </Button>
                      </div>
                    </div>
                  </Card>

                  {/* 快捷打开 */}
                  <Card title="快捷打开">
                    <Space size="large" wrap>
                      <Tooltip title="使用 VSCode Remote SSH 直接打开（新窗口）">
                        <Button
                          type="primary"
                          icon={<CodeOutlined />}
                          size="large"
                          onClick={() => openApp(connections.apps.vscodeURI, 'VSCode')}
                        >
                          VSCode
                        </Button>
                      </Tooltip>
                      
                      {detectOS() === 'windows' && (
                        <Tooltip title="使用 Xshell 打开 SSH 连接">
                          <Button
                            icon={<WindowsOutlined />}
                            size="large"
                            onClick={() => openXshell(connections.apps.xshellURI)}
                          >
                            Xshell
                          </Button>
                        </Tooltip>
                      )}

                      {detectOS() === 'mac' && (
                        <Tooltip title="复制 SSH 命令，在 Terminal.app 中使用">
                          <Button
                            icon={<AppleOutlined />}
                            size="large"
                            onClick={() => copySSHCommand(connections.apps.macTerminalCmd, 'mac')}
                          >
                            Terminal
                          </Button>
                        </Tooltip>
                      )}

                      {detectOS() === 'windows' && (
                        <Tooltip title="复制 SSH 命令，在 PowerShell 或 CMD 中使用">
                          <Button
                            icon={<DesktopOutlined />}
                            size="large"
                            onClick={() => copySSHCommand(connections.apps.winTerminalCmd, 'windows')}
                          >
                            PowerShell
                          </Button>
                        </Tooltip>
                      )}
                    </Space>

                    <Divider />

                    <Title level={5}>VSCode SSH 配置</Title>
                    <Text type="secondary">
                      如果一键打开不工作，请将以下配置添加到 ~/.ssh/config：
                    </Text>
                    <div style={{ marginTop: 12 }}>
                      <pre style={{ 
                        padding: '12px', 
                        background: '#f5f5f5', 
                        borderRadius: '4px',
                        overflow: 'auto'
                      }}>
{`Host ${pod.name}
  HostName ${connections.ssh.host}
  Port ${connections.ssh.port}
  User ${connections.ssh.user}`}
                      </pre>
                      <Button
                        icon={<CopyOutlined />}
                        onClick={() => copyToClipboard(
                          `Host ${pod.name}\n  HostName ${connections.ssh.host}\n  Port ${connections.ssh.port}\n  User ${connections.ssh.user}`,
                          'SSH 配置'
                        )}
                        style={{ marginTop: 8 }}
                      >
                        复制配置
                      </Button>
                    </div>
                  </Card>
                </div>
              )}
            </TabPane>
          </Tabs>
        </Card>

        {/* Commit Modal */}
        <Modal
          title="保存为镜像"
          open={commitModalVisible}
          onCancel={() => setCommitModalVisible(false)}
          onOk={handleCommit}
          confirmLoading={commitSubmitting}
          okText="开始保存"
          cancelText="取消"
        >
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <div>
              <Text strong>当前镜像：</Text>
              <Text code>{pod.image}</Text>
            </div>
            
            <div>
              <Text strong>目标镜像名称：</Text>
              <Input
                placeholder="registry.example.com/namespace/image:tag"
                value={commitImageName}
                onChange={(e) => setCommitImageName(e.target.value)}
                style={{ marginTop: 8 }}
              />
              <Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
                请输入完整的镜像名称，包括仓库地址和标签
              </Text>
            </div>

            <Alert
              message="注意"
              description={
                <ul style={{ margin: 0, paddingLeft: 20 }}>
                  <li>保存过程可能需要几分钟，请耐心等待</li>
                  <li>保存的镜像会包含当前容器的所有修改</li>
                  <li>确保镜像仓库配置正确且有推送权限</li>
                </ul>
              }
              type="warning"
              showIcon
            />
          </Space>
        </Modal>
      </Content>
    </Layout>
  );
};

export default PodDetail;
