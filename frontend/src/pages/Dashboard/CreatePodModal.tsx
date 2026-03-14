import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Modal, Form, Select, InputNumber, Input, message, Alert, AutoComplete, Collapse, Typography, Tooltip, Button, Space, Spin } from 'antd';
import { PlusOutlined, SettingOutlined, QuestionCircleOutlined, EnvironmentOutlined, FolderOutlined, DeleteOutlined, DatabaseOutlined, ThunderboltOutlined, AppstoreOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { getConfig, createDeployment, createPod, createStatefulSet, getGPUOverview, GPUOverviewResponse, NodeGPUInfo, CreateDeploymentRequest, CreatePodRequest, CreateStatefulSetRequest, UserMount, StorageVolumeInfo, UserSavedImage, searchRegistryImages, RegistryImageInfo, getRegistryImageTags } from '../../services/api';
import GPUSelector from '../../components/GPUSelector';
import { getCleanupLabel } from '../../utils/cleanup';
import './CreatePodModal.css';

const { Text } = Typography;

const normalizePoolType = (poolType?: 'shared' | 'exclusive'): 'shared' | 'exclusive' => {
  return poolType === 'exclusive' ? 'exclusive' : 'shared';
};

const getPoolLabel = (poolType?: 'shared' | 'exclusive'): string => {
  return normalizePoolType(poolType) === 'exclusive' ? '独占池' : '共享池';
};

type WorkloadType = 'pod' | 'deployment' | 'statefulset';

const getWorkloadLabel = (workloadType: WorkloadType): string => {
  switch (workloadType) {
    case 'pod':
      return 'Pod';
    case 'statefulset':
      return 'StatefulSet';
    default:
      return 'Deployment';
  }
};

interface CreatePodModalProps {
  visible: boolean;
  isAdmin?: boolean;
  onCancel: () => void;
  onSuccess: () => void;
  currentQuota: any;
}

const CreatePodModal: React.FC<CreatePodModalProps> = ({
  visible,
  isAdmin = false,
  onCancel,
  onSuccess,
  currentQuota,
}) => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [config, setConfig] = useState<any>(null);
  const [selectedGPUCount, setSelectedGPUCount] = useState(1);
  const watchedWorkloadType = (Form.useWatch('workloadType', form) || 'deployment') as WorkloadType;
  const watchedPodCount = Form.useWatch('podCount', form);
  const replicaCount = watchedWorkloadType === 'pod' ? 1 : Math.max(1, Number(watchedPodCount) || 1);
  const allowManualGPUSelection = watchedWorkloadType === 'pod';
  const canUseManualGPUSelection = isAdmin && allowManualGPUSelection;
  const workloadLabel = getWorkloadLabel(watchedWorkloadType);

  // 监听 Platform+GPU 类型变化，用于节点过滤和镜像过滤
  const watchedPlatformGPU = Form.useWatch('platformGpuType', form);

  // Registry 镜像搜索相关状态
  const [registryImages, setRegistryImages] = useState<RegistryImageInfo[]>([]);
  const [registrySearchLoading, setRegistrySearchLoading] = useState(false);
  const searchTimerRef = useRef<NodeJS.Timeout | null>(null);
  const justSelectedRef = useRef(false);

  // Tag 选择相关状态
  const [selectedRegistryImage, setSelectedRegistryImage] = useState<string>('');
  const [imageTags, setImageTags] = useState<string[]>([]);
  const [tagsLoading, setTagsLoading] = useState(false);

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
      setSelectedRegistryImage('');
      setImageTags([]);
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
        workloadType: 'deployment',
        podCount: 1,
        gpuCount: 1,
        cpu: data.ui?.defaultCPU || '4',
        memory: data.ui?.defaultMemory || '8Gi',
        shmSize: data.ui?.defaultShmSize || '1Gi',
      });
    } catch (error: any) {
      message.error(`加载配置失败: ${error.message}`);
    }
  };

  useEffect(() => {
    if (watchedWorkloadType === 'pod') {
      form.setFieldsValue({ podCount: 1 });
    }
    if (watchedWorkloadType !== 'pod' && selectedGPUDevices.length > 0) {
      setSelectedGPUDevices([]);
    }
  }, [form, selectedGPUDevices.length, watchedWorkloadType]);

  // Registry 镜像搜索（防抖）
  const handleRegistrySearch = useCallback((keyword: string) => {
    // 刚刚选中镜像后 AutoComplete 会触发 onSearch，跳过这次搜索
    if (justSelectedRef.current) {
      justSelectedRef.current = false;
      return;
    }

    if (searchTimerRef.current) {
      clearTimeout(searchTimerRef.current);
    }

    if (!keyword || keyword.length < 1) {
      setRegistryImages(prev => prev.length > 0 ? [] : prev);
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

  // 选中镜像后获取 Tags
  const handleImageSelect = useCallback(async (value: string) => {
    // 标记刚刚选中，防止 onSearch 触发不必要的搜索
    justSelectedRef.current = true;
    if (searchTimerRef.current) {
      clearTimeout(searchTimerRef.current);
      searchTimerRef.current = null;
    }

    // 显式设置表单值，防止 onSearch 干扰 Form 的值同步
    form.setFieldsValue({ image: value });

    // 从 value 中提取镜像名（去掉 registryUrl 前缀）
    const registryUrl = config?.registryUrl || '';
    const imageName = registryUrl && value.startsWith(registryUrl + '/')
      ? value.slice(registryUrl.length + 1)
      : '';

    if (!imageName) {
      // 非 Registry 镜像（预设/用户保存的），不获取 tags
      setSelectedRegistryImage('');
      setImageTags([]);
      return;
    }

    // 镜像已包含 tag（如 image:v1.0），不再获取 tags
    if (imageName.includes(':')) {
      setSelectedRegistryImage('');
      setImageTags([]);
      return;
    }

    setSelectedRegistryImage(imageName);
    setImageTags([]);
    setTagsLoading(true);
    try {
      const result = await getRegistryImageTags(imageName, selectedPlatform || undefined);
      setImageTags(result.tags || []);
    } catch (error) {
      console.error('Failed to fetch image tags:', error);
      message.error('获取镜像 Tag 失败');
    } finally {
      setTagsLoading(false);
    }
  }, [config?.registryUrl, form, selectedPlatform]);

  // 获取过滤后的预设镜像（根据 platform 过滤）
  const getFilteredPresetImages = useCallback(() => {
    if (!config?.presetImages) return [];
    if (!selectedPlatform) return config.presetImages;
    return config.presetImages.filter((img: any) =>
      !img.platform || img.platform === selectedPlatform
    );
  }, [config?.presetImages, selectedPlatform]);

  // 与后端 matchPath 规则保持一致：支持 "*" 结尾前缀匹配、精确匹配、目录前缀匹配
  const isPathAllowedForReadWrite = useCallback((path: string): boolean => {
    const targetPath = path.trim();
    if (!targetPath) return false;

    const allowedPaths: string[] = config?.userMountAllowedPaths || [];
    if (allowedPaths.length === 0) return false;

    return allowedPaths.some((pattern) => {
      if (pattern.endsWith('*')) {
        const prefix = pattern.slice(0, -1);
        return targetPath.startsWith(prefix);
      }
      return targetPath === pattern || targetPath.startsWith(`${pattern}/`);
    });
  }, [config]);

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      const workloadType = (values.workloadType || 'deployment') as WorkloadType;
      const requestedReplicas = workloadType === 'pod' ? 1 : Math.max(1, Number(values.podCount) || 1);
      const requestedWorkloadLabel = getWorkloadLabel(workloadType);

      // CPU Only 模式下不需要 GPU 相关验证
      const effectiveGPUCount = isCPUOnly ? 0 : (values.gpuCount || 0);

      setLoading(true);

      // 拼接镜像和 Tag
      let finalImage = values.image;

      // 防御性修复：如果选中了 Registry 镜像但 image 缺少 registry 前缀，补上前缀
      const registryUrl = config?.registryUrl || '';
      if (selectedRegistryImage && registryUrl && !finalImage.startsWith(registryUrl + '/')) {
        finalImage = `${registryUrl}/${selectedRegistryImage}`;
      }

      // 检查镜像名最后一段是否已有 tag（避免端口号中的 : 干扰判断）
      const lastSlashIdx = finalImage.lastIndexOf('/');
      const afterLastSlash = lastSlashIdx >= 0 ? finalImage.slice(lastSlashIdx) : finalImage;
      if (values.imageTag && !afterLastSlash.includes(':')) {
        finalImage = `${finalImage}:${values.imageTag}`;
      }

      const basePayload: CreatePodRequest = {
        image: finalImage,
        gpuCount: effectiveGPUCount,
        gpuType: watchedGPUType, // 使用解析后的 GPU 类型名称
        cpu: values.cpu,
        memory: values.memory,
        shmSize: values.shmSize,
      };

      // 处理自定义名称
      if (values.name && values.name.trim()) {
        basePayload.name = values.name.trim();
      }

      // 处理高级配置
      if (selectedNode) {
        basePayload.nodeName = selectedNode;
      }
      if (canUseManualGPUSelection && selectedGPUDevices.length > 0) {
        basePayload.gpuDevices = selectedGPUDevices;
        // GPU 数量由选择的卡数决定
        basePayload.gpuCount = selectedGPUDevices.length;
      }

      // CPU Only 模式：GPU 数量为 0，但保留 gpuType 用于 NodeSelector
      if (isCPUOnly) {
        basePayload.gpuCount = 0;
      } else if (basePayload.gpuCount === 0) {
        delete basePayload.gpuType;
      }

      // 处理用户自定义挂载
      if (userMounts.length > 0) {
        const validUserMounts = userMounts.filter((m) => m.hostPath && m.mountPath);

        // 前端预校验：只读挂载不限目录；读写挂载必须命中白名单
        const invalidReadWriteMount = validUserMounts.find((m) => !m.readOnly && !isPathAllowedForReadWrite(m.hostPath));
        if (invalidReadWriteMount) {
          message.error(`读写挂载路径不允许: ${invalidReadWriteMount.hostPath}。只读可挂载任意目录，读写仅允许白名单目录。`);
          return;
        }

        basePayload.userMounts = validUserMounts;
      }

      if (workloadType === 'statefulset') {
        const payload: CreateStatefulSetRequest = {
          ...basePayload,
          replicas: requestedReplicas,
        };
        await createStatefulSet(payload);
      } else if (workloadType === 'deployment') {
        const payload: CreateDeploymentRequest = {
          ...basePayload,
          replicas: requestedReplicas,
        };
        await createDeployment(payload);
      } else {
        await createPod(basePayload);
      }

      // 显示创建中状态
      message.loading({
        content: `${requestedWorkloadLabel} 创建中，等待调度...`,
        key: 'podCreating',
        duration: 0,
      });

      // 关闭对话框后调用成功回调
      onSuccess();

      // 延迟更新消息（给用户一个视觉反馈过渡）
      setTimeout(() => {
        message.success({ content: `${requestedWorkloadLabel} 创建已提交，请在列表中查看状态`, key: 'podCreating', duration: 3 });
      }, 2000);

    } catch (error: any) {
      if (error.errorFields) return;
      message.error(`创建失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const willExceedQuota = () => {
    const newPodCount = currentQuota.podUsed + replicaCount;
    // 如果选择了具体 GPU 卡，使用卡数；否则使用输入的 GPU 数量
    const gpuPerReplica = isCPUOnly ? 0 : (canUseManualGPUSelection && selectedGPUDevices.length > 0 ? selectedGPUDevices.length : selectedGPUCount);
    const gpuToAdd = gpuPerReplica * replicaCount;
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
      const nodeMap = new Map<string, NodeGPUInfo>();
      for (const group of gpuOverview.acceleratorGroups) {
        for (const node of group.nodes) {
          if (!nodeMap.has(node.nodeName)) {
            nodeMap.set(node.nodeName, node);
          }
        }
      }
      return Array.from(nodeMap.values());
    }

    // CPU Only 模式：返回所有节点（K8s 会根据 NodeSelector 过滤）
    if (isCPUOnly) {
      const nodeMap = new Map<string, NodeGPUInfo>();
      for (const group of gpuOverview.acceleratorGroups) {
        for (const node of group.nodes) {
          if (!nodeMap.has(node.nodeName)) {
            nodeMap.set(node.nodeName, node);
          }
        }
      }
      return Array.from(nodeMap.values());
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
      if (isSharing && canUseManualGPUSelection) {
        form.setFieldsValue({ gpuCount: devices.length });
      }
    }
  };

  // 计算实际 GPU 数量（CPU Only 模式为 0，共享模式由选中卡数决定）
  const effectiveGPUCount = isCPUOnly ? 0 : (
    canUseManualGPUSelection && isSharing && selectedGPUDevices.length > 0
      ? selectedGPUDevices.length
      : selectedGPUCount
  );

  const quotaCheck = willExceedQuota();
  const canCreate = !quotaCheck.podExceeded && !quotaCheck.gpuExceeded;

  // 渲染节点选择器
  const renderNodeSelector = () => (
    <Form.Item
      label="选择节点（可选）"
      className="node-select-item"
      help={isSharing ? '不选择节点时，将自动选择负载更低的节点' : undefined}
    >
      {(() => {
        const availableNodes = getAvailableNodes();
        const sharedNodes = availableNodes.filter((node) => normalizePoolType(node.poolType) === 'shared');
        const exclusiveNodes = availableNodes.filter((node) => normalizePoolType(node.poolType) === 'exclusive');

        const renderNodeOption = (node: NodeGPUInfo) => {
          const freeDevices = node.totalDevices - node.usedDevices;
          const isDisabled = !isSharing && !node.timeSharingEnabled && freeDevices === 0;
          const poolType = normalizePoolType(node.poolType);

          return (
            <Select.Option
              key={node.nodeName}
              value={node.nodeName}
              label={node.nodeName}
              disabled={isDisabled}
            >
              <div className="node-option-main">
                <span className="node-option-name">{node.nodeName}</span>
                <span className={`node-pool-badge node-pool-badge-${poolType}`}>{getPoolLabel(poolType)}</span>
              </div>
              <div className="node-option-meta">
                {freeDevices}/{node.totalDevices} 空闲
                {isSharing && maxPodsPerGPU > 0 && ` · 每卡最多 ${maxPodsPerGPU} Pod`}
                {!isSharing && node.timeSharingEnabled && ' · 时分复用'}
              </div>
            </Select.Option>
          );
        };

        return (
          <>
            <Select
              placeholder="自动调度（推荐）"
              allowClear
              value={selectedNode}
              onChange={handleNodeChange}
              style={{ width: '100%' }}
              optionLabelProp="label"
            >
              {sharedNodes.length > 0 && (
                <Select.OptGroup
                  key="shared-pool"
                  label={<span className="node-pool-group-label">共享池</span>}
                >
                  {sharedNodes.map(renderNodeOption)}
                </Select.OptGroup>
              )}
              {exclusiveNodes.length > 0 && (
                <Select.OptGroup
                  key="exclusive-pool"
                  label={<span className="node-pool-group-label">独占池</span>}
                >
                  {exclusiveNodes.map(renderNodeOption)}
                </Select.OptGroup>
              )}
            </Select>
            <div className="node-pool-legend">
              <span className="node-pool-legend-item">
                <span className="node-pool-dot node-pool-dot-shared" />
                共享池
              </span>
              <span className="node-pool-legend-item">
                <span className="node-pool-dot node-pool-dot-exclusive" />
                独占池
              </span>
            </div>
          </>
        );
      })()}
    </Form.Item>
  );

  // 渲染 GPU 选择器
  const renderGPUSelector = () => {
    const nodeInfo = getSelectedNodeInfo();
    if (!nodeInfo) return null;
    return (
      <Form.Item
        label="选择 GPU 卡（可选）"
        help={isSharing ? '不选择 GPU 卡时，将按 GPU 数量自动选择热度更低的卡' : undefined}
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
  const readWriteAllowedPaths: string[] = config?.userMountAllowedPaths || [];
  const hasReadWriteAllowedPaths = readWriteAllowedPaths.length > 0;
  const showRightPanel = hasGPUPanel || hasStorageSection;

  return (
    <Modal
      title={
        <div className="modal-title-custom">
          <PlusOutlined />
          <span>创建新的工作负载</span>
        </div>
      }
      open={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
      okText={`创建 ${workloadLabel}`}
      cancelText="取消"
      okButtonProps={{ disabled: !canCreate }}
      width={showRightPanel ? 900 : 600}
      className="create-pod-modal"
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{ workloadType: 'deployment', podCount: 1, gpuCount: 1, cpu: '4', memory: '8Gi', shmSize: '1Gi' }}
        className="create-pod-form"
      >
        <div className={showRightPanel ? 'create-pod-layout' : ''}>
          {/* 左栏 */}
          <div className={showRightPanel ? 'create-pod-left' : ''}>

            <Form.Item
              label="工作负载类型"
              name="workloadType"
              rules={[{ required: true, message: '请选择工作负载类型' }]}
              extra="默认 Deployment；Pod 为单实例，Deployment 和 StatefulSet 支持设置副本数"
            >
              <Select
                onChange={(value: WorkloadType) => {
                  if (value === 'pod') {
                    form.setFieldsValue({ podCount: 1 });
                  }
                  setSelectedGPUDevices([]);
                }}
                options={[
                  { value: 'pod', label: 'Pod' },
                  { value: 'deployment', label: 'Deployment' },
                  { value: 'statefulset', label: 'StatefulSet' },
                ]}
              />
            </Form.Item>

            {/* ── 计算资源 ── */}
            <div className="form-section-title">
              <ThunderboltOutlined />
              <span>计算资源</span>
            </div>

            {watchedWorkloadType !== 'pod' && (
              <Form.Item
                label="Pod 数量"
                name="podCount"
                rules={[
                  { required: true, message: '请输入 Pod 数量' },
                  {
                    validator: (_, value) => {
                      const parsed = Number(value);
                      if (Number.isInteger(parsed) && parsed >= 1 && parsed <= 8) {
                        return Promise.resolve();
                      }
                      return Promise.reject(new Error('请输入 1 到 8 之间的整数'));
                    },
                  },
                ]}
                extra={`${workloadLabel} 将按每副本资源配置创建 ${replicaCount} 个 Pod`}
              >
                <AutoComplete
                  placeholder="选择或输入 Pod 数量"
                  options={[1, 2, 4, 8].map((count) => ({ value: String(count), label: `${count}` }))}
                  onChange={(value) => {
                    const nextCount = Math.max(1, Number(value) || 1);
                    if (nextCount > 1 && selectedGPUDevices.length > 0) {
                      setSelectedGPUDevices([]);
                    }
                  }}
                />
              </Form.Item>
            )}

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

            <Form.Item
              label="共享内存 (/dev/shm)"
              name="shmSize"
              rules={[
                { required: true, message: '请输入共享内存大小' },
                { pattern: /^[0-9]+(\.[0-9]+)?(Mi|Gi|Ti)$/, message: '格式如 1Gi, 512Mi' },
              ]}
            >
              <AutoComplete
                placeholder="选择或输入"
                options={(config?.ui?.shmSizeOptions || ['512Mi', '1Gi', '2Gi', '4Gi']).map((size: string) => ({
                  value: size,
                  label: size,
                }))}
              />
            </Form.Item>

            {/* GPU 数量选择（非 CPU Only） */}
            {!isCPUOnly && (
              <Form.Item
                label="GPU 数量"
                name="gpuCount"
                rules={[{ required: !isCPUOnly, message: '请选择 GPU 数量' }]}
                help={selectedGPUDevices.length > 0
                  ? `已手动选择 ${selectedGPUDevices.length} 张卡`
                  : (isSharing ? '共享模式下可仅填写数量，系统将自动选择节点和卡' : undefined)}
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
                  disabled={selectedGPUDevices.length > 0 && canUseManualGPUSelection}
                />
              </Form.Item>
            )}

            {/* 共享模式调度提示（非 CPU Only） */}
            {isSharing && !isCPUOnly && (
              <div className="sharing-gpu-count">
                {selectedGPUDevices.length > 0 && canUseManualGPUSelection ? (
                  <>
                    <Text>当前模式: </Text>
                    <Text strong>手动指定</Text>
                    <Text type="secondary">（{selectedGPUDevices.length} 张）</Text>
                  </>
                ) : (
                  <>
                    <Text>当前模式: </Text>
                    <Text strong>自动分配</Text>
                    <Text type="secondary">（{selectedGPUCount} 张）</Text>
                  </>
                )}
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
              extra={config?.ui?.enableCustomImage
                ? (config?.registryUrl ? `可选择预设镜像，或输入关键字搜索 ${config.registryUrl} 中的镜像` : '可以从列表选择或输入自定义镜像')
                : '请从预设列表中选择镜像'}
            >
              {config?.ui?.enableCustomImage ? (
                <Spin spinning={registrySearchLoading} size="small">
                  <AutoComplete
                    placeholder={config?.registryUrl ? `点击选择预设镜像，或输入关键字搜索 ${config.registryUrl}` : "输入或选择镜像名称"}
                    onSearch={config?.registryUrl ? handleRegistrySearch : undefined}
                    onSelect={config?.registryUrl ? handleImageSelect : undefined}
                    notFoundContent={registrySearchLoading ? <Spin size="small" tip="搜索中..." /> : null}
                    defaultActiveFirstOption={false}
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

            {selectedRegistryImage && (
              <Form.Item
                label="镜像 Tag"
                name="imageTag"
                help="选择镜像版本 Tag，不选则使用 latest"
              >
                <Select
                  placeholder={tagsLoading ? "加载 Tags 中..." : "选择 Tag（可选）"}
                  loading={tagsLoading}
                  allowClear
                  showSearch
                  optionFilterProp="label"
                  notFoundContent={tagsLoading ? <Spin size="small" /> : "暂无 Tag"}
                  options={imageTags.map(tag => ({ value: tag, label: tag }))}
                />
              </Form.Item>
            )}

            <Form.Item
              label={
                <span>
                  {workloadLabel} 名称
                  <Tooltip title={`自定义 ${workloadLabel} 名称后缀，留空则使用时间戳自动生成`}>
                    <QuestionCircleOutlined style={{ marginLeft: 4, color: '#999' }} />
                  </Tooltip>
                </span>
              }
              name="name"
              rules={[
                {
                  pattern: /^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$/,
                  message: '仅支持a~z、0～9 和"-"，且不能以"-"开头或结尾',
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
              message={<span style={{ display: 'flex', alignItems: 'center', gap: 8 }}><span>⏰</span> 所有工作负载将在 {getCleanupLabel(config?.cleanupSchedule, config?.cleanupTimezone) || '定时'} 自动删除</span>}
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
                      <span>节点与 GPU 卡（选填）</span>
                    </div>
                    <Alert
                      message={canUseManualGPUSelection ? '可选手动指定；留空时将按热力图负载自动分配' : `${workloadLabel} 模式下可固定节点，GPU 卡将自动分配`}
                      type="info"
                      showIcon
                      style={{ marginBottom: 12 }}
                    />
                    {renderNodeSelector()}
                    {canUseManualGPUSelection && selectedNode && (hasPrometheus || isSharing) && renderGPUSelector()}
                    {!canUseManualGPUSelection && (
                      <Alert
                        message={`${workloadLabel} 不支持手动指定具体 GPU 卡`}
                        type="info"
                        showIcon
                        style={{ marginTop: 12 }}
                      />
                    )}
                  </div>
                ) : canUseManualGPUSelection ? (
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
                            {canUseManualGPUSelection && selectedNode && hasPrometheus && (
                              <div className="gpu-selector-wrapper">
                                {renderGPUSelector()}
                              </div>
                            )}
                            {canUseManualGPUSelection && selectedNode && !hasPrometheus && (
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
                ) : (
                  <div className="gpu-selection-panel">
                    <div className="gpu-selection-panel-title">
                      <EnvironmentOutlined />
                      <span>节点调度</span>
                    </div>
                    <Alert
                      message={watchedWorkloadType === 'pod' ? '当前权限不支持手动指定 GPU 卡，可固定节点或使用自动调度' : `${workloadLabel} 支持固定节点，但不支持手动指定具体 GPU 卡`}
                      type="info"
                      showIcon
                      style={{ marginBottom: 12 }}
                    />
                    {renderNodeSelector()}
                  </div>
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
                          onClick={() => setUserMounts([...userMounts, { hostPath: '', mountPath: '', readOnly: true }])}
                        >
                          添加
                        </Button>
                      </div>
                      <Alert
                        message={hasReadWriteAllowedPaths
                          ? '只读挂载可使用任意绝对路径；读写挂载仅允许白名单目录。'
                          : '只读挂载可使用任意绝对路径；当前未配置读写白名单，仅可使用只读挂载。'}
                        type={hasReadWriteAllowedPaths ? 'info' : 'warning'}
                        showIcon
                        style={{ marginBottom: 8 }}
                      />
                      {hasReadWriteAllowedPaths && (
                        <div className="user-mounts-allowed-paths">
                          <Text type="secondary">读写白名单目录: </Text>
                          {readWriteAllowedPaths.map((p: string, i: number) => (
                            <Text code key={i}>{p}</Text>
                          ))}
                        </div>
                      )}
                      {userMounts.length === 0 ? (
                        <Text type="secondary" className="user-mounts-empty">可将宿主机目录挂载到 Pod（默认只读）</Text>
                      ) : (
                        <Space direction="vertical" style={{ width: '100%' }} size={8}>
                          {userMounts.map((mount, index) => (
                            <div key={index}>
                              <div className="user-mount-item">
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
                                <Tooltip title="挂载权限">
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
                                    <Select.Option value="rw" disabled={!hasReadWriteAllowedPaths}>读写</Select.Option>
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
                              {!mount.readOnly && mount.hostPath && !isPathAllowedForReadWrite(mount.hostPath) && (
                                <Text type="danger" style={{ fontSize: 12 }}>
                                  当前路径不在读写白名单中，请改为只读或使用白名单目录。
                                </Text>
                              )}
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
