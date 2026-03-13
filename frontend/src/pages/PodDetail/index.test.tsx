import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import PodDetail from './index';
import {
  commitImage,
  deleteUserImage,
  getCommitLogs,
  getCommitStatus,
  getConfig,
  getPod,
  getPodDescribe,
  getPodEvents,
  getPodLogs,
  getSharedGPUPods,
  listUserImages,
} from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

jest.mock('../../components/GlassCard', () => ({ children, title, extra }: any) => (
  <div>
    {title ? <div>{title}</div> : null}
    {extra}
    {children}
  </div>
));
jest.mock('../../components/StatusBadge', () => ({ status }: any) => <span>{status}</span>);
jest.mock('../../components/ThemeToggle', () => () => <button type="button">theme</button>);
jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    commitImage: fn(),
    deleteUserImage: fn(),
    getCommitLogs: fn(),
    getCommitStatus: fn(),
    getConfig: fn(),
    getPod: fn(),
    getPodDescribe: fn(),
    getPodEvents: fn(),
    getPodLogs: fn(),
    getSharedGPUPods: fn(),
    listUserImages: fn(),
  };
});

const mockedGetPod = getPod as MockedFunction<typeof getPod>;
const mockedGetCommitStatus = getCommitStatus as MockedFunction<typeof getCommitStatus>;
const mockedGetSharedGPUPods = getSharedGPUPods as MockedFunction<typeof getSharedGPUPods>;
const mockedGetConfig = getConfig as MockedFunction<typeof getConfig>;
const mockedGetPodDescribe = getPodDescribe as MockedFunction<typeof getPodDescribe>;
const mockedGetPodLogs = getPodLogs as MockedFunction<typeof getPodLogs>;
const mockedGetPodEvents = getPodEvents as MockedFunction<typeof getPodEvents>;
const mockedListUserImages = listUserImages as MockedFunction<typeof listUserImages>;
const mockedCommitImage = commitImage as MockedFunction<typeof commitImage>;
const mockedDeleteUserImage = deleteUserImage as MockedFunction<typeof deleteUserImage>;
const mockedGetCommitLogs = getCommitLogs as MockedFunction<typeof getCommitLogs>;

const flushEffects = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('PodDetail code-server entry', () => {
  let container: HTMLDivElement;
  let root: Root;
  let openSpy: ReturnType<typeof jest.spyOn>;

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
    openSpy = jest.spyOn(window, 'open').mockImplementation(() => null);
    mockedGetPod.mockResolvedValue({
      id: 'pod-alice-dev',
      name: 'pod-alice-dev',
      status: 'Running',
      phase: 'Running',
      image: 'ubuntu:22.04',
      gpuCount: 0,
      cpu: '4',
      memory: '8Gi',
      createdAt: '2026-03-13T10:00:00Z',
      connections: {
        apps: {
          codeServerURL: '/api/pods/pod-alice-dev/apps/code-server',
          codeServerReady: true,
          codeServerStatus: 'enabled',
        },
      },
    } as any);
    mockedGetCommitStatus.mockResolvedValue({ hasJob: false } as any);
    mockedGetSharedGPUPods.mockResolvedValue({ pods: [] } as any);
    mockedGetConfig.mockResolvedValue({ storageVolumes: [], registryUrl: '' } as any);
    mockedGetPodDescribe.mockResolvedValue({ mounts: [], injectedEnvVars: [] } as any);
    mockedGetPodLogs.mockResolvedValue({ logs: '' } as any);
    mockedGetPodEvents.mockResolvedValue({ events: [] } as any);
    mockedListUserImages.mockResolvedValue({ images: [] } as any);
    mockedCommitImage.mockResolvedValue({} as any);
    mockedDeleteUserImage.mockResolvedValue({} as any);
    mockedGetCommitLogs.mockResolvedValue({ logs: '' } as any);

    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    container.remove();
    openSpy.mockRestore();
    jest.clearAllMocks();
  });

  it('opens code-server from the connection tab when the backend exposes the web IDE URL', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={['/pods/pod-alice-dev']}>
          <Routes>
            <Route path="/pods/:id" element={<PodDetail />} />
          </Routes>
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const connectionTab = Array.from(document.querySelectorAll('[role="tab"]')).find(
      (tab) => tab.textContent?.includes('连接信息'),
    );
    expect(connectionTab).toBeTruthy();

    await act(async () => {
      connectionTab?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    const button = Array.from(document.querySelectorAll('button')).find(
      (candidate) => candidate.textContent?.includes('code-server'),
    );
    expect(button).toBeTruthy();

    await act(async () => {
      button?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(openSpy).toHaveBeenCalledWith('/api/pods/pod-alice-dev/apps/code-server', '_blank', 'noopener,noreferrer');
  });

  it('opens web shell from the connection tab when the backend exposes the shell URL', async () => {
    mockedGetPod.mockResolvedValueOnce({
      id: 'pod-alice-dev',
      name: 'pod-alice-dev',
      status: 'Running',
      phase: 'Running',
      image: 'ubuntu:22.04',
      gpuCount: 0,
      cpu: '4',
      memory: '8Gi',
      createdAt: '2026-03-13T10:00:00Z',
      connections: {
        apps: {
          webShellURL: '/pods/pod-alice-dev/webshell',
          webShellReady: true,
          webShellStatus: 'enabled',
        },
      },
    } as any);

    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={['/pods/pod-alice-dev']}>
          <Routes>
            <Route path="/pods/:id" element={<PodDetail />} />
          </Routes>
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const connectionTab = Array.from(document.querySelectorAll('[role="tab"]')).find(
      (tab) => tab.textContent?.includes('连接信息'),
    );

    await act(async () => {
      connectionTab?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    const button = Array.from(document.querySelectorAll('button')).find(
      (candidate) => candidate.textContent?.includes('Web Shell'),
    );
    expect(button).toBeTruthy();

    await act(async () => {
      button?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(openSpy).toHaveBeenCalledWith('/pods/pod-alice-dev/webshell', '_blank', 'noopener,noreferrer');
  });
});
