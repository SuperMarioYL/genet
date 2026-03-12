import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import AcceleratorHeatmap from './index';
import { getGPUOverview } from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

const mockNavigate = require('jest-mock').fn();
let tooltipRenderCount = 0;

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
    Tooltip: ({ children, title }: any) => {
      tooltipRenderCount += 1;

      return (
        <div className="tooltip-mock">
          {children}
          <div className="tooltip-mock-content">{title}</div>
        </div>
      );
    },
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
              utilization: 45,
              memoryUsed: 1024,
              memoryTotal: 2048,
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

describe('AcceleratorHeatmap', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
    mockedGetGPUOverview.mockResolvedValue(buildOverview());
    mockNavigate.mockReset();
    tooltipRenderCount = 0;

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
    expect(container.textContent).toContain('总卡数量');
    expect(container.textContent).toContain('占用量');
  });

  it('keeps single-pod tooltip details and click navigation', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap refreshInterval={60000} />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(container.textContent).toContain('Pod:');
    expect(container.textContent).toContain('pod-a');
    expect(container.textContent).toContain('User:');
    expect(container.textContent).toContain('alice');
    expect(container.textContent).not.toContain('tabs-mock-nav');

    const usedCell = container.querySelector('.device-cell-used') as HTMLDivElement;
    expect(usedCell).toBeTruthy();

    await act(async () => {
      usedCell.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(mockNavigate).toHaveBeenCalledWith('/pod/default/pod-a');
  });

  it('renders pod tabs for shared slots and disables whole-cell navigation', async () => {
    mockedGetGPUOverview.mockResolvedValueOnce(buildOverview({
      sharedPods: [
        {
          name: 'pod-a',
          namespace: 'default',
          user: 'alice',
          email: 'alice@example.com',
          startTime: '2026-03-12T08:00:00Z',
        },
        {
          name: 'pod-b',
          namespace: 'default',
          user: 'bob',
          email: 'bob@example.com',
          startTime: '2026-03-12T09:30:00Z',
        },
      ],
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
    expect(container.textContent).toContain('pod-b');
    expect(container.textContent).toContain('alice@example.com');
    expect(container.textContent).not.toContain('bob@example.com');

    const secondTab = Array.from(container.querySelectorAll('.tabs-mock-nav button')).find(
      (button) => button.textContent === 'pod-b',
    ) as HTMLButtonElement | undefined;
    expect(secondTab).toBeTruthy();

    await act(async () => {
      secondTab?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(container.textContent).toContain('bob@example.com');
    expect(container.textContent).not.toContain('alice@example.com');

    const usedCell = container.querySelector('.device-cell-used') as HTMLDivElement;
    await act(async () => {
      usedCell.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('does not rerender device tooltips when refresh returns unchanged data', async () => {
    mockedGetGPUOverview
      .mockResolvedValueOnce(buildOverview())
      .mockResolvedValueOnce(buildOverview());

    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap refreshInterval={60000} />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(tooltipRenderCount).toBe(2);

    const refreshButton = container.querySelector('.refresh-btn') as HTMLButtonElement;
    expect(refreshButton).toBeTruthy();

    await act(async () => {
      refreshButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    expect(tooltipRenderCount).toBe(2);
  });

  it('does not refetch immediately when callback props get a new identity', async () => {
    mockedGetGPUOverview.mockResolvedValue(buildOverview());

    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap
            refreshInterval={60000}
            onError={() => undefined}
            onSummaryChange={() => undefined}
          />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(mockedGetGPUOverview).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.render(
        <MemoryRouter>
          <AcceleratorHeatmap
            refreshInterval={60000}
            onError={() => undefined}
            onSummaryChange={() => undefined}
          />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(mockedGetGPUOverview).toHaveBeenCalledTimes(1);
  });
});
