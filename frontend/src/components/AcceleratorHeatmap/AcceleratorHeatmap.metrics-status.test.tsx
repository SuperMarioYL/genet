import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import AcceleratorHeatmap from './index';
import { getGPUOverview } from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

const mockNavigate = require('jest-mock').fn();

jest.mock('antd', () => {
  const React = require('react');

  const Tabs = ({ items, defaultActiveKey }: any) => {
    const [activeKey, setActiveKey] = React.useState(defaultActiveKey ?? items[0]?.key);
    const activeItem = items.find((item: any) => item.key === activeKey) ?? items[0];

    return (
      <div className="tabs-mock">
        <div className="tabs-mock-nav">
          {items.map((item: any) => (
            <button
              key={item.key}
              type="button"
              data-active={item.key === activeKey}
              onClick={() => setActiveKey(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <div className="tabs-mock-content">{activeItem?.children}</div>
      </div>
    );
  };

  return {
    Button: ({ children, className, onClick }: any) => (
      <button type="button" className={className} onClick={onClick}>
        {children}
      </button>
    ),
    Empty: ({ description }: any) => <div>{description}</div>,
    Spin: () => <div>loading</div>,
    Tabs,
    Tooltip: ({ children, title }: any) => (
      <div className="tooltip-mock">
        {children}
        <div className="tooltip-mock-content">{title}</div>
      </div>
    ),
    Typography: {
      Text: ({ children, className }: any) => <span className={className}>{children}</span>,
    },
  };
});

jest.mock('react-router-dom', () => {
  return {
    MemoryRouter: ({ children }: any) => <>{children}</>,
    useNavigate: () => mockNavigate,
  };
});

jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    getGPUOverview: fn(),
  };
});

const mockedGetGPUOverview = getGPUOverview as MockedFunction<typeof getGPUOverview>;

const buildOverview = (slotOverride: Record<string, unknown> = {}) => ({
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
              utilization: 0,
              memoryUsed: 0,
              memoryTotal: 2048,
              metricsStatus: 'fresh',
              metricsUpdatedAt: '2026-03-13T10:00:00Z',
              currentShare: 1,
              maxShare: 1,
              pod: {
                name: 'pod-a',
                namespace: 'default',
                user: 'alice',
                email: 'alice@example.com',
                startTime: '2026-03-12T08:00:00Z',
              },
              ...slotOverride,
            },
            {
              index: 1,
              status: 'free',
              utilization: 0,
              memoryUsed: 0,
              memoryTotal: 2048,
              metricsStatus: 'fresh',
              metricsUpdatedAt: '2026-03-13T10:00:00Z',
              currentShare: 0,
              maxShare: 0,
            },
          ],
        },
      ],
    },
  ],
}) as any;

const flushEffects = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('AcceleratorHeatmap metrics status', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
    mockedGetGPUOverview.mockReset();
    mockedGetGPUOverview.mockResolvedValue(buildOverview());
    mockNavigate.mockReset();

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

  it('renders missing metrics slots in grey and shows missing tooltip copy', async () => {
    mockedGetGPUOverview.mockResolvedValueOnce(buildOverview({
      metricsStatus: 'missing',
      metricsUpdatedAt: undefined,
    }));

    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap refreshInterval={60000} />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const usedCell = container.querySelector('.device-cell-used') as HTMLDivElement;
    expect(usedCell.className).toContain('device-cell-metrics-unavailable');
    expect(container.textContent).toContain('指标状态');
    expect(container.textContent).toContain('未采集');
  });

  it('renders stale metrics slots in grey and shows last metrics update time', async () => {
    mockedGetGPUOverview.mockResolvedValueOnce(buildOverview({
      metricsStatus: 'stale',
      metricsUpdatedAt: '2026-03-13T09:45:00Z',
    }));

    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap refreshInterval={60000} />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const usedCell = container.querySelector('.device-cell-used') as HTMLDivElement;
    expect(usedCell.className).toContain('device-cell-metrics-unavailable');
    expect(container.textContent).toContain('长时间未更新');
    expect(container.textContent).toContain('最后采集时间');
  });

  it('keeps fresh zero-utilization slots green instead of grey', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap refreshInterval={60000} />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const usedCell = container.querySelector('.device-cell-used') as HTMLDivElement;
    expect(usedCell.className).not.toContain('device-cell-metrics-unavailable');
  });

  it('keeps pod tooltip and navigation when metrics are stale', async () => {
    mockedGetGPUOverview.mockResolvedValueOnce(buildOverview({
      metricsStatus: 'stale',
      metricsUpdatedAt: '2026-03-13T09:45:00Z',
    }));

    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap refreshInterval={60000} />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(container.textContent).toContain('pod-a');
    expect(container.textContent).toContain('alice');

    const usedCell = container.querySelector('.device-cell-used') as HTMLDivElement;
    await act(async () => {
      usedCell.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(mockNavigate).toHaveBeenCalledWith('/pod/default/pod-a');
  });
});
