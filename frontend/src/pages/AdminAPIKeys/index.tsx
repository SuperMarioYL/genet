import { ArrowLeftOutlined, CopyOutlined, DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Form, Input, Layout, Modal, Popconfirm, Result, Select, Space, Switch, Table, Tag, Typography, message } from 'antd';
import React, { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import GlassCard from '../../components/GlassCard';
import ThemeToggle from '../../components/ThemeToggle';
import {
  AdminAPIKeyItem,
  createAdminAPIKey,
  deleteAdminAPIKey,
  getAdminMe,
  listAdminAPIKeys,
  updateAdminAPIKey,
} from '../../services/api';
import './index.css';

const { Header, Content } = Layout;
const { Text } = Typography;

const AdminAPIKeys: React.FC = () => {
  const navigate = useNavigate();
  const [accessLoading, setAccessLoading] = useState(true);
  const [isAdmin, setIsAdmin] = useState(false);
  const [currentUser, setCurrentUser] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<AdminAPIKeyItem[]>([]);
  const [createVisible, setCreateVisible] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<AdminAPIKeyItem | null>(null);
  const [createdKey, setCreatedKey] = useState<string>('');
  const [createdKeyVisible, setCreatedKeyVisible] = useState(false);

  const [createForm] = Form.useForm();
  const [editForm] = Form.useForm();

  const loadData = async () => {
    setLoading(true);
    try {
      const data = await listAdminAPIKeys();
      setItems(data.items || []);
    } catch (error: any) {
      message.error(`加载 API Keys 失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

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
      await loadData();
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

  const onCreate = async () => {
    try {
      const values = await createForm.validateFields();
      const payload = {
        name: values.name,
        ownerUser: values.ownerUser,
        scope: values.scope,
        expiresAt: values.expiresAt || undefined,
      };
      const resp = await createAdminAPIKey(payload);
      setCreateVisible(false);
      createForm.resetFields();
      setCreatedKey(resp.plaintextKey);
      setCreatedKeyVisible(true);
      message.success('API Key 创建成功');
      await loadData();
    } catch (error: any) {
      if (error?.errorFields) {
        return;
      }
      message.error(`创建失败: ${error.message}`);
    }
  };

  const openEdit = (item: AdminAPIKeyItem) => {
    setEditingItem(item);
    editForm.setFieldsValue({
      name: item.name,
      ownerUser: item.ownerUser,
      scope: item.scope,
      enabled: item.enabled,
      expiresAt: item.expiresAt || '',
    });
    setEditVisible(true);
  };

  const onUpdate = async () => {
    if (!editingItem) return;
    try {
      const values = await editForm.validateFields();
      await updateAdminAPIKey(editingItem.id, {
        name: values.name,
        ownerUser: values.ownerUser,
        scope: values.scope,
        enabled: values.enabled,
        expiresAt: values.expiresAt || undefined,
      });
      setEditVisible(false);
      setEditingItem(null);
      message.success('更新成功');
      await loadData();
    } catch (error: any) {
      if (error?.errorFields) {
        return;
      }
      message.error(`更新失败: ${error.message}`);
    }
  };

  const onToggleEnabled = async (item: AdminAPIKeyItem, enabled: boolean) => {
    try {
      await updateAdminAPIKey(item.id, { enabled });
      setItems(prev => prev.map(it => (it.id === item.id ? { ...it, enabled } : it)));
    } catch (error: any) {
      message.error(`更新状态失败: ${error.message}`);
    }
  };

  const onDelete = async (item: AdminAPIKeyItem) => {
    try {
      await deleteAdminAPIKey(item.id);
      message.success('已删除');
      await loadData();
    } catch (error: any) {
      message.error(`删除失败: ${error.message}`);
    }
  };

  const columns = useMemo(
    () => [
      {
        title: '名称',
        dataIndex: 'name',
        key: 'name',
      },
      {
        title: '绑定用户',
        dataIndex: 'ownerUser',
        key: 'ownerUser',
      },
      {
        title: '权限',
        dataIndex: 'scope',
        key: 'scope',
        render: (scope: string) => (
          <Tag color={scope === 'write' ? 'volcano' : 'blue'}>
            {scope === 'write' ? 'WRITE' : 'READ'}
          </Tag>
        ),
      },
      {
        title: '启用',
        dataIndex: 'enabled',
        key: 'enabled',
        render: (_: boolean, record: AdminAPIKeyItem) => (
          <Switch checked={record.enabled} onChange={(checked) => onToggleEnabled(record, checked)} />
        ),
      },
      {
        title: 'Key 预览',
        dataIndex: 'keyPreview',
        key: 'keyPreview',
        render: (value: string) => <Text code>{value || '-'}</Text>,
      },
      {
        title: '更新时间',
        dataIndex: 'updatedAt',
        key: 'updatedAt',
        render: (value: string) => <Text type="secondary">{value ? new Date(value).toLocaleString() : '-'}</Text>,
      },
      {
        title: '操作',
        key: 'actions',
        render: (_: any, record: AdminAPIKeyItem) => (
          <Space>
            <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>
              编辑
            </Button>
            <Popconfirm
              title="确定删除该 API Key？"
              okText="删除"
              cancelText="取消"
              onConfirm={() => onDelete(record)}
            >
              <Button size="small" danger icon={<DeleteOutlined />}>
                删除
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []
  );

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      message.success('已复制到剪贴板');
    } catch {
      message.error('复制失败');
    }
  };

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
          <Button icon={<ReloadOutlined />} onClick={loadData} loading={loading} className="glass-button" />
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateVisible(true)}>
            新增 API Key
          </Button>
          <ThemeToggle />
        </Space>
      </Header>

      <Content className="admin-apikeys-content">
        <GlassCard hover={false}>
          <Table
            rowKey="id"
            loading={loading}
            columns={columns}
            dataSource={items}
            pagination={{ pageSize: 10, showSizeChanger: false }}
          />
        </GlassCard>
      </Content>

      <Modal
        title="新增 API Key"
        open={createVisible}
        onCancel={() => setCreateVisible(false)}
        onOk={onCreate}
        okText="创建"
      >
        <Form form={createForm} layout="vertical" initialValues={{ scope: 'read' }}>
          <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="例如：ci-bot" />
          </Form.Item>
          <Form.Item label="绑定用户" name="ownerUser" rules={[{ required: true, message: '请输入绑定用户' }]}>
            <Input placeholder="例如：alice" />
          </Form.Item>
          <Form.Item label="权限" name="scope" rules={[{ required: true, message: '请选择权限' }]}>
            <Select
              options={[
                { label: '只读 (read)', value: 'read' },
                { label: '读写 (write)', value: 'write' },
              ]}
            />
          </Form.Item>
          <Form.Item
            label="过期时间（可选，RFC3339）"
            name="expiresAt"
            tooltip="示例: 2026-12-31T00:00:00Z"
          >
            <Input placeholder="2026-12-31T00:00:00Z" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="编辑 API Key"
        open={editVisible}
        onCancel={() => {
          setEditVisible(false);
          setEditingItem(null);
        }}
        onOk={onUpdate}
        okText="保存"
      >
        <Form form={editForm} layout="vertical">
          <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item label="绑定用户" name="ownerUser" rules={[{ required: true, message: '请输入绑定用户' }]}>
            <Input />
          </Form.Item>
          <Form.Item label="权限" name="scope" rules={[{ required: true, message: '请选择权限' }]}>
            <Select
              options={[
                { label: '只读 (read)', value: 'read' },
                { label: '读写 (write)', value: 'write' },
              ]}
            />
          </Form.Item>
          <Form.Item label="启用" name="enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            label="过期时间（可选，RFC3339）"
            name="expiresAt"
            tooltip="示例: 2026-12-31T00:00:00Z"
          >
            <Input placeholder="2026-12-31T00:00:00Z" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="新 API Key（仅展示一次）"
        open={createdKeyVisible}
        onCancel={() => setCreatedKeyVisible(false)}
        footer={[
          <Button key="copy" icon={<CopyOutlined />} onClick={() => copyToClipboard(createdKey)}>
            复制
          </Button>,
          <Button key="close" type="primary" onClick={() => setCreatedKeyVisible(false)}>
            关闭
          </Button>,
        ]}
      >
        <Text type="secondary">请立即保存，关闭后将无法再次查看明文 Key。</Text>
        <div className="created-key-block">
          <Text code>{createdKey}</Text>
        </div>
      </Modal>
    </Layout>
  );
};

export default AdminAPIKeys;
