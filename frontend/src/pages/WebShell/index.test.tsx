// @ts-nocheck
import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import WebShellPage from './index';
import { createWebShellSession, deleteWebShellSession, getPod } from '../../services/api';

const mockTerminalState = {
  open: jest.fn(),
  loadAddon: jest.fn(),
  focus: jest.fn(),
  write: jest.fn(),
  dispose: jest.fn(),
  onDataCallback: undefined,
};

jest.mock('@xterm/xterm', () => {
  const { fn } = require('jest-mock');
  return {
    Terminal: fn().mockImplementation(() => ({
      open: mockTerminalState.open,
      loadAddon: mockTerminalState.loadAddon,
      focus: mockTerminalState.focus,
      write: mockTerminalState.write,
      dispose: mockTerminalState.dispose,
      onData: (cb) => {
        mockTerminalState.onDataCallback = cb;
        return { dispose: fn() };
      },
    })),
  };
});

jest.mock('@xterm/addon-fit', () => {
  const { fn } = require('jest-mock');
  return {
    FitAddon: fn().mockImplementation(() => ({
      fit: fn(),
    })),
  };
});

jest.mock('../../components/GlassCard', () => (props) => (
  <div>
    {props.title ? <div>{props.title}</div> : null}
    {props.children}
  </div>
));

jest.mock('../../components/ThemeToggle', () => () => <button type="button">theme</button>);
jest.mock('../../components/StatusBadge', () => (props) => <span>{props.status}</span>);

jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    createWebShellSession: fn(),
    deleteWebShellSession: fn(),
    getPod: fn(),
  };
});

class MockWebSocket {
  constructor(url) {
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

MockWebSocket.instances = [];

const flushEffects = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('WebShellPage', () => {
  let container;
  let root;
  const originalWebSocket = global.WebSocket;

  beforeEach(() => {
    jest.useFakeTimers();
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query) => ({
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
    global.WebSocket = MockWebSocket;
    MockWebSocket.instances = [];
    mockTerminalState.open.mockClear();
    mockTerminalState.loadAddon.mockClear();
    mockTerminalState.focus.mockClear();
    mockTerminalState.write.mockClear();
    mockTerminalState.dispose.mockClear();
    mockTerminalState.onDataCallback = undefined;

    createWebShellSession.mockResolvedValue({
      sessionId: 'session-1',
      webSocketURL: '/api/pods/pod-alice-dev/webshell/sessions/session-1/ws',
      container: 'workspace',
      shell: '/bin/bash (fallback /bin/sh)',
      cols: 120,
      rows: 40,
      expiresAt: '2026-03-13T11:00:00Z',
    });
    deleteWebShellSession.mockResolvedValue({ message: 'closed' });
    getPod.mockResolvedValue({
      id: 'pod-alice-dev',
      name: 'pod-alice-dev',
      status: 'Running',
      cpu: '8',
      memory: '32Gi',
      gpuType: 'NVIDIA A100',
      gpuCount: 1,
      nodeIP: '10.0.0.8',
      createdAt: '2026-03-15T09:30:00Z',
    });

    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    document.body.removeChild(container);
    global.WebSocket = originalWebSocket;
    jest.runOnlyPendingTimers();
    jest.useRealTimers();
    jest.clearAllMocks();
  });

  it('creates a session, connects websocket, and deletes the session on unmount', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={['/pods/pod-alice-dev/webshell']}>
          <Routes>
            <Route path="/pods/:id/webshell" element={<WebShellPage />} />
          </Routes>
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(createWebShellSession).toHaveBeenCalledWith('pod-alice-dev', { cols: 120, rows: 40 });
    expect(getPod).toHaveBeenCalledWith('pod-alice-dev');
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toContain('/api/pods/pod-alice-dev/webshell/sessions/session-1/ws');
    expect(mockTerminalState.open).not.toHaveBeenCalled();
    expect(container.textContent).toContain('pod-alice-dev');
    expect(container.textContent).toContain('8 核 / 32Gi');
    expect(container.textContent).toContain('NVIDIA A100 ×1');
    expect(container.textContent).toContain('10.0.0.8');

    await act(async () => {
      MockWebSocket.instances[0].onopen();
      jest.advanceTimersByTime(300);
    });

    expect(mockTerminalState.open).toHaveBeenCalled();
    expect(mockTerminalState.focus).toHaveBeenCalled();

    await act(async () => {
      MockWebSocket.instances[0].onmessage({ data: new Uint8Array([101, 99, 104, 111]) });
    });

    expect(mockTerminalState.write).toHaveBeenCalled();

    await act(async () => {
      mockTerminalState.onDataCallback('ls\n');
    });

    const sentPayload = MockWebSocket.instances[0].send.mock.calls.at(-1)?.[0];
    expect(sentPayload).toBeInstanceOf(Blob);

    await act(async () => {
      root.unmount();
    });

    expect(deleteWebShellSession).toHaveBeenCalledWith('pod-alice-dev', 'session-1');
  });

  it('retries failed sessions before showing the terminal', async () => {
    createWebShellSession
      .mockRejectedValueOnce(new Error('连接关闭'))
      .mockRejectedValueOnce(new Error('连接关闭'))
      .mockResolvedValueOnce({
        sessionId: 'session-3',
        webSocketURL: '/api/pods/pod-alice-dev/webshell/sessions/session-3/ws',
        container: 'workspace',
        shell: '/bin/bash (fallback /bin/sh)',
        cols: 120,
        rows: 40,
        expiresAt: '2026-03-13T11:00:00Z',
      });

    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={['/pods/pod-alice-dev/webshell']}>
          <Routes>
            <Route path="/pods/:id/webshell" element={<WebShellPage />} />
          </Routes>
        </MemoryRouter>,
      );
    });

    await flushEffects();

    expect(container.textContent).toContain('正在建立 Web Shell 连接');
    expect(mockTerminalState.open).not.toHaveBeenCalled();

    await act(async () => {
      jest.advanceTimersByTime(1600);
    });
    await flushEffects();
    await flushEffects();

    expect(createWebShellSession).toHaveBeenCalledTimes(3);
    expect(mockTerminalState.open).not.toHaveBeenCalled();
    expect(container.textContent).toContain('正在建立 Web Shell 连接');
  });

  it('returns to the pod detail page when clicking back', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={['/pods/pod-alice-dev/webshell']}>
          <Routes>
            <Route path="/pods/:id/webshell" element={<WebShellPage />} />
            <Route path="/pods/:id" element={<div>pod detail page</div>} />
          </Routes>
        </MemoryRouter>,
      );
    });

    await flushEffects();

    await act(async () => {
      MockWebSocket.instances[0].onopen();
      jest.advanceTimersByTime(300);
    });

    const backButton = Array.from(container.querySelectorAll('button')).find(
      (candidate) => candidate.textContent?.includes('返回'),
    );
    expect(backButton).toBeTruthy();

    await act(async () => {
      backButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(container.textContent).toContain('pod detail page');
  });
});
