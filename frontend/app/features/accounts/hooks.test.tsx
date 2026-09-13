// @vitest-environment jsdom
import { act,renderHook,waitFor } from '@testing-library/react';
import { beforeEach,describe,expect,test,vi } from 'vitest';
import type { AccountDetail } from './api';
import { getAccountDetails,getAccountRuntimeStatuses } from './api';
import { useAccountsData } from './hooks';

vi.mock('./api', /* accountsApiMockFactory 提供账号详情与运行状态 API 替身。 */ () => ({
  getAccountDetails: vi.fn(),
  getAccountRuntimeStatuses: vi.fn(),
}));

// getDetailsMock 是账号详情请求的可控替身。
const getDetailsMock = vi.mocked(getAccountDetails);
// getRuntimeMock 是账号运行状态请求的可控替身。
const getRuntimeMock = vi.mocked(getAccountRuntimeStatuses);
// accountFixture 是账号数据 Hook 使用的最小账号摘要。
const accountFixture: AccountDetail = { id: 'account-1', enabled: true, auto_confirm: false, remark: '测试账号' };

describe('useAccountsData', /* 当前回调验证账号详情与运行状态轮询。 */ () => {
  beforeEach(/* 当前回调重置账号 API 替身和定时器。 */ () => {
    vi.clearAllMocks();
    getDetailsMock.mockResolvedValue([accountFixture]);
    getRuntimeMock.mockResolvedValue({ 'account-1': { state: 'online', connected: true, failures: 0, message: '在线', updated_at: '2026-08-15T00:00:00Z' } });
  });

  test('加载账号并合并运行状态', /* 当前回调验证账号列表成功加载路径。 */ async () => {
    // hook 是账号数据 Hook 的渲染结果。
    const hook = renderHook(/* accountsHookFactory 创建账号数据 Hook 实例。 */ () => useAccountsData());
    await waitFor(/* loadingAssertion 等待账号详情加载结束。 */ () => expect(hook.result.current.loading).toBe(false));
    await waitFor(/* runtimeAssertion 等待运行状态轮询写入账号列表。 */ () => expect(hook.result.current.accounts[0]).toMatchObject({ id: 'account-1', runtime_state: 'online', runtime_connected: true }));
    hook.unmount();
  });

  test('账号详情失败时结束加载并保留空列表', /* 当前回调验证账号详情错误分支。 */ async () => {
    getDetailsMock.mockRejectedValueOnce(new Error('账号服务失败'));
    // hook 是账号详情失败场景下的 Hook 渲染结果。
    const hook = renderHook(/* failedDetailsHookFactory 创建账号详情失败的 Hook 实例。 */ () => useAccountsData());
    await waitFor(/* loadingAssertion 等待账号详情错误状态收束。 */ () => expect(hook.result.current.loading).toBe(false));
    expect(hook.result.current.accounts).toEqual([]);
    hook.unmount();
  });

  test('运行状态轮询失败时保留账号列表', /* 当前回调验证运行状态错误不会清空账号。 */ async () => {
    getRuntimeMock.mockRejectedValue(new Error('状态服务失败'));
    // hook 是运行状态轮询失败场景下的 Hook 渲染结果。
    const hook = renderHook(/* failedRuntimeHookFactory 创建运行状态失败的 Hook 实例。 */ () => useAccountsData());
    await waitFor(/* loadingAssertion 等待账号详情加载结束。 */ () => expect(hook.result.current.loading).toBe(false));
    expect(hook.result.current.accounts).toEqual([accountFixture]);
    await act(/* flushAction 等待一次异步轮询副作用完成。 */ async () => { await Promise.resolve(); });
    hook.unmount();
  });
});
