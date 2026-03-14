import { CopyOutlined, DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Typography, message } from 'antd';
import React, { useEffect, useState } from 'react';
import GlassCard from '../../components/GlassCard';
import {
  AdminAPIKeyItem,
  createAdminAPIKey,
  deleteAdminAPIKey,
  listAdminAPIKeys,
  updateAdminAPIKey,
} from '../../services/api';

const { Text } = Typography;

interface AdminAPIKeysPanelProps {
  currentUser?: string;
}

export const AdminAPIKeysPanel: React.FC<AdminAPIKeysPanelProps> = ({ currentUser }) => {
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<AdminAPIKeyItem[]>([]);
  const [createVisible, setCreateVisible] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<AdminAPIKeyItem | null>(null);
  const [createdKey, setCreatedKey] = useState('');
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

  useEffect(() => {
    loadData();
  }, []);

  const onCreate = async () => {
    try {
      const values = await createForm.validateFields();
      const resp = await createAdminAPIKey({
        name: values.name,
        ownerUser: values.ownerUser,
        scope: values.scope,
        expiresAt: values.expiresAt || undefined,
      });
      setCreateVisible(false);
      createForm.resetFields();
      setCreatedKey(resp.plaintextKey);
      setCreatedKeyVisible(true);
      await loadData();
    } catch (error: any) {
      if (!error?.errorFields) {
        message.error(`创建失败: ${error.message}`);
      }
    }
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
      await loadData();
    } catch (error: any) {
      if (!error?.errorFields) {
        message.error(`更新失败: ${error.message}`);
      }
    }
  };

  const onToggleEnabled = async (item: AdminAPIKeyItem, enabled: boolean) => {
    try {
      await updateAdminAPIKey(item.id, { enabled });
      setItems((prev) => prev.map((candidate) => (candidate.id === item.id ? { ...candidate, enabled } : candidate)));
    } catch (error: any) {
      message.error(`更新状态失败: ${error.message}`);
    }
  };

  const onDelete = async (item: AdminAPIKeyItem) => {
    try {
      await deleteAdminAPIKey(item.id);
      await loadData();
    } catch (error: any) {
      message.error(`删除失败: ${error.message}`);
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

  const columns = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '绑定用户', dataIndex: 'ownerUser', key: 'ownerUser' },
    {
      title: '权限',
      dataIndex: 'scope',
      key: 'scope',
      render: (scope: string) => <Tag color={scope === 'write' ? 'volcano' : 'blue'}>{scope === 'write' ? 'WRITE' : 'READ'}</Tag>,
    },
    {
      title: '启用',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (_: boolean, record: AdminAPIKeyItem) => <Switch checked={record.enabled} onChange={(checked) => onToggleEnabled(record, checked)} />,
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
          <Popconfirm title="确定删除该 API Key？" okText="删除" cancelText="取消" onConfirm={() => onDelete(record)}>
            <Button size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const copyToClipboard = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      message.success('已复制到剪贴板');
    } catch {
      message.error('复制失败');
    }
  };

  return (
    <>
      <GlassCard hover={false}>
        <div className="admin-apikeys-toolbar">
          <div>
            <Text strong>OpenAPI Key 管理</Text>
            {currentUser ? <div><Text type="secondary">当前管理员：{currentUser}</Text></div> : null}
          </div>
          <Space>
            <Button icon={<ReloadOutlined />} onClick={loadData} loading={loading} className="glass-button" />
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateVisible(true)}>
              新增 API Key
            </Button>
          </Space>
        </div>
        <Table
          rowKey="id"
          loading={loading}
          columns={columns}
          dataSource={items}
          pagination={{ pageSize: 10, showSizeChanger: false }}
        />
      </GlassCard>

      <Modal title="新增 API Key" open={createVisible} onCancel={() => setCreateVisible(false)} onOk={onCreate} okText="创建">
        <Form form={createForm} layout="vertical" initialValues={{ scope: 'read' }}>
          <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="例如：ci-bot" />
          </Form.Item>
          <Form.Item label="绑定用户" name="ownerUser" rules={[{ required: true, message: '请输入绑定用户' }]}>
            <Input placeholder="例如：alice" />
          </Form.Item>
          <Form.Item label="权限" name="scope" rules={[{ required: true, message: '请选择权限' }]}>
            <Select options={[{ label: '只读 (read)', value: 'read' }, { label: '读写 (write)', value: 'write' }]} />
          </Form.Item>
          <Form.Item label="过期时间（可选，RFC3339）" name="expiresAt" tooltip="示例: 2026-12-31T00:00:00Z">
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
            <Select options={[{ label: '只读 (read)', value: 'read' }, { label: '读写 (write)', value: 'write' }]} />
          </Form.Item>
          <Form.Item label="启用" name="enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="过期时间（可选，RFC3339）" name="expiresAt" tooltip="示例: 2026-12-31T00:00:00Z">
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
    </>
  );
};
