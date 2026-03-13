// @ts-nocheck
import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import WebShellPage from './index';
import { createWebShellSession, deleteWebShellSession } from '../../services/api';

const mockTerminalState = {
  open: jest.fn(),
  loadAddon: jest.fn(),
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

jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    createWebShellSession: fn(),
    deleteWebShellSession: fn(),
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
    mockTerminalState.write.mockClear();
    mockTerminalState.dispose.mockClear();
    mockTerminalState.onDataCallback = undefined;

    createWebShellSession.mockResolvedValue({
      sessionId: 'session-1',
      webSocketURL: '/api/pods/pod-alice-dev/webshell/sessions/session-1/ws',
      container: 'workspace',
      shell: '/bin/sh',
      cols: 120,
      rows: 40,
      expiresAt: '2026-03-13T11:00:00Z',
    });
    deleteWebShellSession.mockResolvedValue({ message: 'closed' });

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
    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toContain('/api/pods/pod-alice-dev/webshell/sessions/session-1/ws');

    await act(async () => {
      MockWebSocket.instances[0].onmessage({ data: new Uint8Array([101, 99, 104, 111]) });
    });

    expect(mockTerminalState.write).toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });

    expect(deleteWebShellSession).toHaveBeenCalledWith('pod-alice-dev', 'session-1');
  });
});
