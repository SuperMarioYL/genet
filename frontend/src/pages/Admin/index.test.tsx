import React, { act } from 'react';
import { afterEach, beforeEach, describe, expect, it } from '@jest/globals';
import type { MockedFunction } from 'jest-mock';
import { createRoot, Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import AdminPage from './index';
import {
  deleteAdminUser,
  getAdminMe,
  getAdminOverview,
  listAdminNodePools,
  listAdminUserPools,
} from '../../services/api';

declare const jest: typeof import('@jest/globals').jest;

jest.mock('../../components/GlassCard', () => ({ children, title }: any) => (
  <div>
    {title ? <div>{title}</div> : null}
    {children}
  </div>
));
jest.mock('../../components/ThemeToggle', () => () => <button type="button">theme</button>);
jest.mock('../AdminAPIKeys/Panel', () => ({ AdminAPIKeysPanel: () => <div>apikey panel</div> }));
jest.mock('../../services/api', () => {
  const { fn } = require('jest-mock');
  return {
    deleteAdminUser: fn(),
    getAdminMe: fn(),
    getAdminOverview: fn(),
    listAdminNodePools: fn(),
    listAdminUserPools: fn(),
  };
});

const mockedDeleteAdminUser = deleteAdminUser as MockedFunction<typeof deleteAdminUser>;
const mockedGetAdminMe = getAdminMe as MockedFunction<typeof getAdminMe>;
const mockedGetAdminOverview = getAdminOverview as MockedFunction<typeof getAdminOverview>;
const mockedListAdminNodePools = listAdminNodePools as MockedFunction<typeof listAdminNodePools>;
const mockedListAdminUserPools = listAdminUserPools as MockedFunction<typeof listAdminUserPools>;

describe('AdminPage', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
    mockedGetAdminMe.mockResolvedValue({ username: 'admin', email: 'admin@example.com', isAdmin: true } as any);
    mockedGetAdminOverview.mockResolvedValue({
      nodeSummary: { shared: 2, exclusive: 1 },
      userSummary: { shared: 3, exclusive: 1 },
    } as any);
    mockedListAdminNodePools.mockResolvedValue({
      nodes: [{ nodeName: 'node-a', nodeIP: '10.0.0.1', poolType: 'shared' }],
    } as any);
    mockedListAdminUserPools.mockResolvedValue({
      users: [{ username: 'alice', poolType: 'shared' }],
    } as any);
    mockedDeleteAdminUser.mockResolvedValue({ message: 'ok' } as any);
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

  it('renders admin tabs and fetched pool data', async () => {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <AdminPage />
        </MemoryRouter>,
      );
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('卡池管理');
    expect(container.textContent).toContain('用户管理');
    expect(container.textContent).toContain('API Key 管理');
    expect(mockedListAdminNodePools).toHaveBeenCalled();
    expect(mockedListAdminUserPools).toHaveBeenCalled();
  });

  it('deletes a user from the admin user list', async () => {
    const confirmSpy = jest.spyOn(window, 'confirm').mockReturnValue(true);

    await act(async () => {
      root.render(
        <MemoryRouter>
          <AdminPage />
        </MemoryRouter>,
      );
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const userTab = Array.from(container.querySelectorAll('*')).find((node) => node.textContent === '用户管理') as HTMLElement;
    await act(async () => {
      userTab.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    await act(async () => {
      await Promise.resolve();
    });

    const deleteButton = Array.from(container.querySelectorAll('button')).find((button) => button.textContent?.includes('删除用户')) as HTMLButtonElement;
    await act(async () => {
      deleteButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(confirmSpy).toHaveBeenCalled();
    expect(mockedDeleteAdminUser).toHaveBeenCalledWith('alice');

    confirmSpy.mockRestore();
  });
});
