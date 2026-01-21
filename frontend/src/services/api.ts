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
    const message = error.response?.data?.error || error.message || '请求失败';
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
export const listPods = () => {
  return api.get('/pods');
};

export const createPod = (data: any) => {
  return api.post('/pods', data);
};

export const getPod = (id: string) => {
  return api.get(`/pods/${id}`);
};

export const extendPod = (id: string, hours: number) => {
  return api.post(`/pods/${id}/extend`, { hours });
};

export const deletePod = (id: string) => {
  return api.delete(`/pods/${id}`);
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
  clusterName?: string;
  issuerURL?: string;
}

export interface KubeconfigInstructions {
  installKubelogin: Record<string, string>;
  usage: string[];
}

export interface KubeconfigResponse {
  kubeconfig: string;
  namespace: string;
  clusterName: string;
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

export default api;

