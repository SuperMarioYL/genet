import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Card, Col, Empty, Layout, message, Row, Space, Statistic } from 'antd';
import React, { useEffect, useState } from 'react';
import { listPods } from '../../services/api';
import CreatePodModal from './CreatePodModal';
import './index.css';
import PodCard from './PodCard';

const { Header, Content } = Layout;

const Dashboard: React.FC = () => {
  const [pods, setPods] = useState<any[]>([]);
  const [quota, setQuota] = useState<any>({ podUsed: 0, podLimit: 5, gpuUsed: 0, gpuLimit: 8 });
  const [loading, setLoading] = useState(false);
  const [createModalVisible, setCreateModalVisible] = useState(false);

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
    // 每 30 秒自动刷新
    const timer = setInterval(loadPods, 30000);
    return () => clearInterval(timer);
  }, []);

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
    </Layout>
  );
};

export default Dashboard;

