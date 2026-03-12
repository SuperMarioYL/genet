import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import AcceleratorHeatmap from './index';
import { getGPUOverview } from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    getGPUOverview: fn(),
  };
});

const mockedGetGPUOverview = getGPUOverview as MockedFunction<typeof getGPUOverview>;

const flushEffects = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('AcceleratorHeatmap', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
    mockedGetGPUOverview.mockResolvedValue({
      summary: {
        totalDevices: 2,
        usedDevices: 1,
        totalNodes: 1,
        totalPods: 1,
      },
      acceleratorGroups: [
        {
          type: 'nvidia',
          label: 'NVIDIA',
          totalDevices: 2,
          usedDevices: 1,
          nodes: [
            {
              nodeName: 'worker-1',
              nodeIP: '10.0.0.1',
              deviceType: 'NVIDIA H200 PCIe',
              totalDevices: 2,
              usedDevices: 1,
              poolType: 'shared',
              slots: [
                {
                  index: 0,
                  status: 'used',
                  utilization: 45,
                  memoryUsed: 1024,
                  memoryTotal: 2048,
                  pod: {
                    name: 'pod-a',
                    namespace: 'default',
                    user: 'alice',
                  },
                },
                {
                  index: 1,
                  status: 'free',
                  utilization: 0,
                  memoryUsed: 0,
                  memoryTotal: 2048,
                },
              ],
            },
          ],
        },
      ],
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

  it('renders heatmap content without an internal duplicate title', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap refreshInterval={60000} />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(container.textContent).toContain('NVIDIA');
    expect(container.textContent).not.toContain('Accelerator Heatmap');
    expect(container.textContent).not.toContain('GPU 热力图');
  });
});
