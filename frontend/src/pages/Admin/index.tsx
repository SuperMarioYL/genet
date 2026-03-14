import { ArrowLeftOutlined, ApartmentOutlined, TeamOutlined } from '@ant-design/icons';
import { Button, Layout, Result, Space, Statistic, Tabs, Typography, message } from 'antd';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import GlassCard from '../../components/GlassCard';
import ThemeToggle from '../../components/ThemeToggle';
import { AdminAPIKeysPanel } from '../AdminAPIKeys/Panel';
import {
  AdminNodePoolItem,
  AdminOverviewResponse,
  AdminUserPoolItem,
  getAdminMe,
  getAdminOverview,
  listAdminNodePools,
  listAdminUserPools,
  updateAdminNodePool,
  updateAdminUserPool,
} from '../../services/api';
import './index.css';

const { Header, Content } = Layout;
const { Text } = Typography;

type PoolType = 'shared' | 'exclusive';
type DragPayload =
  | { kind: 'node'; name: string; poolType: PoolType }
  | { kind: 'user'; name: string; poolType: PoolType };

const AdminPage: React.FC = () => {
  const navigate = useNavigate();
  const dragPayloadRef = useRef<DragPayload | null>(null);
  const [accessLoading, setAccessLoading] = useState(true);
  const [isAdmin, setIsAdmin] = useState(false);
  const [currentUser, setCurrentUser] = useState('');
  const [overview, setOverview] = useState<AdminOverviewResponse | null>(null);
  const [nodes, setNodes] = useState<AdminNodePoolItem[]>([]);
  const [users, setUsers] = useState<AdminUserPoolItem[]>([]);
  const [loading, setLoading] = useState(false);

  const loadData = async () => {
    setLoading(true);
    try {
      const [summary, nodeResp, userResp] = await Promise.all([
        getAdminOverview(),
        listAdminNodePools(),
        listAdminUserPools(),
      ]);
      setOverview(summary);
      setNodes(nodeResp.nodes || []);
      setUsers(userResp.users || []);
    } catch (error: any) {
      message.error(`加载管理员数据失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    getAdminMe()
      .then((me) => {
        setCurrentUser(me.username || me.email || '');
        setIsAdmin(!!me.isAdmin);
        if (me.isAdmin) {
          void loadData();
        }
      })
      .catch((error: any) => {
        setIsAdmin(false);
        message.error(`管理员鉴权失败: ${error.message}`);
      })
      .finally(() => setAccessLoading(false));
  }, []);

  const moveNode = async (nodeName: string, nextPoolType: PoolType) => {
    const previous = nodes;
    setNodes((current) => current.map((item) => (item.nodeName === nodeName ? { ...item, poolType: nextPoolType } : item)));
    try {
      await updateAdminNodePool(nodeName, nextPoolType);
      await loadData();
    } catch (error: any) {
      setNodes(previous);
      message.error(`更新节点池失败: ${error.message}`);
    }
  };

  const moveUser = async (username: string, nextPoolType: PoolType) => {
    const previous = users;
    setUsers((current) => current.map((item) => (item.username === username ? { ...item, poolType: nextPoolType } : item)));
    try {
      await updateAdminUserPool(username, nextPoolType);
      await loadData();
    } catch (error: any) {
      setUsers(previous);
      message.error(`更新用户池失败: ${error.message}`);
    }
  };

  const onDrop = async (targetPoolType: PoolType) => {
    const payload = dragPayloadRef.current;
    dragPayloadRef.current = null;
    if (!payload || payload.poolType === targetPoolType) {
      return;
    }

    if (payload.kind === 'node') {
      await moveNode(payload.name, targetPoolType);
      return;
    }
    await moveUser(payload.name, targetPoolType);
  };

  const sharedNodes = useMemo(() => nodes.filter((item) => item.poolType !== 'exclusive'), [nodes]);
  const exclusiveNodes = useMemo(() => nodes.filter((item) => item.poolType === 'exclusive'), [nodes]);
  const sharedUsers = useMemo(() => users.filter((item) => item.poolType !== 'exclusive'), [users]);
  const exclusiveUsers = useMemo(() => users.filter((item) => item.poolType === 'exclusive'), [users]);

  if (accessLoading) {
    return <div className="admin-page-loading"><Text type="secondary">正在检查管理员权限...</Text></div>;
  }

  if (!isAdmin) {
    return (
      <Result
        status="403"
        title="403"
        subTitle="你没有管理员权限访问该页面。"
        extra={<Button onClick={() => navigate('/')}>返回首页</Button>}
      />
    );
  }

  const renderDropColumn = (
    title: string,
    poolType: PoolType,
    cards: React.ReactNode[],
    description: string,
  ) => (
    <div
      className={`pool-column pool-column-${poolType}`}
      onDragOver={(event) => event.preventDefault()}
      onDrop={(event) => {
        event.preventDefault();
        void onDrop(poolType);
      }}
    >
      <div className="pool-column-header">
        <div>
          <h3>{title}</h3>
          <Text type="secondary">{description}</Text>
        </div>
        <span className={`pool-column-pill pool-column-pill-${poolType}`}>{cards.length}</span>
      </div>
      <div className="pool-column-body">{cards.length > 0 ? cards : <Text type="secondary">暂无内容</Text>}</div>
    </div>
  );

  return (
    <Layout className="admin-page-layout">
      <Header className="admin-page-header">
        <div className="admin-page-header-left">
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/')} className="glass-button">
            返回
          </Button>
          <div>
            <h2>管理员页</h2>
            <Text type="secondary">当前管理员：{currentUser || 'unknown'}</Text>
          </div>
        </div>
        <Space>
          <Button className="glass-button" onClick={loadData} loading={loading}>
            刷新
          </Button>
          <ThemeToggle />
        </Space>
      </Header>

      <Content className="admin-page-content">
        <Tabs
          items={[
            {
              key: 'overview',
              label: '概览',
              children: (
                <div className="admin-overview-grid">
                  <GlassCard hover={false}>
                    <Statistic title="共享池节点" value={overview?.nodeSummary.shared || 0} prefix={<ApartmentOutlined />} />
                  </GlassCard>
                  <GlassCard hover={false}>
                    <Statistic title="独占池节点" value={overview?.nodeSummary.exclusive || 0} prefix={<ApartmentOutlined />} />
                  </GlassCard>
                  <GlassCard hover={false}>
                    <Statistic title="共享池用户" value={overview?.userSummary.shared || 0} prefix={<TeamOutlined />} />
                  </GlassCard>
                  <GlassCard hover={false}>
                    <Statistic title="独占池用户" value={overview?.userSummary.exclusive || 0} prefix={<TeamOutlined />} />
                  </GlassCard>
                </div>
              ),
            },
            {
              key: 'nodes',
              label: '卡池管理',
              children: (
                <div className="pool-grid">
                  {renderDropColumn(
                    '共享池',
                    'shared',
                    sharedNodes.map((node) => (
                      <GlassCard
                        key={node.nodeName}
                        hover={false}
                        className="pool-card"
                      >
                        <div
                          draggable
                          onDragStart={() => {
                            dragPayloadRef.current = { kind: 'node', name: node.nodeName, poolType: 'shared' };
                          }}
                        >
                          <strong>{node.nodeName}</strong>
                          <div className="pool-card-meta">{node.nodeIP || '无 IP'}</div>
                        </div>
                      </GlassCard>
                    )),
                    '拖拽节点到这里后，普通用户只能共享使用。',
                  )}
                  {renderDropColumn(
                    '独占池',
                    'exclusive',
                    exclusiveNodes.map((node) => (
                      <GlassCard key={node.nodeName} hover={false} className="pool-card pool-card-exclusive">
                        <div
                          draggable
                          onDragStart={() => {
                            dragPayloadRef.current = { kind: 'node', name: node.nodeName, poolType: 'exclusive' };
                          }}
                        >
                          <strong>{node.nodeName}</strong>
                          <div className="pool-card-meta">{node.nodeIP || '无 IP'}</div>
                        </div>
                      </GlassCard>
                    )),
                    '拖拽节点到这里后，会被标记为独占池节点。',
                  )}
                </div>
              ),
            },
            {
              key: 'users',
              label: '用户管理',
              children: (
                <div className="pool-grid">
                  {renderDropColumn(
                    '共享池用户',
                    'shared',
                    sharedUsers.map((user) => (
                      <GlassCard key={user.username} hover={false} className="pool-card">
                        <div
                          draggable
                          onDragStart={() => {
                            dragPayloadRef.current = { kind: 'user', name: user.username, poolType: 'shared' };
                          }}
                        >
                          <strong>{user.username}</strong>
                          <div className="pool-card-meta">{user.email || '未记录邮箱'}</div>
                        </div>
                      </GlassCard>
                    )),
                    '拖到共享池后，创建 Pod 时只能调度到共享池。',
                  )}
                  {renderDropColumn(
                    '独占池用户',
                    'exclusive',
                    exclusiveUsers.map((user) => (
                      <GlassCard key={user.username} hover={false} className="pool-card pool-card-exclusive">
                        <div
                          draggable
                          onDragStart={() => {
                            dragPayloadRef.current = { kind: 'user', name: user.username, poolType: 'exclusive' };
                          }}
                        >
                          <strong>{user.username}</strong>
                          <div className="pool-card-meta">{user.email || '未记录邮箱'}</div>
                        </div>
                      </GlassCard>
                    )),
                    '拖到独占池后，创建 Pod 时只能调度到独占池。',
                  )}
                </div>
              ),
            },
            {
              key: 'apikeys',
              label: 'API Key 管理',
              children: <AdminAPIKeysPanel currentUser={currentUser} />,
            },
          ]}
        />
      </Content>
    </Layout>
  );
};

export default AdminPage;
