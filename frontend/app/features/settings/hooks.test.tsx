// @vitest-environment jsdom
import { act,renderHook,waitFor } from '@testing-library/react';
import { beforeEach,describe,expect,test,vi } from 'vitest';
import type { OperationResponse,SystemSettings } from './api';
import { getSystemSettings,updateLoginCredentials,updateSystemSettings,verifySession } from './api';
import { useSettings } from './hooks';

vi.mock('./api', /* settingsApiMockFactory 提供系统设置与登录凭据 API 替身。 */ () => ({
  getSystemSettings: vi.fn(),
  updateLoginCredentials: vi.fn(),
  updateSystemSettings: vi.fn(),
  verifySession: vi.fn(),
}));

// getSettingsMock 是系统设置读取请求的可控替身。
const getSettingsMock = vi.mocked(getSystemSettings);
// updateCredentialsMock 是登录凭据更新请求的可控替身。
const updateCredentialsMock = vi.mocked(updateLoginCredentials);
// updateSettingsMock 是系统设置保存请求的可控替身。
const updateSettingsMock = vi.mocked(updateSystemSettings);
// verifySessionMock 是会话校验请求的可控替身。
const verifySessionMock = vi.mocked(verifySession);
// settingsFixture 是覆盖验证码和日志字段的系统配置。
const settingsFixture: SystemSettings = { 'captcha.remote_service_url': 'https://captcha.example.com', log_level: 'info' };
// noopReload 是凭据保存成功后页面重载的浏览器行为替身。
const noopReload = vi.fn();

describe('useSettings', /* 当前回调验证系统设置和登录凭据流程。 */ () => {
  beforeEach(/* 当前回调重置系统设置 API 替身和浏览器副作用。 */ () => {
    vi.clearAllMocks();
    getSettingsMock.mockResolvedValue(settingsFixture);
    verifySessionMock.mockResolvedValue({ authenticated: true, username: 'admin' });
    updateSettingsMock.mockResolvedValue({ success: true });
    updateCredentialsMock.mockResolvedValue({ success: true, message: '凭据已更新' });
    vi.spyOn(window, 'alert').mockImplementation(/* alertImplementation 屏蔽设置保存提示。 */ () => undefined);
    Object.defineProperty(window, 'location', { configurable: true, value: { ...window.location, reload: noopReload } });
  });

  test('加载设置后可以保存配置', /* 当前回调验证系统设置成功加载和保存路径。 */ async () => {
    // hook 是系统设置 Hook 的渲染结果。
    const hook = renderHook(/* settingsHookFactory 创建系统设置 Hook 实例。 */ () => useSettings());
    await waitFor(/* statusAssertion 等待系统设置加载成功。 */ () => expect(hook.result.current.requestStatus).toBe('success'));
    expect(hook.result.current.settings).toMatchObject(settingsFixture);
    await act(/* saveAction 执行系统设置保存动作。 */ async () => hook.result.current.handleSave());
    expect(updateSettingsMock).toHaveBeenCalledWith(expect.objectContaining(settingsFixture), expect.objectContaining({ signal: expect.any(AbortSignal) }));
    expect(window.alert).toHaveBeenCalledWith('系统配置已保存');
  });

  test('凭据校验失败和服务拒绝都写入错误提示', /* 当前回调验证登录凭据校验和后端拒绝路径。 */ async () => {
    // hook 是登录凭据校验场景下的设置 Hook 渲染结果。
    const hook = renderHook(/* credentialsHookFactory 创建凭据测试使用的 Hook 实例。 */ () => useSettings());
    await waitFor(/* statusAssertion 等待系统设置加载成功。 */ () => expect(hook.result.current.requestStatus).toBe('success'));
    const invalidEvent = { preventDefault: vi.fn() } as unknown as React.FormEvent; // invalidEvent 表示不完整凭据表单事件。
    await act(/* invalidAction 提交不完整凭据表单。 */ async () => hook.result.current.handleCredentialsSave(invalidEvent));
    expect(hook.result.current.credentialsMessage?.type).toBe('error');
    await act(/* credentialsAction 写入可提交的凭据草稿。 */ () => hook.result.current.setCredentials({ new_username: 'new-admin', current_password: 'old-password', new_password: 'new-password', confirm_password: 'new-password' }));
    const rejectedResponse: OperationResponse = { success: false, message: '当前密码错误' }; // rejectedResponse 表示后端拒绝凭据更新。
    updateCredentialsMock.mockResolvedValueOnce(rejectedResponse);
    await act(/* rejectedAction 提交合法表单并验证服务端错误。 */ async () => hook.result.current.handleCredentialsSave(invalidEvent));
    expect(hook.result.current.credentialsMessage).toEqual({ type: 'error', text: '当前密码错误' });
  });

  test('设置读取失败时展示加载错误', /* 当前回调验证系统设置初始请求失败路径。 */ async () => {
    getSettingsMock.mockRejectedValueOnce(new Error('设置服务失败'));
    // hook 是设置读取失败场景下的系统设置 Hook 渲染结果。
    const hook = renderHook(/* failedSettingsHookFactory 创建设置读取失败的 Hook 实例。 */ () => useSettings());
    await waitFor(/* errorAssertion 等待设置错误状态写入。 */ () => expect(hook.result.current.requestStatus).toBe('error'));
    expect(hook.result.current.loadError).toBe('设置服务失败');
    expect(hook.result.current.settings).toBeNull();
    hook.unmount();
  });
});
