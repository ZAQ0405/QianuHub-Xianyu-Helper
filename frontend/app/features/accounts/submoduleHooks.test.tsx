// @vitest-environment jsdom
import { act,renderHook } from '@testing-library/react';
import { useState } from 'react';
import { describe,expect,test,vi } from 'vitest';
import type { AccountDetail } from './api';
import { getAccountBindings,getLongLoginSettings,getNotificationChannels } from './api';
import { useAccountSubmodules } from './submoduleHooks';
import type { AccountEditForm } from './types';

vi.mock('./api', /* accountsSubmoduleApiMockFactory 提供编辑弹窗依赖的通知与长登录 API 替身。 */ () => ({
  getAccountBindings: vi.fn(),
  getLongLoginSettings: vi.fn(),
  getNotificationChannels: vi.fn(),
}));

// accountFixture 是编辑子模块使用的最小账号摘要。
const accountFixture = { id: 'account-1', enabled: true, auto_confirm: false, remark: '测试账号', pause_duration: 0 } as AccountDetail;
// editFixture 是编辑弹窗使用的最小表单状态。
const editFixture: AccountEditForm = { remark: '', cookie: '', auto_confirm: false, pause_duration: 0, username: '', login_password: '', show_browser: false, showLoginPassword: false, clear_password: false };

describe('useAccountSubmodules', /* 当前回调验证账号编辑、通知绑定和长登录子模块。 */ () => {
  test('打开编辑弹窗时加载通知绑定和长登录状态', /* 当前回调验证账号编辑所需的两个活动子模块。 */ async () => {
    vi.mocked(getAccountBindings).mockResolvedValue([1]);
    vi.mocked(getNotificationChannels).mockResolvedValue({ success: true, data: [{ id: '1', name: '邮件', type: 'email', enabled: true }] } as never);
    vi.mocked(getLongLoginSettings).mockResolvedValue({ can_open_long_login: true, enabled: true } as never);
    // hook 是账号子模块 Hook 的真实 React 状态实例。
    const hook = renderHook(/* submoduleHookFactory 创建账号子模块 Hook 实例。 */ () => {
      const [editingAccount, setEditingAccount] = useState<AccountDetail | null>(null); // editingAccount 保存当前编辑账号。
      const [activeModal, setActiveModal] = useState<'edit' | null>(null); // activeModal 保存当前编辑弹窗类型。
      const [editForm, setEditForm] = useState(editFixture); // editForm 保存编辑表单草稿。
      return { activeModal, editForm, ...useAccountSubmodules({ editingAccount, setEditingAccount, setActiveModal, editForm, setEditForm, loadAccounts: /* loadAccounts 刷新账号列表的测试替身。 */ async () => undefined }) };
    });
    await act(/* openEditAction 打开账号编辑弹窗并加载依赖。 */ async () => hook.result.current.openEditModal(accountFixture));
    expect(hook.result.current.activeModal).toBe('edit');
    expect(hook.result.current.editForm.remark).toBe('测试账号');
    expect(hook.result.current.selectedChannelIds).toEqual([1]);
    expect(hook.result.current.longLogin.enabled).toBe(true);
    hook.unmount();
  });
});
