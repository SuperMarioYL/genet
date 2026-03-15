// @ts-nocheck
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
  getPodLogStreamURL,
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
    getPodLogStreamURL: fn(),
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
const mockedGetPodLogStreamURL = getPodLogStreamURL as MockedFunction<typeof getPodLogStreamURL>;
const mockedGetPodEvents = getPodEvents as MockedFunction<typeof getPodEvents>;
const mockedListUserImages = listUserImages as MockedFunction<typeof listUserImages>;
const mockedCommitImage = commitImage as MockedFunction<typeof commitImage>;
const mockedDeleteUserImage = deleteUserImage as MockedFunction<typeof deleteUserImage>;
const mockedGetCommitLogs = getCommitLogs as MockedFunction<typeof getCommitLogs>;

class MockWebSocket {
  constructor(url: string) {
    this.url = url;
    this.binaryType = 'blob';
    this.onopen = null;
    this.onmessage = null;
    this.onclose = null;
    this.onerror = null;
    this.send = jest.fn();
    this.close = jest.fn(() => {
      if (this.onclose) {
        this.onclose();
      }
    });
    MockWebSocket.instances.push(this);
  }
}

(MockWebSocket as any).instances = [];

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
  const originalWebSocket = global.WebSocket;

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
    global.WebSocket = MockWebSocket as any;
    MockWebSocket.instances = [];
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
    mockedGetPodLogStreamURL.mockImplementation((id: string, options?: any) => {
      const suffix = options?.since ? `?since=${encodeURIComponent(options.since)}` : '';
      return `/api/pods/${id}/logs/stream${suffix}`;
    });
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
    global.WebSocket = originalWebSocket;
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

  it('requests previous pod logs when the previous logs view is selected', async () => {
    mockedGetPodLogs.mockImplementation(async (_id: string, options?: any) => (
      { logs: options?.previous ? 'previous logs' : 'current logs', cursor: options?.previous ? undefined : '2026-03-15T10:00:00Z' } as any
    ));

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

    const logsTab = Array.from(document.querySelectorAll('[role="tab"]')).find(
      (tab) => tab.textContent?.includes('实时日志'),
    );

    await act(async () => {
      logsTab?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    expect(MockWebSocket.instances).toHaveLength(1);

    const previousButton = Array.from(document.querySelectorAll('button')).find(
      (candidate) => candidate.textContent?.includes('之前日志'),
    );
    expect(previousButton).toBeTruthy();

    await act(async () => {
      previousButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(MockWebSocket.instances[0].close).toHaveBeenCalled();

    const refreshButton = Array.from(document.querySelectorAll('button')).find(
      (candidate) => candidate.textContent?.includes('刷新日志'),
    );
    expect(refreshButton).toBeTruthy();

    await act(async () => {
      refreshButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    expect(mockedGetPodLogs as any).toHaveBeenLastCalledWith('pod-alice-dev', { previous: true });
    expect(container.textContent).toContain('previous logs');
  });

  it('loads current logs once and appends streaming log chunks', async () => {
    mockedGetPodLogs.mockResolvedValueOnce({
      logs: 'line 1\nline 2\n',
      cursor: '2026-03-15T10:00:02Z',
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

    const logsTab = Array.from(document.querySelectorAll('[role="tab"]')).find(
      (tab) => tab.textContent?.includes('实时日志'),
    );

    await act(async () => {
      logsTab?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    expect(mockedGetPodLogs).toHaveBeenCalledWith('pod-alice-dev', { previous: false, tailLines: 200 });
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toContain('/api/pods/pod-alice-dev/logs/stream');
    expect(MockWebSocket.instances[0].url).toContain('since=2026-03-15T10%3A00%3A02Z');
    expect(container.textContent).toContain('line 1');
    expect(container.textContent).toContain('line 2');
    const callsBeforeMessage = mockedGetPodLogs.mock.calls.length;

    await act(async () => {
      MockWebSocket.instances[0].onmessage?.({
        data: JSON.stringify({
          type: 'chunk',
          content: 'line 3\n',
          cursor: '2026-03-15T10:00:03Z',
        }),
      });
    });

    await flushEffects();

    expect(container.textContent).toContain('line 3');
    expect(mockedGetPodLogs.mock.calls.length).toBe(callsBeforeMessage);
  });

  it('trims the current log buffer while appending streamed chunks', async () => {
    const initialLogs = `${Array.from({ length: 3000 }, (_, index) => `entry-${String(index + 1).padStart(4, '0')}`).join('\n')}\n`;
    mockedGetPodLogs.mockResolvedValueOnce({
      logs: initialLogs,
      cursor: '2026-03-15T10:00:02Z',
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

    const logsTab = Array.from(document.querySelectorAll('[role="tab"]')).find(
      (tab) => tab.textContent?.includes('实时日志'),
    );

    await act(async () => {
      logsTab?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    await act(async () => {
      MockWebSocket.instances[0].onmessage?.({
        data: JSON.stringify({
          type: 'chunk',
          content: 'entry-3001\nentry-3002\n',
          cursor: '2026-03-15T10:00:04Z',
        }),
      });
    });

    await flushEffects();

    expect(container.textContent).not.toContain('entry-0001');
    expect(container.textContent).not.toContain('entry-0002');
    expect(container.textContent).toContain('entry-3001');
    expect(container.textContent).toContain('entry-3002');
  });
});
