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
  oauthEnabled: boolean;
  loginURL?: string;
}

export const getAuthStatus = (): Promise<AuthStatus> => {
  return api.get('/auth/status');
};

export const logout = () => {
  return api.get('/auth/logout');
};

// 配置相关
export const getConfig = () => {
  return api.get('/config');
};

// Pod 相关
export interface CreatePodRequest {
  image: string;
  gpuType?: string;
  gpuCount: number;
  cpu?: string;
  memory?: string;
  // 高级配置
  nodeName?: string;      // 指定调度节点（可选）
  gpuDevices?: number[];  // 指定 GPU 卡编号（可选）
  name?: string;          // 自定义 Pod 名称后缀（可选）
}

export const listPods = () => {
  return api.get('/pods');
};

export const createPod = (data: CreatePodRequest) => {
  return api.post('/pods', data);
};

export const getPod = (id: string) => {
  return api.get(`/pods/${id}`);
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
  pod?: PodInfo;         // 主 Pod 信息（兼容独占模式）
  // 共享模式字段
  sharedPods?: PodInfo[];  // 共享该卡的所有 Pod
  currentShare: number;     // 当前共享数
  maxShare: number;         // 共享上限，0 表示不限
}

export interface NodeGPUInfo {
  nodeName: string;
  nodeIP: string;
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

export default api;

