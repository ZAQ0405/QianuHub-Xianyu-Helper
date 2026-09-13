import React from 'react';
import {
  Bell, Box, CreditCard, LayoutDashboard,
  LogOut, MessageCircleMore, Settings, ShoppingBag, Users, Zap,
} from 'lucide-react';
import { QianuHubBrandIcon } from './QianuHubLogo';

// SidebarProps 描述侧边栏所需的导航和会话回调。
interface SidebarProps {
  /** activeTab 表示当前Tab。 */ activeTab: string;
  /** isAdmin 表示当前用户是否为管理员。 */ isAdmin?: boolean;
  /** collapsed 表示侧边栏是否折叠。 */ collapsed: boolean;
  /** onToggleCollapsed 表示切换侧边栏折叠状态的回调。 */ onToggleCollapsed: () => void;
  /** onNavigate 表示切换主导航页面的回调。 */ onNavigate: (tab: string) => void;
  /** onLogout 表示注销当前会话的回调。 */ onLogout: () => void;
  /** hasUnreadChatMessage 表示在线聊天入口是否仍有未读消息，需要展示红点。 */ hasUnreadChatMessage?: boolean;
}

// Sidebar 渲染侧边栏导航组件。
const Sidebar: React.FC<SidebarProps> = ({
  activeTab, isAdmin = false, collapsed, onToggleCollapsed, onNavigate, onLogout, hasUnreadChatMessage = false,
}) => {
  // menuItems 侧边栏菜单项。
  const menuItems = [
    { id: 'dashboard', icon: LayoutDashboard, label: '仪表盘' },
    { id: 'accounts', icon: Users, label: '账号管理' },
    { id: 'chat', icon: MessageCircleMore, label: '在线聊天' },
    { id: 'cards', icon: CreditCard, label: '卡密库存' },
    { id: 'items', icon: Box, label: '商品列表' },
    { id: 'orders', icon: ShoppingBag, label: '订单管理' },
    { id: 'rules', icon: Zap, label: '自动化规则' },
    { id: 'notifications', icon: Bell, label: '通知设置' },
    ...(isAdmin ? [{ id: 'settings', icon: Settings, label: '系统设置' }] : []),
  ];
  // handleSidebarDoubleClick 仅响应侧边栏空白区域的双击，避免菜单、链接和按钮双击误触发折叠。
  const handleSidebarDoubleClick = (event: React.MouseEvent<HTMLElement>): void => {
    const target = event.target as HTMLElement;
    if (target.closest('a,button,input,select,textarea')) return;
    onToggleCollapsed();
  };
  return (
    <aside
      className={`fixed inset-y-0 left-0 z-20 flex select-none flex-col border-r border-slate-200 bg-white shadow-sidebar transition-[width] duration-300 ${collapsed ? 'w-16' : 'w-64'}`}
      onDoubleClick={handleSidebarDoubleClick}
      aria-label="侧边栏，双击空白区域展开或收起"
    >
      <a
        href="https://qianqiu.org"
        aria-label="访问 QianuHub 闲鱼助手官网"
        className={`group flex h-16 items-center border-b border-slate-100 transition-colors hover:bg-brand-50/60 ${collapsed ? 'justify-center px-2' : 'gap-3 px-5'}`}
      >
        <QianuHubBrandIcon sizeClass="h-10 w-10" />
        {!collapsed && (
          <span className="flex min-w-0 flex-col text-left">
            <span className="truncate text-[13px] font-extrabold tracking-tight text-slate-900">QianuHub 闲鱼助手</span>
            <span className="text-[8px] font-bold tracking-[0.24em] text-brand-600">QIANUHUB · OPERATIONS</span>
          </span>
        )}
      </a>

      <nav className={`flex-1 space-y-1 overflow-y-auto pt-4 ${collapsed ? 'px-2' : 'px-3'}`} aria-label="主导航">
        {menuItems.map(/* 当前回调处理集合中的单个元素。 */ (item) => {
          // Icon 渲染Icon React 组件。
          const Icon = item.icon;
          // active 当前状态。
          const active = activeTab === item.id;
          return (
            <React.Fragment key={item.id}>
              <button
              type="button"
              title={collapsed ? item.label : undefined}
              aria-label={item.label}
              aria-current={active ? 'page' : undefined}
              onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => onNavigate(item.id)}
              className={`group relative flex h-10 w-full items-center rounded-lg transition-colors ${collapsed ? 'justify-center' : 'gap-3 px-3'} ${
                active
                  ? 'bg-brand-50 text-brand-800 before:absolute before:left-0 before:top-2 before:h-6 before:w-0.5 before:rounded-full before:bg-brand'
                  : 'text-slate-500 hover:bg-slate-50 hover:text-slate-900'
              }`}
            >
              <span className="relative flex h-[19px] w-[19px] shrink-0">
                <Icon className={`h-[18px] w-[18px] ${active ? 'text-brand-700' : 'text-slate-400 group-hover:text-slate-700'}`} />
                {item.id === 'chat' && hasUnreadChatMessage && <span className="absolute -right-1 -top-1 h-3 w-3 rounded-full border-2 border-white bg-red-500" role="status" aria-label="在线聊天有未读消息" />}
              </span>
              {!collapsed && <span className={`truncate text-[13px] ${active ? 'font-bold' : 'font-semibold'}`}>{item.label}</span>}
              {active && !collapsed && <span className="ml-auto h-1.5 w-1.5 rounded-full bg-brand" />}
              </button>
            </React.Fragment>
          );
        })}
      </nav>

      <div>
        <div className={`space-y-1 p-2 ${collapsed ? '' : 'p-3'}`}>
          <button
            type="button"
            onClick={onLogout}
            title={collapsed ? '退出登录' : undefined}
            aria-label="退出登录"
            className={`flex h-9 w-full items-center rounded-lg text-slate-500 transition-colors hover:bg-red-50 hover:text-red-600 ${collapsed ? 'justify-center' : 'gap-3 px-3'}`}
          >
            <LogOut className="h-5 w-5" />
            {!collapsed && <span className="text-sm font-bold">退出登录</span>}
          </button>
        </div>
      </div>
    </aside>
  );
};

export default Sidebar;
