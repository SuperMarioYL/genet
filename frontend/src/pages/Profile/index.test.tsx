import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import ProfilePage from './index';
import { getAuthStatus } from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

jest.mock('../../components/GlassCard', () => ({ children, title }: any) => (
  <div>
    {title ? <div>{title}</div> : null}
    {children}
  </div>
));
jest.mock('../../components/ThemeToggle', () => () => <button type="button">theme</button>);
jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    getAuthStatus: fn(),
  };
});

const mockedGetAuthStatus = getAuthStatus as MockedFunction<typeof getAuthStatus>;

describe('ProfilePage', () => {
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
    mockedGetAuthStatus.mockResolvedValue({
      authenticated: true,
      username: 'alice',
      email: 'alice@example.com',
      isAdmin: false,
      poolType: 'exclusive',
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

  it('renders the current pool type from auth status', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <ProfilePage />
        </MemoryRouter>,
      );
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('alice');
    expect(container.textContent).toContain('exclusive');
  });
});
