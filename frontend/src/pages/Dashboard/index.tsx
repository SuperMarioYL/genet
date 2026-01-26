import { CloudDownloadOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Col, Empty, Layout, message, Modal, Row, Space, Statistic, Typography } from 'antd';
import React, { useEffect, useState } from 'react';
import { downloadKubeconfig, getClusterInfo, getKubeconfig, listPods } from '../../services/api';
import GlassCard from '../../components/GlassCard';
import ThemeToggle from '../../components/ThemeToggle';
import CreatePodModal from './CreatePodModal';
import './index.css';
import PodCard from './PodCard';

const { Text, Paragraph } = Typography;
const { Header, Content } = Layout;

const Dashboard: React.FC = () => {
  const [pods, setPods] = useState<any[]>([]);
  const [quota, setQuota] = useState<any>({ podUsed: 0, podLimit: 5, gpuUsed: 0, gpuLimit: 8 });
  const [loading, setLoading] = useState(false);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [kubeconfigModalVisible, setKubeconfigModalVisible] = useState(false);
  const [kubeconfigData, setKubeconfigData] = useState<any>(null);
  const [clusterInfo, setClusterInfo] = useState<any>(null);

  const loadPods = async () => {
    setLoading(true);
    try {
      const data: any = await listPods();
      setPods(data.pods || []);
      setQuota(data.quota || quota);
    } catch (error: any) {
      message.error(`加载 Pod 列表失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadPods();
    getClusterInfo().then((info) => {
      setClusterInfo(info);
    }).catch(() => {});
    const timer = setInterval(loadPods, 30000);
    return () => clearInterval(timer);
  }, []);

  const showKubeconfigButton = clusterInfo?.kubeconfigMode === 'cert' || clusterInfo?.kubeconfigMode === 'oidc';

  const handleShowKubeconfig = async () => {
    try {
      const data = await getKubeconfig();
      setKubeconfigData(data);
      setKubeconfigModalVisible(true);
    } catch (error: any) {
      message.error(`获取 Kubeconfig 失败: ${error.message}`);
    }
  };

  const handleDownloadKubeconfig = () => {
    downloadKubeconfig();
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      message.success('已复制到剪贴板');
    }).catch(() => {
      message.error('复制失败');
    });
  };

  const handleCreateSuccess = () => {
    setCreateModalVisible(false);
    message.success('Pod 创建成功！');
    loadPods();
  };

  return (
    <Layout className="dashboard-layout">
      <Header className="dashboard-header glass-header">
        <div className="header-content">
          <div className="header-brand">
            <div className="brand-icon">
              <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M12 2L2 7L12 12L22 7L12 2Z" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
                <path d="M2 17L12 22L22 17" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
                <path d="M2 12L12 17L22 12" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
              </svg>
            </div>
            <h1 className="brand-title">Genet</h1>
            <span className="brand-subtitle">Pod 管理平台</span>
          </div>
          <Space size="middle">
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setCreateModalVisible(true)}
              className="action-btn"
            >
              创建 Pod
            </Button>
            {showKubeconfigButton && (
              <Button
                icon={<CloudDownloadOutlined />}
                onClick={handleShowKubeconfig}
                className="action-btn glass-button"
              >
                Kubeconfig
              </Button>
            )}
            <Button
              icon={<ReloadOutlined />}
              onClick={loadPods}
              loading={loading}
              className="action-btn glass-button"
            />
            <ThemeToggle />
          </Space>
        </div>
      </Header>

      <Content className="dashboard-content">
        <div className="content-wrapper">
          {/* 配额卡片 */}
          <GlassCard className="quota-card animate-slide-up" hover={false}>
            <Row gutter={[32, 16]} align="middle">
              <Col xs={24} sm={12} md={6}>
                <div className="quota-item">
                  <div className="quota-icon pod-icon">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <rect x="3" y="3" width="18" height="18" rx="2" ry="2"/>
                      <line x1="9" y1="3" x2="9" y2="21"/>
                    </svg>
                  </div>
                  <Statistic
                    title="Pod 使用"
                    value={quota.podUsed}
                    suffix={`/ ${quota.podLimit}`}
                    valueStyle={{ 
                      color: quota.podUsed >= quota.podLimit ? 'var(--error)' : 'var(--success)',
                      fontWeight: 600
                    }}
                  />
                </div>
              </Col>
              <Col xs={24} sm={12} md={6}>
                <div className="quota-item">
                  <div className="quota-icon gpu-icon">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <rect x="4" y="4" width="16" height="16" rx="2"/>
                      <rect x="8" y="8" width="8" height="8"/>
                    </svg>
                  </div>
                  <Statistic
                    title="GPU 使用"
                    value={quota.gpuUsed}
                    suffix={`/ ${quota.gpuLimit}`}
                    valueStyle={{ 
                      color: quota.gpuUsed >= quota.gpuLimit ? 'var(--error)' : 'var(--success)',
                      fontWeight: 600
                    }}
                  />
                </div>
              </Col>
              <Col xs={24} sm={12} md={6}>
                <div className="quota-item">
                  <div className="quota-icon running-icon">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <circle cx="12" cy="12" r="10"/>
                      <polyline points="12,6 12,12 16,14"/>
                    </svg>
                  </div>
                  <Statistic
                    title="运行中"
                    value={pods.filter(p => p.status === 'Running').length}
                    valueStyle={{ color: 'var(--success)', fontWeight: 600 }}
                  />
                </div>
              </Col>
              <Col xs={24} sm={12} md={6}>
                <div className="quota-info">
                  <Text type="secondary" className="quota-notice">
                    ⏰ 所有 Pod 将在 23:00 自动清理
                  </Text>
                </div>
              </Col>
            </Row>
          </GlassCard>

          {/* Pod 列表标题 */}
          <div className="section-header animate-slide-up stagger-1">
            <h2 className="section-title">我的 Pods</h2>
            <Text type="secondary">{pods.length} 个实例</Text>
          </div>

          {/* Pod 列表 */}
          <div className="pods-grid">
            {loading && pods.length === 0 ? (
              <div className="loading-placeholder">
                {[1, 2, 3].map((i) => (
                  <GlassCard key={i} className="skeleton-card" hover={false}>
                    <div className="skeleton-shimmer skeleton-header" />
                    <div className="skeleton-shimmer skeleton-body" />
                    <div className="skeleton-shimmer skeleton-footer" />
                  </GlassCard>
                ))}
              </div>
            ) : pods.length === 0 ? (
              <GlassCard className="empty-card animate-scale-in" hover={false}>
                <Empty
                  image={
                    <div className="empty-icon">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                        <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
                        <polyline points="3.27,6.96 12,12.01 20.73,6.96"/>
                        <line x1="12" y1="22.08" x2="12" y2="12"/>
                      </svg>
                    </div>
                  }
                  description={
                    <span className="empty-text">
                      暂无 Pod，点击上方按钮创建你的第一个开发环境
                    </span>
                  }
                >
                  <Button
                    type="primary"
                    icon={<PlusOutlined />}
                    onClick={() => setCreateModalVisible(true)}
                    size="large"
                  >
                    创建 Pod
                  </Button>
                </Empty>
              </GlassCard>
            ) : (
              <Row gutter={[20, 20]}>
                {pods.map((pod, index) => (
                  <Col key={pod.id} xs={24} sm={24} md={12} lg={8} xl={8}>
                    <div className={`animate-slide-up stagger-${Math.min(index + 2, 6)}`}>
                      <PodCard pod={pod} onUpdate={loadPods} />
                    </div>
                  </Col>
                ))}
              </Row>
            )}
          </div>
        </div>
      </Content>

      {/* 创建 Pod 对话框 */}
      <CreatePodModal
        visible={createModalVisible}
        onCancel={() => setCreateModalVisible(false)}
        onSuccess={handleCreateSuccess}
        currentQuota={quota}
      />

      {/* Kubeconfig 对话框 */}
      <Modal
        title={
          <div className="modal-title-custom">
            <CloudDownloadOutlined />
            <span>Kubeconfig 配置</span>
          </div>
        }
        open={kubeconfigModalVisible}
        onCancel={() => setKubeconfigModalVisible(false)}
        width={800}
        footer={[
          <Button key="close" onClick={() => setKubeconfigModalVisible(false)}>
            关闭
          </Button>,
          <Button key="download" type="primary" icon={<CloudDownloadOutlined />} onClick={handleDownloadKubeconfig}>
            下载 Kubeconfig
          </Button>,
        ]}
      >
        {kubeconfigData && (
          <div className="kubeconfig-content">
            <GlassCard size="small" title="你的 Namespace" hover={false} className="kubeconfig-card">
              <Text code copyable className="mono">{kubeconfigData.namespace}</Text>
            </GlassCard>

            {kubeconfigData.mode === 'oidc' && kubeconfigData.instructions?.installKubelogin && (
              <GlassCard size="small" title="安装 kubelogin" hover={false} className="kubeconfig-card">
                <Paragraph>
                  <Text strong>macOS:</Text>
                  <br />
                  <Text code copyable className="mono">{kubeconfigData.instructions?.installKubelogin?.macOS}</Text>
                </Paragraph>
                <Paragraph>
                  <Text strong>Linux:</Text>
                  <br />
                  <Text code copyable className="mono" style={{ fontSize: 12 }}>{kubeconfigData.instructions?.installKubelogin?.Linux}</Text>
                </Paragraph>
                <Paragraph>
                  <Text strong>Windows:</Text>
                  <br />
                  <Text code copyable className="mono">{kubeconfigData.instructions?.installKubelogin?.Windows}</Text>
                </Paragraph>
              </GlassCard>
            )}

            {kubeconfigData.mode === 'cert' && clusterInfo?.certValidityDays && (
              <GlassCard size="small" title="证书信息" hover={false} className="kubeconfig-card">
                <Text>证书有效期: <Text strong>{clusterInfo.certValidityDays} 天</Text></Text>
                <br />
                <Text type="secondary">证书过期后请重新下载 kubeconfig</Text>
              </GlassCard>
            )}

            <GlassCard size="small" title="使用说明" hover={false} className="kubeconfig-card">
              <ol className="usage-list">
                {kubeconfigData.instructions?.usage?.map((step: string, index: number) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </GlassCard>

            <GlassCard
              size="small"
              title="Kubeconfig 内容"
              extra={<Button size="small" onClick={() => copyToClipboard(kubeconfigData.kubeconfig)}>复制</Button>}
              hover={false}
              className="kubeconfig-card"
            >
              <pre className="kubeconfig-pre mono">
                {kubeconfigData.kubeconfig}
              </pre>
            </GlassCard>
          </div>
        )}
      </Modal>
    </Layout>
  );
};

export default Dashboard;
