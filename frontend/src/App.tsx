import React, { useEffect, useState } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { ConfigProvider, Spin, Result, Button } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import Dashboard from './pages/Dashboard';
import PodDetail from './pages/PodDetail';
import { getAuthStatus, AuthStatus } from './services/api';
import './App.css';

function App() {
  const [loading, setLoading] = useState(true);
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    checkAuth();
  }, []);

  const checkAuth = async () => {
    try {
      const status = await getAuthStatus();
      setAuthStatus(status);
      
      // 如果 OAuth 已启用但未认证，跳转到登录页
      if (status.oauthEnabled && !status.authenticated && status.loginURL) {
        window.location.href = status.loginURL;
        return;
      }
    } catch (err: any) {
      setError(err.message || '认证检查失败');
    } finally {
      setLoading(false);
    }
  };

  // 加载中
  if (loading) {
    return (
      <div style={{ 
        display: 'flex', 
        justifyContent: 'center', 
        alignItems: 'center', 
        height: '100vh' 
      }}>
        <Spin size="large" tip="正在检查认证状态..." />
      </div>
    );
  }

  // 错误状态
  if (error) {
    return (
      <Result
        status="error"
        title="认证失败"
        subTitle={error}
        extra={
          <Button type="primary" onClick={() => window.location.reload()}>
            重试
          </Button>
        }
      />
    );
  }

  return (
    <ConfigProvider locale={zhCN}>
      <Router>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/pods/:id" element={<PodDetail />} />
        </Routes>
      </Router>
    </ConfigProvider>
  );
}

export default App;
