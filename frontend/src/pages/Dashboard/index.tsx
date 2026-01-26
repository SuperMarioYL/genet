import { CloudDownloadOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Card, Col, Empty, Layout, message, Modal, Row, Space, Statistic, Typography } from 'antd';
import React, { useEffect, useState } from 'react';
import { downloadKubeconfig, getClusterInfo, getKubeconfig, listPods } from '../../services/api';
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
    // 获取集群信息
    getClusterInfo().then((info) => {
      setClusterInfo(info);
    }).catch(() => {});
    // 每 30 秒自动刷新
    const timer = setInterval(loadPods, 30000);
    return () => clearInterval(timer);
  }, []);

  // 判断是否显示 kubeconfig 按钮
  // 当 kubeconfigMode 为 "cert" 或 "oidc" 时显示
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
      <Header className="dashboard-header">
        <div className="header-content">
          <h1>Genet Pod 申请工具</h1>
          <Space>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setCreateModalVisible(true)}
            >
              创建新 Pod
            </Button>
            {showKubeconfigButton && (
              <Button
                icon={<CloudDownloadOutlined />}
                onClick={handleShowKubeconfig}
              >
                获取 Kubeconfig
              </Button>
            )}
            <Button
              icon={<ReloadOutlined />}
              onClick={loadPods}
              loading={loading}
            >
              刷新
            </Button>
          </Space>
        </div>
      </Header>

      <Content className="dashboard-content">
        <div className="content-wrapper">
          {/* 配额展示 */}
          <Card className="quota-card">
            <Row gutter={16}>
              <Col span={12}>
                <Statistic
                  title="Pod 配额"
                  value={quota.podUsed}
                  suffix={`/ ${quota.podLimit}`}
                  valueStyle={{ color: quota.podUsed >= quota.podLimit ? '#ff4d4f' : '#3f8600' }}
                />
              </Col>
              <Col span={12}>
                <Statistic
                  title="GPU 配额"
                  value={quota.gpuUsed}
                  suffix={`/ ${quota.gpuLimit}`}
                  valueStyle={{ color: quota.gpuUsed >= quota.gpuLimit ? '#ff4d4f' : '#3f8600' }}
                />
              </Col>
            </Row>
          </Card>

          {/* Pod 列表 */}
          <div className="pods-section">
            <h2>我的 Pods</h2>
            {loading && pods.length === 0 ? (
              <div className="loading-container">
                <p>加载中...</p>
              </div>
            ) : pods.length === 0 ? (
              <Empty
                description="暂无 Pod，点击上方按钮创建"
                style={{ margin: '40px 0' }}
              />
            ) : (
              <Row gutter={[16, 16]}>
                {pods.map((pod) => (
                  <Col key={pod.id} xs={24} sm={24} md={12} lg={8} xl={8}>
                    <PodCard pod={pod} onUpdate={loadPods} />
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
        title="Kubeconfig 配置"
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
          <div>
            <Card size="small" title="你的 Namespace" style={{ marginBottom: 16 }}>
              <Text code copyable>{kubeconfigData.namespace}</Text>
            </Card>

            {/* 仅 OIDC 模式显示 kubelogin 安装说明 */}
            {kubeconfigData.mode === 'oidc' && kubeconfigData.instructions?.installKubelogin && (
              <Card size="small" title="安装 kubelogin" style={{ marginBottom: 16 }}>
                <Paragraph>
                  <Text strong>macOS:</Text>
                  <br />
                  <Text code copyable>{kubeconfigData.instructions?.installKubelogin?.macOS}</Text>
                </Paragraph>
                <Paragraph>
                  <Text strong>Linux:</Text>
                  <br />
                  <Text code copyable style={{ fontSize: 12 }}>{kubeconfigData.instructions?.installKubelogin?.Linux}</Text>
                </Paragraph>
                <Paragraph>
                  <Text strong>Windows:</Text>
                  <br />
                  <Text code copyable>{kubeconfigData.instructions?.installKubelogin?.Windows}</Text>
                </Paragraph>
              </Card>
            )}

            {/* 证书模式显示有效期信息 */}
            {kubeconfigData.mode === 'cert' && clusterInfo?.certValidityDays && (
              <Card size="small" title="证书信息" style={{ marginBottom: 16 }}>
                <Text>证书有效期: <Text strong>{clusterInfo.certValidityDays} 天</Text></Text>
                <br />
                <Text type="secondary">证书过期后请重新下载 kubeconfig</Text>
              </Card>
            )}

            <Card size="small" title="使用说明" style={{ marginBottom: 16 }}>
              <ol style={{ paddingLeft: 20, margin: 0 }}>
                {kubeconfigData.instructions?.usage?.map((step: string, index: number) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </Card>

            <Card 
              size="small" 
              title="Kubeconfig 内容" 
              extra={<Button size="small" onClick={() => copyToClipboard(kubeconfigData.kubeconfig)}>复制</Button>}
            >
              <pre style={{ 
                background: '#f5f5f5', 
                padding: 12, 
                borderRadius: 4, 
                overflow: 'auto', 
                maxHeight: 300,
                fontSize: 12,
                margin: 0
              }}>
                {kubeconfigData.kubeconfig}
              </pre>
            </Card>
          </div>
        )}
      </Modal>
    </Layout>
  );
};

export default Dashboard;
