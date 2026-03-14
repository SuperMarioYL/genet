import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import Dashboard from './index';
import {
  getAuthStatus,
  getClusterInfo,
  getConfig,
  getKubeconfig,
  listPods,
  listDeployments,
  listStatefulSets,
} from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

jest.mock('../../components/AcceleratorHeatmap', () => () => <div>mock heatmap</div>);
jest.mock('../../components/GlassCard', () => ({ children, title }: any) => (
  <div>
    {title ? <div>{title}</div> : null}
    {children}
  </div>
));
jest.mock('../../components/ThemeToggle', () => () => <button type="button">theme</button>);
jest.mock('./CreatePodModal', () => () => null);
jest.mock('./PodCard', () => () => <div>pod card</div>);
jest.mock('./DeploymentCard', () => () => <div>deployment card</div>);
jest.mock('./StatefulSetCard', () => () => <div>statefulset card</div>);
jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    listPods: fn(),
    listDeployments: fn(),
    listStatefulSets: fn(),
    getClusterInfo: fn(),
    getConfig: fn(),
    getAuthStatus: fn(),
    getKubeconfig: fn(),
    downloadKubeconfig: fn(),
  };
});

const mockedListPods = listPods as MockedFunction<typeof listPods>;
const mockedListDeployments = listDeployments as MockedFunction<typeof listDeployments>;
const mockedListStatefulSets = listStatefulSets as MockedFunction<typeof listStatefulSets>;
const mockedGetClusterInfo = getClusterInfo as MockedFunction<typeof getClusterInfo>;
const mockedGetConfig = getConfig as MockedFunction<typeof getConfig>;
const mockedGetAuthStatus = getAuthStatus as MockedFunction<typeof getAuthStatus>;
const mockedGetKubeconfig = getKubeconfig as MockedFunction<typeof getKubeconfig>;

const flushEffects = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('Dashboard heatmap entry', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string): MediaQueryList => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: () => undefined,
        removeListener: () => undefined,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        dispatchEvent: () => false,
      }),
    });

    mockedListPods.mockResolvedValue({
      pods: [],
      quota: { podUsed: 0, podLimit: 5, gpuUsed: 0, gpuLimit: 8 },
    } as any);
    mockedListDeployments.mockResolvedValue({ items: [] } as any);
    mockedListStatefulSets.mockResolvedValue({ items: [] } as any);
    mockedGetClusterInfo.mockResolvedValue({ kubeconfigMode: 'cert' } as any);
    mockedGetConfig.mockResolvedValue({} as any);
    mockedGetAuthStatus.mockResolvedValue({ authenticated: true, username: 'alice', isAdmin: false, poolType: 'shared' } as any);
    mockedGetKubeconfig.mockResolvedValue({} as any);

    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    container.remove();
    jest.clearAllMocks();
  });

  it('uses GPU 热力图 for both the button and modal title', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <Dashboard />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const heatmapButton = Array.from(container.querySelectorAll('button')).find(
      (button) => button.textContent?.includes('GPU 热力图'),
    );

    expect(heatmapButton).toBeTruthy();

    await act(async () => {
      heatmapButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    expect(document.body.textContent).toContain('GPU 热力图');
    expect(document.body.textContent).not.toContain('GPU Heatmap');
  });

  it('keeps the heatmap modal title free of summary stats', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <Dashboard />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const heatmapButton = Array.from(container.querySelectorAll('button')).find(
      (button) => button.textContent?.includes('GPU 热力图'),
    );

    await act(async () => {
      heatmapButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    expect(document.body.querySelector('.modal-title-custom')).toBeTruthy();
    expect(document.body.querySelector('.heatmap-title-summary')).toBeNull();
    expect(document.body.textContent).not.toContain('总卡数量');
    expect(document.body.textContent).not.toContain('占用量');
  });

  it('renders statefulset cards when workload data contains statefulsets', async () => {
    mockedListStatefulSets.mockResolvedValueOnce({
      items: [
        {
          id: 'sts-alice-train',
          name: 'sts-alice-train',
          replicas: 2,
          readyReplicas: 1,
          pods: [],
        },
      ],
    } as any);

    await act(async () => {
      root.render(
        <MemoryRouter>
          <Dashboard />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(container.textContent).toContain('statefulset card');
  });

  it('renders deployment cards when workload data contains deployments', async () => {
    mockedListDeployments.mockResolvedValueOnce({
      items: [
        {
          id: 'deploy-alice-train',
          name: 'deploy-alice-train',
          replicas: 1,
          readyReplicas: 1,
          pods: [],
        },
      ],
    } as any);

    await act(async () => {
      root.render(
        <MemoryRouter>
          <Dashboard />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(container.textContent).toContain('deployment card');
  });

  it('shows the current user in the header and removes the old API keys shortcut', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <Dashboard />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(container.textContent).toContain('alice');
    expect(container.textContent).not.toContain('API Keys');
  });
});
