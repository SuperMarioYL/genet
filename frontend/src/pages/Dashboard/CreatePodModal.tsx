import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Modal, Form, Select, InputNumber, Input, message, Alert, AutoComplete, Collapse, Typography, Tooltip, Button, Space, Spin } from 'antd';
import { PlusOutlined, SettingOutlined, QuestionCircleOutlined, EnvironmentOutlined, FolderOutlined, DeleteOutlined, DatabaseOutlined, ThunderboltOutlined, AppstoreOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { getConfig, createPod, getGPUOverview, GPUOverviewResponse, NodeGPUInfo, CreatePodRequest, UserMount, StorageVolumeInfo, UserSavedImage, searchRegistryImages, RegistryImageInfo } from '../../services/api';
import GPUSelector from '../../components/GPUSelector';
import { getCleanupLabel } from '../../utils/cleanup';
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

  // 监听 Platform+GPU 类型变化，用于节点过滤和镜像过滤
  const watchedPlatformGPU = Form.useWatch('platformGpuType', form);

  // Registry 镜像搜索相关状态
  const [registryImages, setRegistryImages] = useState<RegistryImageInfo[]>([]);
  const [registrySearchLoading, setRegistrySearchLoading] = useState(false);
  const searchTimerRef = useRef<NodeJS.Timeout | null>(null);

  // 解析 Platform+GPU 类型选择值
  const parseSelectedPlatformGPU = useCallback(() => {
    if (!watchedPlatformGPU || !config?.gpuTypes) return { platform: '', gpuType: '', gpuConfig: null };
    const gpuConfig = config.gpuTypes.find((g: any) => `${g.platform}|${g.name}` === watchedPlatformGPU);
    if (!gpuConfig) return { platform: '', gpuType: '', gpuConfig: null };
    return {
      platform: gpuConfig.platform || '',
      gpuType: gpuConfig.name,
      gpuConfig,
    };
  }, [watchedPlatformGPU, config]);

  const { platform: selectedPlatform, gpuType: watchedGPUType, gpuConfig: selectedGPUConfig } = parseSelectedPlatformGPU();

  // 判断是否为 CPU Only 模式（resourceName 为空）
  const isCPUOnly = selectedGPUConfig && !selectedGPUConfig.resourceName;

  // 高级配置状态
  const [gpuOverview, setGpuOverview] = useState<GPUOverviewResponse | null>(null);
  const [selectedNode, setSelectedNode] = useState<string | undefined>(undefined);
  const [selectedGPUDevices, setSelectedGPUDevices] = useState<number[]>([]);
  const [hasPrometheus, setHasPrometheus] = useState(false);

  // 用户自定义挂载
  const [userMounts, setUserMounts] = useState<UserMount[]>([]);

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
      setUserMounts([]);
      setRegistryImages([]);
    }
    return () => {
      // 清理搜索定时器
      if (searchTimerRef.current) {
        clearTimeout(searchTimerRef.current);
      }
    };
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

      // 设置默认 Platform+GPU 类型
      if (data.gpuTypes && data.gpuTypes.length > 0) {
        const firstGPU = data.gpuTypes[0];
        form.setFieldsValue({ platformGpuType: `${firstGPU.platform}|${firstGPU.name}` });
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

  // Registry 镜像搜索（防抖）
  const handleRegistrySearch = useCallback((keyword: string) => {
    if (searchTimerRef.current) {
      clearTimeout(searchTimerRef.current);
    }

    if (!keyword || keyword.length < 1) {
      setRegistryImages([]);
      return;
    }

    setRegistrySearchLoading(true);
    searchTimerRef.current = setTimeout(async () => {
      try {
        const result = await searchRegistryImages(keyword, 20);
        setRegistryImages(result.images || []);
      } catch (error) {
        console.error('Failed to search registry images:', error);
        message.error('镜像搜索失败，请检查网络或仓库配置');
        setRegistryImages([]);
      } finally {
        setRegistrySearchLoading(false);
      }
    }, 300);
  }, []);

  // 获取过滤后的预设镜像（根据 platform 过滤）
  const getFilteredPresetImages = useCallback(() => {
    if (!config?.presetImages) return [];
    if (!selectedPlatform) return config.presetImages;
    return config.presetImages.filter((img: any) =>
      !img.platform || img.platform === selectedPlatform
    );
  }, [config?.presetImages, selectedPlatform]);

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();

      // CPU Only 模式下不需要 GPU 相关验证
      const effectiveGPUCount = isCPUOnly ? 0 : (values.gpuCount || 0);

      // 共享模式下必须选择节点和 GPU 卡（非 CPU Only 模式）
      if (isSharing && effectiveGPUCount > 0) {
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
        gpuCount: effectiveGPUCount,
        gpuType: watchedGPUType, // 使用解析后的 GPU 类型名称
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

      // CPU Only 模式：GPU 数量为 0，但保留 gpuType 用于 NodeSelector
      if (isCPUOnly) {
        payload.gpuCount = 0;
      } else if (payload.gpuCount === 0) {
        delete payload.gpuType;
      }

      // 处理用户自定义挂载
      if (userMounts.length > 0) {
        payload.userMounts = userMounts.filter(m => m.hostPath && m.mountPath);
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

  // 获取可选择的节点列表（根据选中的 Platform+GPU 类型过滤）
  const getAvailableNodes = (): NodeGPUInfo[] => {
    if (!gpuOverview) return [];

    // 如果没有选中类型或为 CPU Only 模式，返回所有节点
    if (!selectedGPUConfig) {
      const nodes: NodeGPUInfo[] = [];
      for (const group of gpuOverview.acceleratorGroups) {
        nodes.push(...group.nodes);
      }
      return nodes;
    }

    // CPU Only 模式：返回所有节点（K8s 会根据 NodeSelector 过滤）
    if (isCPUOnly) {
      const nodes: NodeGPUInfo[] = [];
      for (const group of gpuOverview.acceleratorGroups) {
        nodes.push(...group.nodes);
      }
      return nodes;
    }

    // GPU 模式：只返回匹配 resourceName 的 AcceleratorGroup 中的节点
    const resourceName = selectedGPUConfig.resourceName;
    for (const group of gpuOverview.acceleratorGroups) {
      if (group.resourceName === resourceName) {
        return group.nodes;
      }
    }

    return [];
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

  // 计算实际 GPU 数量（CPU Only 模式为 0，共享模式由选中卡数决定）
  const effectiveGPUCount = isCPUOnly ? 0 : (
    isSharing && selectedGPUDevices.length > 0
      ? selectedGPUDevices.length
      : selectedGPUCount
  );

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

  // 判断是否需要显示右栏（CPU Only 模式下不需要 GPU 面板）
  const hasGPUPanel = !isCPUOnly && effectiveGPUCount > 0 && gpuOverview && gpuOverview.acceleratorGroups.length > 0;
  const hasStorageVolumes = config?.storageVolumes && config.storageVolumes.length > 0;
  const hasStorageSection = hasStorageVolumes || config?.allowUserMounts;
  const showRightPanel = hasGPUPanel || hasStorageSection;

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
          {/* 左栏 */}
          <div className={showRightPanel ? 'create-pod-left' : ''}>

            {/* ── 计算资源 ── */}
            <div className="form-section-title">
              <ThunderboltOutlined />
              <span>计算资源</span>
            </div>

            <Form.Item
              label="平台 / 计算类型"
              name="platformGpuType"
              rules={[{ required: true, message: '请选择平台和计算类型' }]}
            >
              <Select
                placeholder="选择平台和计算类型"
                onChange={() => {
                  setSelectedNode(undefined);
                  setSelectedGPUDevices([]);
                }}
              >
                {config?.gpuTypes?.map((gpu: any) => (
                  <Select.Option key={`${gpu.platform}|${gpu.name}`} value={`${gpu.platform}|${gpu.name}`}>
                    {gpu.platform} - {gpu.name}
                    {!gpu.resourceName && <Text type="secondary"> (CPU Only)</Text>}
                  </Select.Option>
                ))}
              </Select>
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

            {/* GPU 数量选择（CPU Only 模式和共享模式下隐藏） */}
            {!isCPUOnly && !isSharing && (
              <Form.Item
                label="GPU 数量"
                name="gpuCount"
                rules={[{ required: !isCPUOnly, message: '请选择 GPU 数量' }]}
                help={selectedGPUDevices.length > 0 ? `已选择 ${selectedGPUDevices.length} 张卡` : undefined}
              >
                <InputNumber
                  min={1}
                  max={8}
                  style={{ width: '100%' }}
                  onChange={(value) => {
                    setSelectedGPUCount(value || 1);
                    if (selectedGPUDevices.length > 0 && value !== selectedGPUDevices.length) {
                      setSelectedGPUDevices([]);
                    }
                  }}
                  value={selectedGPUDevices.length > 0 ? selectedGPUDevices.length : selectedGPUCount}
                  disabled={selectedGPUDevices.length > 0}
                />
              </Form.Item>
            )}

            {/* 共享模式显示已选 GPU 数量（非 CPU Only） */}
            {isSharing && !isCPUOnly && (
              <div className="sharing-gpu-count">
                <Text>已选择 GPU: </Text>
                <Text strong>{selectedGPUDevices.length}</Text>
                <Text type="secondary"> 张</Text>
              </div>
            )}

            {/* CPU Only 模式提示 */}
            {isCPUOnly && (
              <Alert
                message="CPU Only 模式"
                description="当前选择的是纯 CPU 类型，Pod 将不使用 GPU 资源"
                type="info"
                showIcon
                style={{ marginBottom: 16 }}
              />
            )}

            {/* ── 镜像与基本信息 ── */}
            <div className="form-section-title">
              <AppstoreOutlined />
              <span>镜像与基本信息</span>
            </div>

            <Form.Item
              label="基础镜像"
              name="image"
              rules={[
                { required: true, message: '请输入镜像' },
                { pattern: /^[a-zA-Z0-9\-_./:]+$/, message: '请输入有效的镜像名称' },
              ]}
              help={config?.ui?.enableCustomImage
                ? (config?.registryUrl ? `支持模糊搜索 ${config.registryUrl} 中的镜像` : '可以从列表选择或输入自定义镜像')
                : '请从预设列表中选择镜像'}
            >
              {config?.ui?.enableCustomImage ? (
                <Spin spinning={registrySearchLoading} size="small">
                  <AutoComplete
                    placeholder={config?.registryUrl ? `输入关键字搜索 ${config.registryUrl} 镜像...` : "输入或选择镜像名称"}
                    onSearch={config?.registryUrl ? handleRegistrySearch : undefined}
                    notFoundContent={registrySearchLoading ? <Spin size="small" tip="搜索中..." /> : null}
                    filterOption={!config?.registryUrl ? (inputValue, option) => {
                      if (!option) return false;
                      const val = (option as any).value;
                      return val ? String(val).toUpperCase().indexOf(inputValue.toUpperCase()) !== -1 : false;
                    } : false}
                    options={[
                      ...(registryImages.length > 0 ? [{
                        label: 'Registry 镜像',
                        options: registryImages.map((img: RegistryImageInfo) => ({
                          value: `${config?.registryUrl}/${img.name}`,
                          label: img.name + (img.description ? ` - ${img.description}` : ''),
                        })),
                      }] : []),
                      ...(config?.userImages?.length ? [{
                        label: '我保存的镜像',
                        options: config.userImages.map((img: UserSavedImage) => ({
                          value: img.image,
                          label: `${img.image.split('/').pop()} (${dayjs(img.savedAt).format('MM-DD HH:mm')})`,
                        })),
                      }] : []),
                      ...(getFilteredPresetImages().length ? [{
                        label: '预设镜像',
                        options: getFilteredPresetImages().map((img: any) => ({
                          value: img.image,
                          label: img.image,
                        })),
                      }] : []),
                    ]}
                  />
                </Spin>
              ) : (
                <Select placeholder="选择镜像" showSearch optionFilterProp="label">
                  {config?.userImages?.length > 0 && (
                    <Select.OptGroup label="我保存的镜像">
                      {config.userImages.map((img: UserSavedImage) => (
                        <Select.Option key={img.image} value={img.image} label={img.image}>
                          {img.image.split('/').pop()} <Text type="secondary">({dayjs(img.savedAt).format('MM-DD HH:mm')})</Text>
                        </Select.Option>
                      ))}
                    </Select.OptGroup>
                  )}
                  <Select.OptGroup label="预设镜像">
                    {getFilteredPresetImages().map((img: any) => (
                      <Select.Option key={img.image} value={img.image} label={img.image}>
                        {img.image}
                      </Select.Option>
                    ))}
                  </Select.OptGroup>
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

            {/* ── 底部信息 ── */}
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
              message={<span style={{ display: 'flex', alignItems: 'center', gap: 8 }}><span>⏰</span> 所有 Pod 将在 {getCleanupLabel(config?.cleanupSchedule, config?.cleanupTimezone) || '定时'} 自动删除</span>}
              type="warning"
              showIcon={false}
              className="time-warning"
            />
          </div>

          {/* 右栏：节点调度 + 存储挂载 */}
          {showRightPanel && (
            <div className="create-pod-right">
              {/* GPU 节点选择 */}
              {hasGPUPanel && (
                isSharing ? (
                  <div className="gpu-selection-panel">
                    <div className="gpu-selection-panel-title">
                      <EnvironmentOutlined />
                      <span>选择节点和 GPU 卡（必填）</span>
                    </div>
                    {renderNodeSelector()}
                    {selectedNode && (hasPrometheus || isSharing) && renderGPUSelector()}
                  </div>
                ) : (
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
                )
              )}

              {/* 存储与挂载 */}
              {hasStorageSection && (
                <div className="storage-mounts-section">
                  <div className="storage-mounts-header">
                    <DatabaseOutlined />
                    <span>存储与挂载</span>
                  </div>

                  {/* 系统已挂载目录 */}
                  {hasStorageVolumes && (
                    <div className="storage-volumes-list">
                      {config.storageVolumes.map((vol: StorageVolumeInfo) => (
                        <div key={vol.name} className="storage-volume-item">
                          <div className="storage-volume-path">
                            <Text code>{vol.mountPath}</Text>
                            {vol.readOnly && <Text type="secondary" style={{ marginLeft: 4 }}>(只读)</Text>}
                          </div>
                          {vol.description && (
                            <div className="storage-volume-desc">
                              <Text type="secondary">{vol.description}</Text>
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}

                  {/* 自定义挂载 */}
                  {config?.allowUserMounts && (
                    <div className="user-mounts-subsection">
                      <div className="user-mounts-header">
                        <FolderOutlined />
                        <span>自定义挂载</span>
                        <Button
                          type="link"
                          size="small"
                          icon={<PlusOutlined />}
                          onClick={() => setUserMounts([...userMounts, { hostPath: '', mountPath: '', readOnly: false }])}
                        >
                          添加
                        </Button>
                      </div>
                      {userMounts.length === 0 ? (
                        <Text type="secondary" className="user-mounts-empty">可挂载宿主机目录到 Pod 中</Text>
                      ) : (
                        <Space direction="vertical" style={{ width: '100%' }} size={8}>
                          {userMounts.map((mount, index) => (
                            <div key={index} className="user-mount-item">
                              <Input
                                placeholder="宿主机路径"
                                value={mount.hostPath}
                                onChange={(e) => {
                                  const newMounts = [...userMounts];
                                  newMounts[index].hostPath = e.target.value;
                                  setUserMounts(newMounts);
                                }}
                                style={{ flex: 1 }}
                              />
                              <Input
                                placeholder="挂载路径"
                                value={mount.mountPath}
                                onChange={(e) => {
                                  const newMounts = [...userMounts];
                                  newMounts[index].mountPath = e.target.value;
                                  setUserMounts(newMounts);
                                }}
                                style={{ flex: 1 }}
                              />
                              <Tooltip title="只读">
                                <Select
                                  value={mount.readOnly ? 'ro' : 'rw'}
                                  onChange={(v) => {
                                    const newMounts = [...userMounts];
                                    newMounts[index].readOnly = v === 'ro';
                                    setUserMounts(newMounts);
                                  }}
                                  style={{ width: 70 }}
                                  size="small"
                                >
                                  <Select.Option value="rw">读写</Select.Option>
                                  <Select.Option value="ro">只读</Select.Option>
                                </Select>
                              </Tooltip>
                              <Button
                                type="text"
                                danger
                                icon={<DeleteOutlined />}
                                onClick={() => setUserMounts(userMounts.filter((_, i) => i !== index))}
                              />
                            </div>
                          ))}
                        </Space>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      </Form>
    </Modal>
  );
};

export default CreatePodModal;
