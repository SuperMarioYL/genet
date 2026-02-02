import React, { useState, useEffect, useCallback } from 'react';
import { Modal, Form, Select, InputNumber, Input, message, Alert, AutoComplete, Collapse, Typography, Tooltip } from 'antd';
import { PlusOutlined, SettingOutlined, QuestionCircleOutlined, EnvironmentOutlined } from '@ant-design/icons';
import { getConfig, createPod, getGPUOverview, GPUOverviewResponse, NodeGPUInfo, CreatePodRequest } from '../../services/api';
import GPUSelector from '../../components/GPUSelector';
import './CreatePodModal.css';

const { Text } = Typography;

interface CreatePodModalProps {
  visible: boolean;
  onCancel: () => void;
  onSuccess: () => void;
  currentQuota: any;
}

const CreatePodModal: React.FC<CreatePodModalProps> = ({
  visible,
  onCancel,
  onSuccess,
  currentQuota,
}) => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<any>(null);
  const [selectedGPUCount, setSelectedGPUCount] = useState(1);

  // 高级配置状态
  const [gpuOverview, setGpuOverview] = useState<GPUOverviewResponse | null>(null);
  const [selectedNode, setSelectedNode] = useState<string | undefined>(undefined);
  const [selectedGPUDevices, setSelectedGPUDevices] = useState<number[]>([]);
  const [hasPrometheus, setHasPrometheus] = useState(false);

  // 获取调度模式
  const isSharing = gpuOverview?.schedulingMode === 'sharing';
  const maxPodsPerGPU = gpuOverview?.maxPodsPerGPU || 0;

  useEffect(() => {
    if (visible) {
      loadConfig();
      loadGPUOverview();
      form.resetFields();
      setSelectedGPUCount(1);
      setSelectedNode(undefined);
      setSelectedGPUDevices([]);
    }
    // form 是 antd useForm 返回的稳定引用，loadConfig/loadGPUOverview 在组件内定义
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible]);

  // 加载 GPU 概览数据（用于高级选择）
  const loadGPUOverview = useCallback(async () => {
    try {
      const data = await getGPUOverview();
      setGpuOverview(data);
      // 直接使用后端返回的 prometheusEnabled 字段
      setHasPrometheus(data.prometheusEnabled);
    } catch (error: any) {
      console.error('Failed to load GPU overview:', error);
      setHasPrometheus(false);
    }
  }, []);

  const loadConfig = async () => {
    try {
      const data: any = await getConfig();
      setConfig(data);
      if (data.presetImages && data.presetImages.length > 0) {
        form.setFieldsValue({ image: data.presetImages[0].image });
      }
      if (data.gpuTypes && data.gpuTypes.length > 0) {
        form.setFieldsValue({ gpuType: data.gpuTypes[0].name });
      }
      form.setFieldsValue({
        gpuCount: 1,
        cpu: data.ui?.defaultCPU || '4',
        memory: data.ui?.defaultMemory || '8Gi',
      });
    } catch (error: any) {
      message.error(`加载配置失败: ${error.message}`);
    }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();

      // 共享模式下必须选择节点和 GPU 卡
      if (isSharing && values.gpuCount > 0) {
        if (!selectedNode) {
          message.error('共享模式下必须选择节点');
          return;
        }
        if (selectedGPUDevices.length === 0) {
          message.error('共享模式下必须选择 GPU 卡');
          return;
        }
      }

      setLoading(true);

      const payload: CreatePodRequest = {
        image: values.image,
        gpuCount: values.gpuCount,
        gpuType: values.gpuType,
        cpu: values.cpu,
        memory: values.memory,
      };

      // 处理自定义名称
      if (values.name && values.name.trim()) {
        payload.name = values.name.trim();
      }

      // 处理高级配置
      if (selectedNode) {
        payload.nodeName = selectedNode;
      }
      if (selectedGPUDevices.length > 0) {
        payload.gpuDevices = selectedGPUDevices;
        // GPU 数量由选择的卡数决定
        payload.gpuCount = selectedGPUDevices.length;
      }

      if (payload.gpuCount === 0) {
        delete payload.gpuType;
      }

      await createPod(payload);

      // 显示创建中状态
      message.loading({
        content: 'Pod 创建中，等待调度...',
        key: 'podCreating',
        duration: 0,
      });

      // 关闭对话框后调用成功回调
      onSuccess();

      // 延迟更新消息（给用户一个视觉反馈过渡）
      setTimeout(() => {
        message.success({ content: 'Pod 创建已提交，请在列表中查看状态', key: 'podCreating', duration: 3 });
      }, 2000);

    } catch (error: any) {
      if (error.errorFields) return;
      message.error(`创建失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const willExceedQuota = () => {
    const newPodCount = currentQuota.podUsed + 1;
    // 如果选择了具体 GPU 卡，使用卡数；否则使用输入的 GPU 数量
    const gpuToAdd = selectedGPUDevices.length > 0 ? selectedGPUDevices.length : selectedGPUCount;
    const newGPUCount = currentQuota.gpuUsed + gpuToAdd;
    return {
      podExceeded: newPodCount > currentQuota.podLimit,
      gpuExceeded: newGPUCount > currentQuota.gpuLimit,
      newPodCount,
      newGPUCount,
    };
  };

  // 获取当前选中节点的信息
  const getSelectedNodeInfo = (): NodeGPUInfo | undefined => {
    if (!selectedNode || !gpuOverview) return undefined;
    for (const group of gpuOverview.acceleratorGroups) {
      const node = group.nodes.find(n => n.nodeName === selectedNode);
      if (node) return node;
    }
    return undefined;
  };

  // 获取可选择的节点列表
  const getAvailableNodes = (): NodeGPUInfo[] => {
    if (!gpuOverview) return [];
    const nodes: NodeGPUInfo[] = [];

    for (const group of gpuOverview.acceleratorGroups) {
      // 显示所有有 GPU 的节点
      nodes.push(...group.nodes);
    }

    return nodes;
  };

  // 处理节点选择变化
  const handleNodeChange = (nodeName: string | undefined) => {
    setSelectedNode(nodeName);
    setSelectedGPUDevices([]); // 切换节点时清空 GPU 选择
  };

  // 处理 GPU 卡选择变化
  const handleGPUDevicesChange = (devices: number[]) => {
    setSelectedGPUDevices(devices);
    // 同步更新 GPU 数量显示
    if (devices.length > 0) {
      setSelectedGPUCount(devices.length);
      // 共享模式下同步表单值
      if (isSharing) {
        form.setFieldsValue({ gpuCount: devices.length });
      }
    }
  };

  // 计算实际 GPU 数量（共享模式由选中卡数决定）
  const effectiveGPUCount = isSharing && selectedGPUDevices.length > 0
    ? selectedGPUDevices.length
    : selectedGPUCount;

  const quotaCheck = willExceedQuota();
  const canCreate = !quotaCheck.podExceeded && !quotaCheck.gpuExceeded;

  // 渲染节点选择器
  const renderNodeSelector = () => (
    <Form.Item
      label={isSharing ? "选择节点 *" : "选择节点"}
      className="node-select-item"
      validateStatus={isSharing && !selectedNode ? 'error' : undefined}
      help={isSharing && !selectedNode ? '共享模式下必须选择节点' : undefined}
    >
      <Select
        placeholder={isSharing ? "请选择节点" : "自动调度（推荐）"}
        allowClear={!isSharing}
        value={selectedNode}
        onChange={handleNodeChange}
        style={{ width: '100%' }}
      >
        {getAvailableNodes().map(node => {
          const freeDevices = node.totalDevices - node.usedDevices;
          const isDisabled = !isSharing && !node.timeSharingEnabled && freeDevices === 0;
          return (
            <Select.Option
              key={node.nodeName}
              value={node.nodeName}
              disabled={isDisabled}
            >
              {node.nodeName}
              <Text type="secondary" style={{ marginLeft: 8 }}>
                ({freeDevices}/{node.totalDevices} 空闲)
                {isSharing && maxPodsPerGPU > 0 && ` [每卡最多 ${maxPodsPerGPU} Pod]`}
                {!isSharing && node.timeSharingEnabled && ' [时分复用]'}
              </Text>
            </Select.Option>
          );
        })}
      </Select>
    </Form.Item>
  );

  // 渲染 GPU 选择器
  const renderGPUSelector = () => {
    const nodeInfo = getSelectedNodeInfo();
    if (!nodeInfo) return null;
    return (
      <Form.Item
        label={isSharing ? "选择 GPU 卡 *" : "选择 GPU 卡"}
        validateStatus={isSharing && selectedGPUDevices.length === 0 ? 'error' : undefined}
        help={isSharing && selectedGPUDevices.length === 0 ? '共享模式下必须选择 GPU 卡' : undefined}
      >
        <GPUSelector
          slots={nodeInfo.slots}
          selectedDevices={selectedGPUDevices}
          timeSharingEnabled={nodeInfo.timeSharingEnabled}
          onChange={handleGPUDevicesChange}
          schedulingMode={gpuOverview?.schedulingMode || 'exclusive'}
          maxPodsPerGPU={maxPodsPerGPU}
        />
      </Form.Item>
    );
  };

  // 判断是否需要显示右栏
  const showRightPanel = effectiveGPUCount > 0 && gpuOverview && gpuOverview.acceleratorGroups.length > 0;

  return (
    <Modal
      title={
        <div className="modal-title-custom">
          <PlusOutlined />
          <span>创建新的 Pod</span>
        </div>
      }
      open={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
      okText="创建 Pod"
      cancelText="取消"
      okButtonProps={{ disabled: !canCreate }}
      width={showRightPanel ? 900 : 600}
      className="create-pod-modal"
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{ gpuCount: 1, cpu: '4', memory: '8Gi' }}
        className="create-pod-form"
      >
        <div className={showRightPanel ? 'create-pod-layout' : ''}>
          {/* 左栏：基础配置 */}
          <div className={showRightPanel ? 'create-pod-left' : ''}>
            <Form.Item
              label="基础镜像"
              name="image"
              rules={[
                { required: true, message: '请输入镜像' },
                { pattern: /^[a-zA-Z0-9\-_./:]+$/, message: '请输入有效的镜像名称' },
              ]}
              help={config?.ui?.enableCustomImage ? '可以从列表选择或输入自定义镜像' : '请从预设列表中选择镜像'}
            >
              {config?.ui?.enableCustomImage ? (
                <AutoComplete
                  placeholder="选择或输入镜像名称"
                  filterOption={(inputValue, option) =>
                    option?.value ? String(option.value).toUpperCase().indexOf(inputValue.toUpperCase()) !== -1 : false
                  }
                  options={config?.presetImages?.map((img: any) => ({
                    value: img.image,
                    label: `${img.name} - ${img.description}`,
                  }))}
                />
              ) : (
                <Select placeholder="选择镜像" showSearch optionFilterProp="children">
                  {config?.presetImages?.map((img: any) => (
                    <Select.Option key={img.image} value={img.image}>
                      {img.name} - {img.description}
                    </Select.Option>
                  ))}
                </Select>
              )}
            </Form.Item>

            <Form.Item
              label={
                <span>
                  Pod 名称
                  <Tooltip title="自定义 Pod 名称后缀，留空则使用时间戳自动生成">
                    <QuestionCircleOutlined style={{ marginLeft: 4, color: '#999' }} />
                  </Tooltip>
                </span>
              }
              name="name"
              rules={[
                {
                  pattern: /^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$/,
                  message: '只能包含小写字母、数字和连字符，不能以连字符开头或结尾',
                },
                { max: 20, message: '最多 20 个字符' },
              ]}
            >
              <Input
                placeholder="例如: train, dev, test（留空自动生成）"
                allowClear
              />
            </Form.Item>

            <div className="form-row">
              <Form.Item
                label="CPU 核数"
                name="cpu"
                rules={[
                  { required: true, message: '请输入 CPU 核数' },
                  { pattern: /^[0-9]+(\.[0-9]+)?$/, message: '请输入有效的数字' },
                ]}
                className="form-col"
              >
                <AutoComplete
                  placeholder="选择或输入"
                  options={(config?.ui?.cpuOptions || ['2', '4', '8', '16']).map((cpu: string) => ({
                    value: cpu,
                    label: `${cpu} 核`,
                  }))}
                />
              </Form.Item>

              <Form.Item
                label="内存大小"
                name="memory"
                rules={[
                  { required: true, message: '请输入内存大小' },
                  { pattern: /^[0-9]+(\.[0-9]+)?(Mi|Gi)$/, message: '格式如 4Gi, 512Mi' },
                ]}
                className="form-col"
              >
                <AutoComplete
                  placeholder="选择或输入"
                  options={(config?.ui?.memoryOptions || ['4Gi', '8Gi', '16Gi', '32Gi']).map((mem: string) => ({
                    value: mem,
                    label: mem,
                  }))}
                />
              </Form.Item>
            </div>

            <div className="form-row">
              {/* 共享模式下隐藏 GPU 数量输入，由选择的卡决定 */}
              {!isSharing && (
                <Form.Item
                  label="GPU 数量"
                  name="gpuCount"
                  rules={[{ required: true, message: '请选择 GPU 数量' }]}
                  help={selectedGPUDevices.length > 0 ? `已选择 ${selectedGPUDevices.length} 张卡` : "设置为 0 可创建纯 CPU Pod"}
                  className="form-col"
                >
                  <InputNumber
                    min={0}
                    max={8}
                    style={{ width: '100%' }}
                    onChange={(value) => {
                      setSelectedGPUCount(value || 0);
                      if (selectedGPUDevices.length > 0 && value !== selectedGPUDevices.length) {
                        setSelectedGPUDevices([]);
                      }
                    }}
                    value={selectedGPUDevices.length > 0 ? selectedGPUDevices.length : selectedGPUCount}
                    disabled={selectedGPUDevices.length > 0}
                  />
                </Form.Item>
              )}

              <Form.Item
                label="GPU 类型"
                name="gpuType"
                rules={[{ required: effectiveGPUCount > 0, message: '请选择 GPU 类型' }]}
                hidden={effectiveGPUCount === 0}
                className="form-col"
              >
                <Select placeholder="选择 GPU 类型" allowClear>
                  {config?.gpuTypes?.map((gpu: any) => (
                    <Select.Option key={gpu.name} value={gpu.name}>{gpu.name}</Select.Option>
                  ))}
                </Select>
              </Form.Item>
            </div>

            {/* 共享模式显示已选 GPU 数量 */}
            {isSharing && (
              <div className="sharing-gpu-count">
                <Text>已选择 GPU: </Text>
                <Text strong>{selectedGPUDevices.length}</Text>
                <Text type="secondary"> 张</Text>
              </div>
            )}

            <div className="quota-preview">
              <div className="quota-preview-title">创建后配额使用</div>
              <div className="quota-preview-items">
                <div className={`quota-preview-item ${quotaCheck.podExceeded ? 'exceeded' : ''}`}>
                  <span className="quota-label">Pod</span>
                  <span className="quota-value">{quotaCheck.newPodCount} / {currentQuota.podLimit}</span>
                </div>
                <div className={`quota-preview-item ${quotaCheck.gpuExceeded ? 'exceeded' : ''}`}>
                  <span className="quota-label">GPU</span>
                  <span className="quota-value">{quotaCheck.newGPUCount} / {currentQuota.gpuLimit}</span>
                </div>
              </div>
            </div>

            <Alert
              message={<span style={{ display: 'flex', alignItems: 'center', gap: 8 }}><span>⏰</span> 所有 Pod 将在今晚 23:00 自动删除</span>}
              type="warning"
              showIcon={false}
              className="time-warning"
            />
          </div>

          {/* 右栏：节点和 GPU 选择 */}
          {showRightPanel && (
            <div className="create-pod-right">
              {isSharing ? (
                // 共享模式：直接显示，不折叠
                <div className="gpu-selection-panel">
                  <div className="gpu-selection-panel-title">
                    <EnvironmentOutlined />
                    <span>选择节点和 GPU 卡（必填）</span>
                  </div>
                  {renderNodeSelector()}
                  {selectedNode && (hasPrometheus || isSharing) && renderGPUSelector()}
                </div>
              ) : (
                // 独占模式：保持 Collapse 折叠
                <Collapse
                  ghost
                  className="advanced-settings-collapse"
                  defaultActiveKey={[]}
                  items={[
                    {
                      key: 'advanced',
                      label: (
                        <span className="advanced-settings-label">
                          <SettingOutlined />
                          <span>高级设置</span>
                          {selectedNode && <Text type="secondary" style={{ marginLeft: 8 }}>已选节点: {selectedNode}</Text>}
                        </span>
                      ),
                      children: (
                        <div className="advanced-settings-content">
                          {renderNodeSelector()}
                          {selectedNode && hasPrometheus && (
                            <div className="gpu-selector-wrapper">
                              {renderGPUSelector()}
                            </div>
                          )}
                          {selectedNode && !hasPrometheus && (
                            <Alert
                              message="未配置 Prometheus，无法选择具体 GPU 卡"
                              type="info"
                              showIcon
                              style={{ marginTop: 12 }}
                            />
                          )}
                        </div>
                      ),
                    },
                  ]}
                />
              )}
            </div>
          )}
        </div>
      </Form>
    </Modal>
  );
};

export default CreatePodModal;
