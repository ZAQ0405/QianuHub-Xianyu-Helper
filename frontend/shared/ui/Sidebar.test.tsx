// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, test, vi } from 'vitest';
import Sidebar from './Sidebar';

describe('Sidebar', /* 当前测试组验证全局聊天新消息状态在侧边栏中的可见标记。 */ () => {
  test('移除发货模板入口并保留自动化规则入口', /* 当前回调验证已下线功能不再出现在侧边栏。 */ () => {
    // navigationHandler 是侧边栏导航测试使用的无副作用回调。
    const navigationHandler = vi.fn();
    // logoutHandler 是侧边栏注销测试使用的无副作用回调。
    const logoutHandler = vi.fn();
    // collapseHandler 是侧边栏折叠测试使用的无副作用回调。
    const collapseHandler = vi.fn();
    render(<Sidebar activeTab="dashboard" collapsed={false} onToggleCollapsed={collapseHandler} onNavigate={navigationHandler} onLogout={logoutHandler} />);

    // navigationLabels 保存侧边栏按钮的可见顺序，包含业务入口和底部操作入口。
    const navigationLabels = screen.getAllByRole('button').map(/* button 是当前侧边栏中的可见按钮。 */ button => button.textContent?.trim() || '');
    expect(navigationLabels).not.toContain('发货模板');
    expect(navigationLabels).toContain('自动化规则');
    expect(navigationLabels).not.toContain('收起侧边栏');
  });

  test('双击侧边栏空白区域切换折叠状态，交互控件双击不切换', () => {
    const navigationHandler = vi.fn();
    const logoutHandler = vi.fn();
    const collapseHandler = vi.fn();
    const view = render(<Sidebar activeTab="dashboard" collapsed={false} onToggleCollapsed={collapseHandler} onNavigate={navigationHandler} onLogout={logoutHandler} />);
    const sidebar = view.container.querySelector('aside');
    expect(sidebar).not.toBeNull();
    if (!sidebar) return;
    expect(sidebar.classList.contains('select-none')).toBe(true);

    fireEvent.doubleClick(sidebar);
    expect(collapseHandler).toHaveBeenCalledTimes(1);

    const logoutButton = sidebar.querySelector('button[aria-label="退出登录"]');
    const brandLink = sidebar.querySelector('a[aria-label="访问 QianuHub 闲鱼助手官网"]');
    expect(logoutButton).not.toBeNull();
    expect(brandLink).not.toBeNull();
    if (!logoutButton || !brandLink) return;
    fireEvent.doubleClick(logoutButton);
    fireEvent.doubleClick(brandLink);
    expect(collapseHandler).toHaveBeenCalledTimes(1);
  });

  test('品牌入口指向 QianuHub 官网且不展示运行版本信息', () => {
    const navigationHandler = vi.fn();
    const logoutHandler = vi.fn();
    const collapseHandler = vi.fn();
    const view = render(<Sidebar activeTab="dashboard" collapsed={false} onToggleCollapsed={collapseHandler} onNavigate={navigationHandler} onLogout={logoutHandler} />);

    const brandLink = view.container.querySelector('a[href="https://qianqiu.org"]');
    expect(brandLink).not.toBeNull();
    if (!brandLink) return;
    expect(brandLink.getAttribute('href')).toBe('https://qianqiu.org');
    expect(view.container.textContent).toContain('QianuHub 闲鱼助手');
    expect(view.container.textContent).not.toContain('运行版本');
    expect(view.container.textContent).not.toContain('unknown');
  });

  test('未查看聊天消息时仅在在线聊天入口显示红点', /* 当前回调验证红点出现与消失不依赖页面标题状态。 */ () => {
    // navigationHandler 是侧边栏导航测试使用的无副作用回调。
    const navigationHandler = vi.fn();
    // logoutHandler 是侧边栏注销测试使用的无副作用回调。
    const logoutHandler = vi.fn();
    // collapseHandler 是侧边栏折叠测试使用的无副作用回调。
    const collapseHandler = vi.fn();
    // view 保存带有新消息状态的侧边栏渲染控制器。
    const view = render(
      <Sidebar
        activeTab="dashboard"
        collapsed={false}
        onToggleCollapsed={collapseHandler}
        onNavigate={navigationHandler}
        onLogout={logoutHandler}
        hasUnreadChatMessage
      />,
    );
    // unreadIndicator 表示挂载在聊天图标右上角的未读提示元素。
    const unreadIndicator = screen.getByLabelText('在线聊天有未读消息');
    expect(unreadIndicator).toBeTruthy();
    expect(unreadIndicator.classList.contains('absolute')).toBe(true);
    expect(unreadIndicator.classList.contains('-right-1')).toBe(true);
    expect(unreadIndicator.classList.contains('-top-1')).toBe(true);
    expect(unreadIndicator.classList.contains('h-3')).toBe(true);
    expect(unreadIndicator.classList.contains('w-3')).toBe(true);
    view.rerender(
      <Sidebar
        activeTab="dashboard"
        collapsed={false}
        onToggleCollapsed={collapseHandler}
        onNavigate={navigationHandler}
        onLogout={logoutHandler}
        hasUnreadChatMessage={false}
      />,
    );
    expect(screen.queryByLabelText('在线聊天有未读消息')).toBeNull();
  });
});
