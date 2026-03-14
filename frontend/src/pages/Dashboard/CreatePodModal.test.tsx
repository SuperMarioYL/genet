import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import CreatePodModal from './CreatePodModal';
import { getConfig, getGPUOverview } from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

jest.mock('../../components/GPUSelector', () => () => <div>gpu selector</div>);
jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    getConfig: fn(),
    getGPUOverview: fn(),
    createPod: fn(),
    createDeployment: fn(),
    createStatefulSet: fn(),
    searchRegistryImages: fn(),
    getRegistryImageTags: fn(),
  };
});

const mockedGetConfig = getConfig as MockedFunction<typeof getConfig>;
const mockedGetGPUOverview = getGPUOverview as MockedFunction<typeof getGPUOverview>;

describe('CreatePodModal manual GPU selection access', () => {
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

    mockedGetConfig.mockResolvedValue({
      gpuTypes: [{ platform: 'nvidia', name: 'A100', resourceName: 'nvidia.com/gpu' }],
      ui: { defaultCPU: '4', defaultMemory: '8Gi', defaultShmSize: '1Gi' },
      storageVolumes: [],
      allowUserMounts: false,
    } as any);
    mockedGetGPUOverview.mockResolvedValue({
      acceleratorGroups: [
        {
          type: 'nvidia',
          label: 'NVIDIA GPU',
          resourceName: 'nvidia.com/gpu',
          nodes: [
            {
              nodeName: 'node-a',
              nodeIP: '10.0.0.1',
              poolType: 'shared',
              deviceType: 'A100',
              totalDevices: 8,
              usedDevices: 2,
              slots: [],
              timeSharingEnabled: false,
              timeSharingReplicas: 1,
            },
          ],
          totalDevices: 8,
          usedDevices: 2,
        },
      ],
      schedulingMode: 'exclusive',
      maxPodsPerGPU: 1,
      prometheusEnabled: true,
    } as any);

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

  it('hides manual GPU selection for non-admin users', async () => {
    await act(async () => {
      root.render(
        <CreatePodModal
          visible
          isAdmin={false}
          onCancel={() => undefined}
          onSuccess={() => undefined}
          currentQuota={{ podUsed: 0, podLimit: 5, gpuUsed: 0, gpuLimit: 8 }}
        />,
      );
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(document.body.textContent).not.toContain('高级设置');
  });

  it('shows manual GPU selection for admin users', async () => {
    await act(async () => {
      root.render(
        <CreatePodModal
          visible
          isAdmin
          onCancel={() => undefined}
          onSuccess={() => undefined}
          currentQuota={{ podUsed: 0, podLimit: 5, gpuUsed: 0, gpuLimit: 8 }}
        />,
      );
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const selectors = document.body.querySelectorAll('.ant-select-selector');
    const workloadTypeSelector = selectors.item(0) as HTMLElement | null;
    expect(workloadTypeSelector).toBeTruthy();

    await act(async () => {
      workloadTypeSelector?.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
    });

    await act(async () => {
      await Promise.resolve();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const podOption = Array.from(document.body.querySelectorAll('.ant-select-item-option')).find(
      (candidate) => candidate.textContent?.trim() === 'Pod',
    ) as HTMLElement | undefined;
    expect(podOption).toBeTruthy();

    await act(async () => {
      podOption?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(document.body.textContent).toContain('高级设置');
  });
});
