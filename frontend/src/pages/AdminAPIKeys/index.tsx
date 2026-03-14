import { ArrowLeftOutlined } from '@ant-design/icons';
import { Button, Layout, Result, Space, Typography, message } from 'antd';
import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import ThemeToggle from '../../components/ThemeToggle';
import { getAdminMe } from '../../services/api';
import { AdminAPIKeysPanel } from './Panel';
import './index.css';

const { Header, Content } = Layout;
const { Text } = Typography;

const AdminAPIKeys: React.FC = () => {
  const navigate = useNavigate();
  const [accessLoading, setAccessLoading] = useState(true);
  const [isAdmin, setIsAdmin] = useState(false);
  const [currentUser, setCurrentUser] = useState<string>('');

  const checkAccess = async () => {
    setAccessLoading(true);
    try {
      const me = await getAdminMe();
      setCurrentUser(me.username || me.email || '');
      if (!me.isAdmin) {
        setIsAdmin(false);
        return;
      }
      setIsAdmin(true);
    } catch (error: any) {
      setIsAdmin(false);
      message.error(`管理员鉴权失败: ${error.message}`);
    } finally {
      setAccessLoading(false);
    }
  };

  useEffect(() => {
    checkAccess();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (accessLoading) {
    return (
      <div className="admin-apikeys-loading">
        <Text type="secondary">正在检查管理员权限...</Text>
      </div>
    );
  }

  if (!isAdmin) {
    return (
      <Result
        status="403"
        title="403"
        subTitle="你没有管理员权限访问该页面。"
        extra={
          <Button type="primary" onClick={() => navigate('/')}>
            返回首页
          </Button>
        }
      />
    );
  }

  return (
    <Layout className="admin-apikeys-layout">
      <Header className="admin-apikeys-header">
        <div className="admin-apikeys-header-left">
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/')} className="glass-button">
            返回
          </Button>
          <div>
            <h2>OpenAPI Key 管理</h2>
            <Text type="secondary">当前管理员：{currentUser || 'unknown'}</Text>
          </div>
        </div>
        <Space>
          <ThemeToggle />
        </Space>
      </Header>

      <Content className="admin-apikeys-content">
        <AdminAPIKeysPanel currentUser={currentUser} />
      </Content>
    </Layout>
  );
};

export default AdminAPIKeys;
