import axios from 'axios';

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  withCredentials: true, // 携带 cookies 用于 session 认证
});

// 请求拦截器
api.interceptors.request.use(
  (config) => {
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 响应拦截器
api.interceptors.response.use(
  (response) => {
    return response.data;
  },
  (error) => {
    // 安全地提取错误信息
    let message = '请求失败';

    if (error.response) {
      // 服务器返回了错误响应
      const data = error.response.data;
      if (data && typeof data === 'object' && 'error' in data) {
        message = String(data.error);
      } else if (typeof data === 'string' && data.length > 0) {
        message = data;
      } else {
        message = `服务器错误 (${error.response.status})`;
      }
    } else if (error.request) {
      // 请求已发送但无响应
      message = '网络连接失败，请检查网络';
    } else if (error.message) {
      message = error.message;
    }

    return Promise.reject(new Error(message));
  }
);

// 认证相关
export interface AuthStatus {
  authenticated: boolean;
  username?: string;
  email?: string;
  isAdmin?: boolean;
  poolType?: 'shared' | 'exclusive';
  oauthEnabled: boolean;
  loginURL?: string;
}

export const getAuthStatus = (): Promise<AuthStatus> => {
  return api.get('/auth/status');
};

export const logout = () => {
  return api.get('/auth/logout');
};

export interface AdminMeResponse {
  username: string;
  email: string;
  isAdmin: boolean;
}

export interface AdminAPIKeyItem {
  id: string;
  name: string;
  ownerUser: string;
  scope: 'read' | 'write';
  enabled: boolean;
  keyPreview?: string;
  expiresAt?: string;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
}

export interface AdminAPIKeyListResponse {
  items: AdminAPIKeyItem[];
}

export interface AdminOverviewResponse {
  nodeSummary: {
    shared: number;
    exclusive: number;
  };
  userSummary: {
    shared: number;
    exclusive: number;
  };
}

export interface AdminNodePoolItem {
  nodeName: string;
  nodeIP: string;
  poolType: 'shared' | 'exclusive';
}

export interface AdminNodePoolListResponse {
  nodes: AdminNodePoolItem[];
}

export interface AdminUserPoolItem {
  username: string;
  email?: string;
  poolType: 'shared' | 'exclusive';
  updatedAt?: string;
  updatedBy?: string;
}

export interface AdminUserPoolListResponse {
  users: AdminUserPoolItem[];
}

export interface CreateAdminAPIKeyRequest {
  name: string;
  ownerUser: string;
  scope: 'read' | 'write';
  expiresAt?: string;
}

export interface CreateAdminAPIKeyResponse {
  item: AdminAPIKeyItem;
  plaintextKey: string;
}

export interface UpdateAdminAPIKeyRequest {
  name?: string;
  ownerUser?: string;
  scope?: 'read' | 'write';
  enabled?: boolean;
  expiresAt?: string;
}

export const getAdminMe = (): Promise<AdminMeResponse> => {
  return api.get('/admin/me');
};

export const getAdminOverview = (): Promise<AdminOverviewResponse> => {
  return api.get('/admin/overview');
};

export const listAdminNodePools = (): Promise<AdminNodePoolListResponse> => {
  return api.get('/admin/nodes/pools');
};

export const updateAdminNodePool = (name: string, poolType: 'shared' | 'exclusive'): Promise<AdminNodePoolItem> => {
  return api.patch(`/admin/nodes/${encodeURIComponent(name)}/pool`, { poolType });
};

export const listAdminUserPools = (): Promise<AdminUserPoolListResponse> => {
  return api.get('/admin/users/pools');
};

export const updateAdminUserPool = (username: string, poolType: 'shared' | 'exclusive'): Promise<AdminUserPoolItem> => {
  return api.patch(`/admin/users/${encodeURIComponent(username)}/pool`, { poolType });
};

export const deleteAdminUser = (username: string): Promise<{ message: string }> => {
  return api.delete(`/admin/users/${encodeURIComponent(username)}`);
};

export const listAdminAPIKeys = (): Promise<AdminAPIKeyListResponse> => {
  return api.get('/admin/apikeys');
};

export const createAdminAPIKey = (data: CreateAdminAPIKeyRequest): Promise<CreateAdminAPIKeyResponse> => {
  return api.post('/admin/apikeys', data);
};

export const updateAdminAPIKey = (id: string, data: UpdateAdminAPIKeyRequest): Promise<{ item: AdminAPIKeyItem }> => {
  return api.patch(`/admin/apikeys/${encodeURIComponent(id)}`, data);
};

export const deleteAdminAPIKey = (id: string): Promise<{ message: string }> => {
  return api.delete(`/admin/apikeys/${encodeURIComponent(id)}`);
};

// 配置相关
export const getConfig = () => {
  return api.get('/config');
};

// 用户自定义挂载
export interface UserMount {
  hostPath: string;   // 宿主机路径
  mountPath: string;  // 容器内挂载路径
  readOnly?: boolean; // 是否只读
}

// 存储卷信息（用于前端展示）
export interface StorageVolumeInfo {
  name: string;         // 卷名称
  mountPath: string;    // 挂载路径
  description?: string; // 描述信息
  readOnly: boolean;    // 是否只读
  type: string;         // 存储类型: pvc | hostpath
  scope?: string;       // 作用域: user | pod
}

// Pod 相关
export interface CreatePodRequest {
  image: string;
  gpuType?: string;
  gpuCount: number;
  cpu?: string;
  memory?: string;
  shmSize?: string;
  // 高级配置
  nodeName?: string;      // 指定调度节点（可选）
  gpuDevices?: number[];  // 指定 GPU 卡编号（可选）
  name?: string;          // 自定义 Pod 名称后缀（可选）
  userMounts?: UserMount[]; // 用户自定义挂载（可选）
}

export interface ManagedPod {
  id: string;
  name: string;
  namespace: string;
  container: string;
  status: string;
  phase: string;
  image: string;
  gpuType: string;
  gpuCount: number;
  cpu: string;
  memory: string;
  createdAt: string;
  nodeIP?: string;
  workloadKind?: string;
  workloadName?: string;
  protectedUntil?: string;
  connections?: any;
}

export interface PodListResponse {
  pods: ManagedPod[];
  quota: {
    podUsed: number;
    podLimit: number;
    gpuUsed: number;
    gpuLimit: number;
  };
}

export interface CreateStatefulSetRequest {
  image: string;
  gpuType?: string;
  gpuCount: number;
  cpu?: string;
  memory?: string;
  shmSize?: string;
  nodeName?: string;
  name?: string;
  replicas: number;
  userMounts?: UserMount[];
}

export interface CreateDeploymentRequest {
  image: string;
  gpuType?: string;
  gpuCount: number;
  cpu?: string;
  memory?: string;
  shmSize?: string;
  nodeName?: string;
  name?: string;
  replicas: number;
  userMounts?: UserMount[];
}

export interface ManagedDeployment {
  id: string;
  name: string;
  namespace: string;
  status: string;
  image: string;
  gpuType: string;
  gpuCount: number;
  cpu: string;
  memory: string;
  replicas: number;
  readyReplicas: number;
  createdAt: string;
  pods: ManagedPod[];
  suspended?: boolean;
  suspendedImage?: string;
  suspendedReplicas?: number;
  suspendedAt?: string;
}

export interface ManagedStatefulSet {
  id: string;
  name: string;
  namespace: string;
  status: string;
  image: string;
  gpuType: string;
  gpuCount: number;
  cpu: string;
  memory: string;
  replicas: number;
  readyReplicas: number;
  createdAt: string;
  serviceName: string;
  pods: ManagedPod[];
  suspended?: boolean;
  suspendedImage?: string;
  suspendedReplicas?: number;
  suspendedAt?: string;
}

export interface StatefulSetListResponse {
  items: ManagedStatefulSet[];
}

export interface DeploymentListResponse {
  items: ManagedDeployment[];
}

export const listPods = (): Promise<PodListResponse> => {
  return api.get('/pods');
};

export const createPod = (data: CreatePodRequest) => {
  return api.post('/pods', data);
};

export const getPod = (id: string) => {
  return api.get(`/pods/${id}`);
};

export const listDeployments = (): Promise<DeploymentListResponse> => {
  return api.get('/deployments');
};

export const createDeployment = (data: CreateDeploymentRequest) => {
  return api.post('/deployments', data);
};

export const getDeployment = (id: string): Promise<ManagedDeployment> => {
  return api.get(`/deployments/${id}`);
};

export const deleteDeployment = (id: string) => {
  return api.delete(`/deployments/${id}`);
};

export const resumeDeployment = (id: string): Promise<ManagedDeployment> => {
  return api.post(`/deployments/${id}/resume`);
};

export const listStatefulSets = (): Promise<StatefulSetListResponse> => {
  return api.get('/statefulsets');
};

export const createStatefulSet = (data: CreateStatefulSetRequest) => {
  return api.post('/statefulsets', data);
};

export const getStatefulSet = (id: string): Promise<ManagedStatefulSet> => {
  return api.get(`/statefulsets/${id}`);
};

export const deleteStatefulSet = (id: string) => {
  return api.delete(`/statefulsets/${id}`);
};

export const resumeStatefulSet = (id: string): Promise<ManagedStatefulSet> => {
  return api.post(`/statefulsets/${id}/resume`);
};

export const downloadPodYAML = (id: string) => {
  window.location.href = `/api/pods/${encodeURIComponent(id)}/yaml`;
};

export const deletePod = (id: string) => {
  return api.delete(`/pods/${id}`);
};

// 延长 Pod 保护期
export interface ExtendPodResponse {
  message: string;
  protectedUntil: string;
}

export const extendPod = (id: string): Promise<ExtendPodResponse> => {
  return api.post(`/pods/${id}/extend`);
};

export const getPodLogs = (id: string) => {
  return api.get(`/pods/${id}/logs`);
};

export const getPodEvents = (id: string) => {
  return api.get(`/pods/${id}/events`);
};

export const getPodDescribe = (id: string) => {
  return api.get(`/pods/${id}/describe`);
};

export const buildImage = (id: string) => {
  return api.post(`/pods/${id}/build`);
};

export interface CreateWebShellSessionRequest {
  cols: number;
  rows: number;
}

export interface WebShellSessionResponse {
  sessionId: string;
  webSocketURL: string;
  container: string;
  shell: string;
  cols: number;
  rows: number;
  expiresAt: string;
}

export const createWebShellSession = (
  id: string,
  data: CreateWebShellSessionRequest,
): Promise<WebShellSessionResponse> => {
  return api.post(`/pods/${encodeURIComponent(id)}/webshell/sessions`, data);
};

export const deleteWebShellSession = (id: string, sessionId: string): Promise<{ message: string }> => {
  return api.delete(`/pods/${encodeURIComponent(id)}/webshell/sessions/${encodeURIComponent(sessionId)}`);
};

// 镜像 Commit 相关
export interface CommitImageRequest {
  imageName: string;
}

export interface CommitStatus {
  hasJob: boolean;
  jobName?: string;
  status?: 'Pending' | 'Running' | 'Succeeded' | 'Failed';
  message?: string;
  startTime?: string;
  endTime?: string;
  targetImage?: string;
}

export const commitImage = (id: string, imageName: string): Promise<any> => {
  return api.post(`/pods/${id}/commit`, { imageName });
};

export const getCommitStatus = (id: string): Promise<CommitStatus> => {
  return api.get(`/pods/${id}/commit/status`);
};

export const getCommitLogs = (id: string): Promise<{ logs: string }> => {
  return api.get(`/pods/${id}/commit/logs`);
};

// Kubeconfig 相关
export interface ClusterInfo {
  oidcEnabled: boolean;
  kubeconfigMode: string; // "oidc" 或 "cert"
  clusterName?: string;
  issuerURL?: string;
  certValidityDays?: number; // 证书有效期（天），仅 cert 模式
}

export interface KubeconfigInstructions {
  installKubelogin?: Record<string, string>; // 仅 OIDC 模式
  usage: string[];
}

export interface KubeconfigResponse {
  kubeconfig: string;
  namespace: string;
  clusterName: string;
  mode: string; // "oidc" 或 "cert"
  instructions: KubeconfigInstructions;
}

export const getClusterInfo = (): Promise<ClusterInfo> => {
  return api.get('/cluster/info');
};

export const getKubeconfig = (): Promise<KubeconfigResponse> => {
  return api.get('/kubeconfig');
};

export const downloadKubeconfig = () => {
  // 使用 window.location 触发文件下载
  window.location.href = '/api/kubeconfig/download';
};

// GPU 热力图相关
export interface PodInfo {
  name: string;
  namespace: string;
  user: string;
  email?: string;
  gpuCount?: number;
  startTime?: string;
}

export interface DeviceSlot {
  index: number;
  status: 'free' | 'used' | 'full';  // full: 共享模式已满
  utilization: number;
  memoryUsed?: number;   // 已用显存 (MiB)
  memoryTotal?: number;  // 总显存 (MiB)
  metricsStatus: 'fresh' | 'stale' | 'missing';
  metricsUpdatedAt?: string;
  pod?: PodInfo;         // 主 Pod 信息（兼容独占模式）
  // 共享模式字段
  sharedPods?: PodInfo[];  // 共享该卡的所有 Pod
  currentShare: number;     // 当前共享数
  maxShare: number;         // 共享上限，0 表示不限
}

export interface NodeGPUInfo {
  nodeName: string;
  nodeIP: string;
  poolType?: 'shared' | 'exclusive';
  deviceType: string;
  totalDevices: number;
  usedDevices: number;
  slots: DeviceSlot[];
  timeSharingEnabled: boolean;  // 是否支持时分复用
  timeSharingReplicas: number;  // 每卡可共享数
}

export interface AcceleratorGroup {
  type: string;
  label: string;
  resourceName: string;
  nodes: NodeGPUInfo[];
  totalDevices: number;
  usedDevices: number;
}

export interface GPUOverviewResponse {
  acceleratorGroups: AcceleratorGroup[];
  summary: {
    totalDevices: number;
    usedDevices: number;
    byType: Record<string, { total: number; used: number }>;
  };
  updatedAt: string;
  prometheusEnabled: boolean;  // Prometheus 是否已配置
  schedulingMode: 'sharing' | 'exclusive';  // 调度模式
  maxPodsPerGPU: number;  // 每卡最大共享数
}

export const getGPUOverview = (): Promise<GPUOverviewResponse> => {
  return api.get('/cluster/gpu-overview');
};

// 用户保存的镜像
export interface UserSavedImage {
  image: string;
  description?: string;
  sourcePod?: string;
  savedAt: string;
}

export interface UserImageListResponse {
  images: UserSavedImage[];
}

export const listUserImages = (): Promise<UserImageListResponse> => {
  return api.get('/images');
};

export const addUserImage = (data: { image: string; description?: string; sourcePod?: string }) => {
  return api.post('/images', data);
};

export const deleteUserImage = (image: string) => {
  return api.delete(`/images?image=${encodeURIComponent(image)}`);
};

// 共用 GPU 的 Pod 信息
export interface SharedGPUPod {
  name: string;
  namespace: string;
  user: string;
  gpuDevices: number[];   // 该 Pod 使用的所有 GPU
  sharedWith: number[];   // 与当前 Pod 共用的 GPU 编号
  createdAt: string;
}

export interface SharedGPUPodsResponse {
  pods: SharedGPUPod[];
}

export const getSharedGPUPods = (id: string): Promise<SharedGPUPodsResponse> => {
  return api.get(`/pods/${id}/shared-gpus`);
};

// Registry 镜像搜索
export interface RegistryImageInfo {
  name: string;
  tags?: string[];
  description?: string;
}

export interface SearchImagesResponse {
  images: RegistryImageInfo[];
}

export interface GetImageTagsResponse {
  tags: string[];
}

export const searchRegistryImages = (keyword: string, limit: number = 20): Promise<SearchImagesResponse> => {
  return api.get(`/registry/images?keyword=${encodeURIComponent(keyword)}&limit=${limit}`);
};

export const getRegistryImageTags = (imageName: string, platform?: string): Promise<GetImageTagsResponse> => {
  let url = `/registry/tags?image=${encodeURIComponent(imageName)}`;
  if (platform) {
    url += `&platform=${encodeURIComponent(platform)}`;
  }
  return api.get(url);
};

export default api;
