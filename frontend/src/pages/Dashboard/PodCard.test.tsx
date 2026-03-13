import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import PodCard from './PodCard';

declare const jest: typeof import('@jest/globals').jest;

jest.mock('../../components/GlassCard', () => ({ children }: any) => <div>{children}</div>);
jest.mock('../../components/StatusBadge', () => ({ status }: any) => <span>{status}</span>);
jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    deletePod: fn(),
    downloadPodYAML: fn(),
    extendPod: fn(),
  };
});

const flushEffects = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('PodCard code-server entry', () => {
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

  it('opens code-server in a new tab when the pod reports a ready web IDE', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <PodCard
            pod={{
              id: 'pod-alice-dev',
              name: 'pod-alice-dev',
              namespace: 'user-alice',
              status: 'Running',
              cpu: '4',
              memory: '8Gi',
              gpuCount: 0,
              connections: {
                apps: {
                  codeServerURL: '/api/pods/pod-alice-dev/apps/code-server',
                  codeServerReady: true,
                  codeServerStatus: 'enabled',
                },
              },
            }}
            onUpdate={() => undefined}
          />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const button = Array.from(container.querySelectorAll('button')).find(
      (candidate) => candidate.textContent?.includes('code-server'),
    );
    expect(button).toBeTruthy();

    await act(async () => {
      button?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(openSpy).toHaveBeenCalledWith('/api/pods/pod-alice-dev/apps/code-server', '_blank', 'noopener,noreferrer');
  });

  it('opens web shell in a new tab when the pod reports a ready shell entry', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <PodCard
            pod={{
              id: 'pod-alice-dev',
              name: 'pod-alice-dev',
              namespace: 'user-alice',
              status: 'Running',
              cpu: '4',
              memory: '8Gi',
              gpuCount: 0,
              connections: {
                apps: {
                  webShellURL: '/pods/pod-alice-dev/webshell',
                  webShellReady: true,
                  webShellStatus: 'enabled',
                },
              },
            }}
            onUpdate={() => undefined}
          />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const button = Array.from(container.querySelectorAll('button')).find(
      (candidate) => candidate.textContent?.includes('Web Shell'),
    );
    expect(button).toBeTruthy();

    await act(async () => {
      button?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(openSpy).toHaveBeenCalledWith('/pods/pod-alice-dev/webshell', '_blank', 'noopener,noreferrer');
  });

  it('renders a disabled code-server button when the web IDE is not ready', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <PodCard
            pod={{
              id: 'pod-alice-dev',
              name: 'pod-alice-dev',
              namespace: 'user-alice',
              status: 'Running',
              cpu: '4',
              memory: '8Gi',
              gpuCount: 0,
              connections: {
                apps: {
                  codeServerURL: '/api/pods/pod-alice-dev/apps/code-server',
                  codeServerReady: false,
                  codeServerStatus: 'starting',
                },
              },
            }}
            onUpdate={() => undefined}
          />
        </MemoryRouter>,
      );
    });

    await flushEffects();

    const button = Array.from(container.querySelectorAll('button')).find(
      (candidate) => candidate.textContent?.includes('code-server'),
    );
    expect(button).toBeTruthy();
    expect(button?.hasAttribute('disabled')).toBe(true);
    expect(container.textContent).toContain('启动中');
  });
});
