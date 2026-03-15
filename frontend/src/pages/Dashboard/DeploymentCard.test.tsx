import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import { createRoot, Root } from 'react-dom/client';
import DeploymentCard from './DeploymentCard';
import { resumeDeployment } from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

jest.mock('../../components/GlassCard', () => ({ children }: any) => <div>{children}</div>);
jest.mock('../../components/StatusBadge', () => ({ status }: any) => <span>{status}</span>);
jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    deleteDeployment: fn(),
    resumeDeployment: fn(),
  };
});

const flushEffects = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('DeploymentCard suspend state', () => {
  let container: HTMLDivElement;
  let root: Root;
  const onUpdate = jest.fn();

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

    (resumeDeployment as any).mockResolvedValue({});
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    container.remove();
    onUpdate.mockReset();
    jest.clearAllMocks();
  });

  it('renders suspended state and resumes the workload', async () => {
    await act(async () => {
      root.render(
        <DeploymentCard
          deployment={{
            id: 'deploy-alice-train',
            name: 'deploy-alice-train',
            status: 'Suspended',
            suspended: true,
            replicas: 0,
            readyReplicas: 0,
            gpuCount: 0,
            cpu: '4',
            memory: '8Gi',
            createdAt: '2026-03-15T10:00:00Z',
            pods: [],
          }}
          onUpdate={onUpdate}
        />,
      );
    });

    await flushEffects();

    expect(container.textContent).toContain('挂起');

    const resumeButton = Array.from(container.querySelectorAll('*')).find(
      (candidate) => candidate.textContent?.replace(/\s+/g, '') === '恢复',
    );
    expect(resumeButton).toBeTruthy();

    await act(async () => {
      resumeButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await flushEffects();

    expect(resumeDeployment).toHaveBeenCalledWith('deploy-alice-train');
    expect(onUpdate).toHaveBeenCalled();
  });
});
