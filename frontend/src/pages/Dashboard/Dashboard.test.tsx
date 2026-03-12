import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import Dashboard from './index';
import {
  getAdminMe,
  getClusterInfo,
  getConfig,
  getKubeconfig,
  listPods,
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
jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    listPods: fn(),
    getClusterInfo: fn(),
    getConfig: fn(),
    getAdminMe: fn(),
    getKubeconfig: fn(),
    downloadKubeconfig: fn(),
  };
});

const mockedListPods = listPods as MockedFunction<typeof listPods>;
const mockedGetClusterInfo = getClusterInfo as MockedFunction<typeof getClusterInfo>;
const mockedGetConfig = getConfig as MockedFunction<typeof getConfig>;
const mockedGetAdminMe = getAdminMe as MockedFunction<typeof getAdminMe>;
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
    mockedGetClusterInfo.mockResolvedValue({ kubeconfigMode: 'cert' } as any);
    mockedGetConfig.mockResolvedValue({} as any);
    mockedGetAdminMe.mockResolvedValue({ isAdmin: false } as any);
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
});
