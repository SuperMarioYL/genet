import React, { useEffect, useState } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { ConfigProvider, Spin, Result, Button } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { ThemeProvider, useTheme, lightAntdTheme, darkAntdTheme } from './theme';
import ParticleBackground from './components/ParticleBackground';
import Dashboard from './pages/Dashboard';
import PodDetail from './pages/PodDetail';
import { getAuthStatus, AuthStatus } from './services/api';
import './App.css';

// 内部组件，使用主题
const AppContent: React.FC = () => {
  const { mode } = useTheme();
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

  if (loading) {
    return (
      <div className="app-loading">
        <div className="loading-content animate-scale-in">
          <div className="loading-spinner">
            <Spin size="large" />
          </div>
          <p className="loading-text">正在检查认证状态...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="app-error">
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
      </div>
    );
  }

  return (
    <ConfigProvider
      locale={zhCN}
      theme={mode === 'light' ? lightAntdTheme : darkAntdTheme}
    >
      <Router>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/pods/:id" element={<PodDetail />} />
        </Routes>
      </Router>
    </ConfigProvider>
  );
};

// 主 App 组件
function App() {
  return (
    <ThemeProvider>
      <div className="app-container">
        <ParticleBackground />
        <AppContent />
      </div>
    </ThemeProvider>
  );
}

export default App;
