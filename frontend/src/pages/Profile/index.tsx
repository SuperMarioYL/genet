import { ArrowLeftOutlined, UserOutlined } from '@ant-design/icons';
import { Button, Descriptions, Layout, Result, Space, Tag, Typography } from 'antd';
import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import GlassCard from '../../components/GlassCard';
import ThemeToggle from '../../components/ThemeToggle';
import { AuthStatus, getAuthStatus } from '../../services/api';
import './index.css';

const { Header, Content } = Layout;
const { Text } = Typography;

const ProfilePage: React.FC = () => {
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [profile, setProfile] = useState<AuthStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getAuthStatus()
      .then((resp) => setProfile(resp))
      .catch((err: any) => setError(err.message || '加载个人信息失败'))
      .finally(() => setLoading(false));
  }, []);

  if (!loading && error) {
    return <Result status="error" title="加载失败" subTitle={error} extra={<Button onClick={() => navigate('/')}>返回首页</Button>} />;
  }

  return (
    <Layout className="profile-layout">
      <Header className="profile-header">
        <div className="profile-header-left">
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/')} className="glass-button">
            返回
          </Button>
          <div>
            <h2>个人详情</h2>
            <Text type="secondary">查看当前登录身份与卡池归属</Text>
          </div>
        </div>
        <ThemeToggle />
      </Header>

      <Content className="profile-content">
        <GlassCard className="profile-hero" hover={false}>
          <div className="profile-hero-icon">
            <UserOutlined />
          </div>
          <div>
            <h1>{profile?.username || 'unknown'}</h1>
            <Space size={8} wrap>
              <Tag color={profile?.isAdmin ? 'volcano' : 'blue'}>{profile?.isAdmin ? 'admin' : 'user'}</Tag>
              <Tag color={profile?.poolType === 'exclusive' ? 'orange' : 'green'}>
                {profile?.poolType || 'shared'}
              </Tag>
            </Space>
          </div>
        </GlassCard>

        <GlassCard hover={false}>
          <Descriptions column={1} styles={{ label: { width: 120 } }}>
            <Descriptions.Item label="用户名">{profile?.username || '-'}</Descriptions.Item>
            <Descriptions.Item label="邮箱">{profile?.email || '-'}</Descriptions.Item>
            <Descriptions.Item label="角色">{profile?.isAdmin ? '管理员' : '普通用户'}</Descriptions.Item>
            <Descriptions.Item label="当前卡池">
              <Text code>{profile?.poolType || 'shared'}</Text>
            </Descriptions.Item>
            <Descriptions.Item label="调度规则">
              后续创建 Pod 时，只能调度到当前绑定的{profile?.poolType === 'exclusive' ? '独占池' : '共享池'}节点。
            </Descriptions.Item>
          </Descriptions>
        </GlassCard>
      </Content>
    </Layout>
  );
};

export default ProfilePage;
