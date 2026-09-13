import { ArrowRight,Box,CheckCircle2,CircleDashed,Edit,Filter,Link2,LocateFixed,PackagePlus,Plus,RefreshCw,Save,Search,ShoppingBag,Trash2,User,X } from 'lucide-react';
import React,{ useCallback,useEffect,useMemo,useRef,useState } from 'react';
import { createPortal } from 'react-dom';
import type { AccountDetail,Item,PublishLocation,ShippingRule } from '../api';
import {
getAccountDetails,
getItems,
getShippingRules,
} from '../api';
import { ManualLocationPicker } from '../components/ManualLocationPicker';
import { useItemActions } from '../itemActions';
import type { ItemListProps } from '../types';

// formatItemPrice 将商品价格转换为本地化展示文本。
const formatItemPrice = (price?: string) => {
  // value 值。
  const value = String(price || '').trim();
  if (!value) return '-';
  return /^[¥￥]/.test(value) ? value : `¥${value}`;
};

// ItemList 渲染商品列表组件。
const ItemList: React.FC<ItemListProps> = ({ onConfigureDelivery, publishImagesEditor: ImagesEditor }) => {
  // [items, 解构得到当前 Hook 返回的状态和操作函数。
  const [items, setItems] = useState<Item[]>([]);
  // [shippingRules, 解构得到当前 Hook 返回的状态和操作函数。
  const [shippingRules, setShippingRules] = useState<ShippingRule[]>([]);
  // [accounts, 解构得到当前 Hook 返回的状态和操作函数。
  const [accounts, setAccounts] = useState<AccountDetail[]>([]);
  // [selectedAccount, 解构得到当前 Hook 返回的状态和操作函数。
  const [selectedAccount, setSelectedAccount] = useState<string>('');
  // [accountFilter, 解构得到当前 Hook 返回的状态和操作函数。
  const [accountFilter, setAccountFilter] = useState<string>('');
  // itemsRequestGeneration 标识商品列表最新一次读取，旧响应不得覆盖较新的同步或刷新结果。
  const itemsRequestGeneration = useRef(0);
  // shippingRulesRequestGeneration 标识发货规则最新一次读取，旧响应不得覆盖较新的规则配置。
  const shippingRulesRequestGeneration = useRef(0);
  // loadItems 刷新商品列表，供普通操作和刷新结果复用。
  const loadItems = useCallback(/* 当前回调封装可复用的交互处理逻辑。 */ async () => {
    // requestGeneration 是本次商品刷新请求的单调递增代次。
    const requestGeneration = ++itemsRequestGeneration.current;
    // itemsList 商品列表列表，负责当前功能中的对应处理。
    const itemsList = await getItems();
    if (requestGeneration === itemsRequestGeneration.current) setItems(itemsList);
  }, []);

  // loadShippingRules 刷新商品关联的自动化规则。
  const loadShippingRules = useCallback(/* 当前回调封装可复用的交互处理逻辑。 */ async () => {
    // requestGeneration 是本次规则刷新请求的单调递增代次。
    const requestGeneration = ++shippingRulesRequestGeneration.current;
    // rules 是当前规则读取返回的非敏感自动化规则集合。
    const rules = await getShippingRules();
    if (requestGeneration === shippingRulesRequestGeneration.current) setShippingRules(rules);
  }, []);

  // itemActions 商品 feature 提供普通商品操作、发布表单和定位动作。
  const itemActions = useItemActions({
    selectedAccount,
    setSelectedAccount,
    setItems,
    loadItems,
    loadShippingRules,
    onConfigureDelivery,
  });
  // 解构商品动作，保持页面 JSX 只负责布局和表单字段组合。
  const {
    loading,
    publishing,
    showEditModal,
    setShowEditModal,
    showAddModal,
    setShowAddModal,
    showPublishModal,
    setShowPublishModal,
    locationLoading,
    publishLocations,
    setPublishLocations,
    publishLocation,
    setPublishLocation,
    publishCategoryKeyword,
    setPublishCategoryKeyword,
    publishCategoryLoading,
    publishCategory,
    setPublishCategory,
    selectedItem,
    editForm,
    setEditForm,
    addForm,
    setAddForm,
    publishForm,
    setPublishForm,
    publishImagePreviews,
    handleSync,
    handleEdit,
    handleSaveEdit,
    handleDelete,
    handleAddItem,
    handlePublishItem,
    handleRecommendPublishCategory,
    openAddModal,
    openPublishModal,
    locateForPublish,
  } = itemActions;

  // ManualLocationTarget 表示手动地点选择完成后要回填的发布流程。
  type ManualLocationTarget = 'publish';
  // manualLocationTarget 保存当前手动地点弹窗服务的发布场景。
  const [manualLocationTarget, setManualLocationTarget] = useState<ManualLocationTarget | null>(null);
  // confirmManualLocation 将用户选中的高德 POI 写回对应发布表单。
  const confirmManualLocation = useCallback(/* confirmManualLocationCallback 将已确认的高德 POI 写入对应发布场景。 */ (location: PublishLocation) => {
    if (manualLocationTarget === 'publish') {
      setPublishLocations([location]);
      setPublishLocation(location);
    }
    setManualLocationTarget(null);
  }, [manualLocationTarget, setPublishLocation, setPublishLocations]);

  useEffect(/* 当前回调同步 React 副作用和资源生命周期。 */ () => {
    // controller 取消组件卸载前仍在执行的首屏并行请求。
    const controller = new AbortController();
    // initialItemsGeneration、initialRulesGeneration 分别记录首屏请求对应的列表与规则代次。
    const initialItemsGeneration = ++itemsRequestGeneration.current;
    const initialRulesGeneration = ++shippingRulesRequestGeneration.current;
    // active 标识当前组件实例是否仍接受首屏响应。
    let active = true;
    Promise.all([getAccountDetails({ signal: controller.signal }), getItems(undefined, { signal: controller.signal }), getShippingRules({ signal: controller.signal })])
      .then(/* 当前回调处理异步操作结果。 */ ([accountList, itemList, ruleList]) => {
        if (!active || controller.signal.aborted || initialItemsGeneration !== itemsRequestGeneration.current || initialRulesGeneration !== shippingRulesRequestGeneration.current) return;
        setAccounts(accountList);
        setItems(itemList);
        setShippingRules(ruleList);
      })
      .catch(/* 当前回调处理异步操作结果。 */ (e) => {
        if (!controller.signal.aborted) console.error('加载商品配置失败:', e);
      });
    return /* 首屏请求清理回调在卸载时取消请求并阻止状态回写。 */ () => {
      active = false;
      controller.abort();
    };
  }, []);

  // rulesForItem 规则列表For商品，负责当前功能中的对应处理。
  const rulesForItem = (item: Item) => shippingRules.filter(/* 当前回调处理集合中的单个元素。 */ rule =>
    rule.cookie_id === item.cookie_id && rule.item_id === item.item_id
  ).length > 0
    ? shippingRules.filter(/* 当前回调处理集合中的单个元素。 */ rule => rule.cookie_id === item.cookie_id && rule.item_id === item.item_id)
    : shippingRules.filter(/* 当前回调处理集合中的单个元素。 */ rule => rule.cookie_id === item.cookie_id && !rule.item_id);

  // accountMap 账号索引。
  const accountMap = useMemo(
    /* 当前回调处理集合中的单个元素。 */ () => new Map(accounts.map(/* 当前回调处理集合中的单个元素。 */ account => [account.id, account])),
    [accounts],
  );
  // visibleItems 可见商品列表。
  const visibleItems = useMemo(
    /* 当前回调处理集合中的单个元素。 */ () => accountFilter ? items.filter(/* 当前回调处理集合中的单个元素。 */ item => item.cookie_id === accountFilter) : items,
    [accountFilter, items],
  );
  // accountName 账号名称。
  const accountName = (cookieId: string) => {
    // account 账号。
    const account = accountMap.get(cookieId);
    // name 名称。
    const name = account?.remark || account?.nickname;
    return name ? `${name} · ${cookieId.slice(0, 6)}` : `账号 ${cookieId.slice(0, 8)}`;
  };
  // accountNickname 账号昵称。
  const accountNickname = (cookieId: string) => {
    // account 账号。
    const account = accountMap.get(cookieId);
    return account?.remark || account?.nickname || '未命名账号';
  };

  return (
    <div className="page-stack animate-fade-in">
      <div className="flex flex-col xl:flex-row xl:justify-between xl:items-center gap-4">
        <div>
          <h2 className="text-3xl font-bold text-gray-900">商品管理</h2>
          <p className="text-gray-500 mt-2 text-sm">监控并管理所有账号下的闲鱼商品。</p>
        </div>
        <div className="flex flex-wrap items-end gap-3">
            <div className="flex min-w-[200px] flex-col gap-1.5">
              <label htmlFor="item-account-filter" className="px-1 text-[11px] font-extrabold tracking-wide text-gray-500">
                商品列表筛选
              </label>
              <div className="relative">
                <Filter className="w-4 h-4 absolute left-4 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
                <select
                  id="item-account-filter"
                  aria-label="按账号筛选商品列表"
                  className="ios-input w-full pl-10 pr-9 py-3 rounded-xl text-sm"
                  value={accountFilter}
                  onChange={/* 当前回调处理用户交互或异步状态变化。 */ event => setAccountFilter(event.target.value)}
                >
                  <option value="">全部账号</option>
                  {accounts.map(/* 当前回调处理集合中的单个元素。 */ account => (
                    <option key={account.id} value={account.id}>{accountName(account.id)}</option>
                  ))}
                </select>
              </div>
            </div>
            <div className="flex min-w-[200px] flex-col gap-1.5">
              <label htmlFor="item-sync-account" className="px-1 text-[11px] font-extrabold tracking-wide text-gray-500">
                同步商品账号
              </label>
              <div className="relative">
                <User className="w-4 h-4 absolute left-4 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
                <select
                  id="item-sync-account"
                  aria-label="选择要同步商品的账号"
                  className="ios-input w-full pl-10 pr-9 py-3 rounded-xl text-sm"
                  value={selectedAccount}
                  onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setSelectedAccount(e.target.value)}
                >
                  <option value="">请选择账号</option>
                  {accounts.map(/* 当前回调处理集合中的单个元素。 */ acc => (
                      <option key={acc.id} value={acc.id}>{accountName(acc.id)}</option>
                  ))}
                </select>
              </div>
            </div>
            <button
                onClick={handleSync}
                disabled={loading || !selectedAccount}
                className="ios-btn-primary flex items-center gap-2 px-6 py-3 rounded-2xl font-bold shadow-lg shadow-blue-200 disabled:opacity-50"
            >
                <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
                同步商品
            </button>
            <button
              onClick={openAddModal}
              className="px-5 py-3 rounded-2xl font-bold bg-gray-900 text-white hover:bg-gray-800 transition-colors flex items-center gap-2 shadow-lg"
            >
              <Plus className="w-4 h-4" />
              添加商品
            </button>
            <button
              onClick={openPublishModal}
              className="px-5 py-3 rounded-2xl font-bold bg-emerald-500 text-white hover:bg-emerald-600 transition-colors flex items-center gap-2 shadow-lg shadow-emerald-100"
            >
              <PackagePlus className="w-4 h-4" />
              发布商品
            </button>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
          {visibleItems.map(/* 当前回调处理集合中的单个元素。 */ item => {
            // linkedRules 关联规则列表。
            const linkedRules = rulesForItem(item);
            // hasRule 是否存在规则。
            const hasRule = linkedRules.length > 0;
            return (
              <div key={`${item.cookie_id}-${item.item_id}`} className="ios-card p-3 rounded-2xl hover:shadow-lg transition-all group relative flex flex-col">
                  <div className="absolute top-2 right-2 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity z-10">
                      <button
                        onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => handleEdit(item)}
                        className="p-1.5 bg-white/90 backdrop-blur rounded-lg shadow-md text-gray-600 hover:bg-brand hover:text-white transition-colors"
                        title="编辑"
                      >
                        <Edit className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => handleDelete(item)}
                        className="p-1.5 bg-white/90 backdrop-blur rounded-lg shadow-md hover:bg-red-100 text-red-500 transition-colors"
                        title="删除"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                  </div>
                  <div className="aspect-square bg-gray-100 rounded-xl mb-2.5 overflow-hidden relative">
                      {item.item_image ? (
                          <img src={item.item_image} alt="" className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500" />
                      ) : (
                          <div className="w-full h-full flex items-center justify-center text-gray-300">
                              <Box className="w-8 h-8" />
                          </div>
                      )}
                      <div className="absolute top-1.5 left-1.5 bg-black/50 backdrop-blur-md text-white text-[10px] font-bold px-1.5 py-0.5 rounded-md">
                          {formatItemPrice(item.item_price)}
                      </div>
                  </div>
                  <h3 className="font-bold text-gray-900 line-clamp-2 text-xs mb-1.5 h-8 leading-4">{item.item_title}</h3>
                  <div className="mb-2 inline-flex min-w-0 max-w-full items-center gap-1 self-start rounded-md bg-blue-50 px-2 py-1 text-[10px] font-extrabold text-blue-700" title={accountNickname(item.cookie_id)}>
                    <User className="h-3 w-3 shrink-0" />
                    <span className="min-w-0 truncate whitespace-nowrap">{accountNickname(item.cookie_id)}</span>
                  </div>
                  <div className="flex justify-between items-center text-[10px] text-gray-500 mb-2">
                      <span className="bg-gray-100 px-1.5 py-0.5 rounded truncate max-w-[80px]">ID: {item.item_id}</span>
                      <span className={`inline-flex items-center gap-1 font-bold ${hasRule ? 'text-emerald-600' : 'text-amber-600'}`}>
                        {hasRule ? <CheckCircle2 className="w-3 h-3" /> : <CircleDashed className="w-3 h-3" />}
                        {hasRule ? `${linkedRules.length} 规则` : '未配置'}
                      </span>
                  </div>
                  <div className="space-y-2 mt-auto">
                      <button
                        onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => onConfigureDelivery(item)}
                        className={`w-full flex items-center justify-between gap-1 px-2.5 py-2 rounded-lg text-[11px] font-extrabold transition-all ${hasRule ? 'bg-gray-900 text-white hover:bg-black' : 'bg-brand text-white hover:bg-brand-highlight shadow-md shadow-blue-100'}`}
                      >
                        <span className="flex items-center gap-1.5"><Link2 className="w-3.5 h-3.5" />{hasRule ? '查看发货规则' : '关联发货规则'}</span>
                        <ArrowRight className="w-3.5 h-3.5" />
                      </button>
                  </div>
              </div>
            );
          })}
          {visibleItems.length === 0 && (
             <div className="col-span-full py-20 text-center text-gray-400">
                 <ShoppingBag className="w-12 h-12 mx-auto mb-4 opacity-30" />
                 {accountFilter ? '该账号暂无商品数据' : '暂无商品数据，请选择账号进行同步'}
             </div>
          )}
      </div>

      {showEditModal && selectedItem && createPortal(
        <div className="modal-overlay-centered">
          <div className="modal-container" style={{maxWidth: '560px'}}>
            <div className="modal-header flex items-center justify-between">
              <div>
                <h3 className="text-xl font-extrabold text-gray-900">编辑商品</h3>
                <p className="text-xs text-gray-500 mt-1">ID: {selectedItem.item_id}</p>
              </div>
              <button onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => setShowEditModal(false)} className="p-2 rounded-xl hover:bg-gray-100 transition-colors">
                <X className="w-5 h-5 text-gray-500" />
              </button>
            </div>
            <div className="modal-body space-y-4">
              <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="商品标题" value={editForm.item_title || ''} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setEditForm({...editForm, item_title: e.target.value})} />
              <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="价格" value={editForm.item_price || ''} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setEditForm({...editForm, item_price: e.target.value})} />
              <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="分类" value={editForm.item_category || ''} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setEditForm({...editForm, item_category: e.target.value})} />
              <textarea className="w-full ios-input px-4 py-3 rounded-xl h-28 resize-none" placeholder="描述" value={editForm.item_description || ''} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setEditForm({...editForm, item_description: e.target.value})} />
            </div>
            <div className="modal-footer">
              <button onClick={handleSaveEdit} className="w-full ios-btn-primary px-6 py-3 rounded-xl font-bold flex items-center justify-center gap-2">
                <Save className="w-4 h-4" />
                保存
              </button>
            </div>
          </div>
        </div>
      , document.body)}

      {showAddModal && createPortal(
        <div className="modal-overlay-centered">
          <div className="modal-container" style={{maxWidth: '720px'}}>
            <div className="modal-header flex items-center justify-between">
              <div>
                <h3 className="text-xl font-extrabold text-gray-900">添加商品</h3>
                <p className="text-xs text-gray-500 mt-1">手动建立商品与自动发货规则的关联</p>
              </div>
              <button onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => setShowAddModal(false)} className="p-2 rounded-xl hover:bg-gray-100 transition-colors" title="关闭">
                <X className="w-5 h-5 text-gray-500" />
              </button>
            </div>
            <div className="modal-body space-y-5">
              <div className="space-y-2">
                <label className="block text-sm font-bold text-gray-700">所属账号</label>
                <select className="w-full ios-input px-4 py-3 rounded-xl" value={addForm.cookie_id} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setAddForm({...addForm, cookie_id: e.target.value})}>
                  <option value="">选择账号</option>
                  {accounts.map(/* 当前回调处理集合中的单个元素。 */ acc => <option key={acc.id} value={acc.id}>{accountName(acc.id)}</option>)}
                </select>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <label className="block text-sm font-bold text-gray-700">商品 ID</label>
                  <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="输入闲鱼商品 ID" value={addForm.item_id} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setAddForm({...addForm, item_id: e.target.value})} />
                </div>
                <div className="space-y-2">
                  <label className="block text-sm font-bold text-gray-700">商品价格</label>
                  <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="例如 99.00" value={addForm.item_price} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setAddForm({...addForm, item_price: e.target.value})} />
                </div>
              </div>
              <div className="space-y-2">
                <label className="block text-sm font-bold text-gray-700">商品标题</label>
                <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="输入商品标题" value={addForm.item_title} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setAddForm({...addForm, item_title: e.target.value})} />
              </div>
              <div className="space-y-2">
                <label className="block text-sm font-bold text-gray-700">图片 URL</label>
                <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="https://..." value={addForm.item_image} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setAddForm({...addForm, item_image: e.target.value})} />
              </div>
            </div>
            <div className="modal-footer">
              <button onClick={handleAddItem} className="w-full ios-btn-primary px-6 py-3.5 rounded-xl font-bold flex items-center justify-center gap-2">
                <Plus className="w-4 h-4" />
                添加商品
              </button>
            </div>
          </div>
        </div>
      , document.body)}

      {showPublishModal && createPortal(
        <div className="modal-overlay-centered">
          <div className="modal-container" style={{maxWidth: '820px'}}>
            <div className="modal-header flex items-center justify-between">
              <div>
                <h3 className="text-xl font-extrabold text-gray-900">发布商品到闲鱼</h3>
                <p className="text-xs text-gray-500 mt-1">当前仅支持单规格电子资料商品。</p>
              </div>
              <button onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => setShowPublishModal(false)} className="p-2 rounded-xl hover:bg-gray-100 transition-colors" title="关闭">
                <X className="w-5 h-5 text-gray-500" />
              </button>
            </div>
            <div className="modal-body space-y-5">
              <div className="rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 leading-6">
                填写商品总库存。库存权限不足时会返回明确提示。
              </div>
              <div className="space-y-2">
                <label className="block text-sm font-bold text-gray-700">发布账号</label>
                <select className="w-full ios-input px-4 py-3 rounded-xl" value={publishForm.cookie_id} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => {
				  setPublishForm({...publishForm, cookie_id: e.target.value});
				  setPublishCategoryKeyword('');
				  setPublishCategory(null);
				  setPublishLocations([]);
				  setPublishLocation(null);
				}}>
                  <option value="">选择账号</option>
                  {accounts.map(/* 当前回调处理集合中的单个元素。 */ acc => <option key={acc.id} value={acc.id}>{accountName(acc.id)}</option>)}
                </select>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <label className="block text-sm font-bold text-gray-700">商品标题</label>
                  <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="例如：会员月卡自动发货" value={publishForm.title} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setPublishForm({...publishForm, title: e.target.value})} />
                </div>
                <div className="space-y-2">
                  <label className="block text-sm font-bold text-gray-700">库存数量</label>
                  <input className="w-full ios-input px-4 py-3 rounded-xl" type="number" min="1" placeholder="必须大于 0" value={publishForm.quantity} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setPublishForm({...publishForm, quantity: e.target.value})} />
                </div>
              </div>
              <div className="space-y-2">
                <label className="block text-sm font-bold text-gray-700">商品描述</label>
                <textarea className="w-full ios-input px-4 py-3 rounded-xl h-28 resize-none" placeholder="描述会用于自动识别类目；留空时使用标题" value={publishForm.description} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setPublishForm({...publishForm, description: e.target.value})} />
              </div>
              <div className="rounded-2xl border border-amber-200 bg-amber-50 p-4 space-y-3">
                <div>
                  <div className="text-sm font-extrabold text-gray-900">发布类目（可选）</div>
                  <p className="mt-1 text-xs leading-5 text-amber-800">填写关键词获取准确类目；留空时由闲鱼自动识别，识别不到时默认使用“电子资料”兜底。</p>
                </div>
                <div className="flex flex-col gap-2 sm:flex-row">
                  <input className="w-full ios-input rounded-xl bg-white px-4 py-3" placeholder="例如：课程资料、电子书" value={publishCategoryKeyword} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => { setPublishCategoryKeyword(e.target.value); setPublishCategory(null); }} onKeyDown={/* 当前回调处理用户交互或异步状态变化。 */ e => { if (e.key === 'Enter') { e.preventDefault(); void handleRecommendPublishCategory(); } }} />
                  <button type="button" disabled={publishCategoryLoading || !publishForm.cookie_id || !publishCategoryKeyword.trim()} onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => void handleRecommendPublishCategory()} className="shrink-0 rounded-xl bg-amber-500 px-4 py-3 text-sm font-bold text-white hover:bg-amber-600 disabled:opacity-50">
                    {publishCategoryLoading ? '匹配中...' : '获取类目'}
                  </button>
                </div>
                {publishCategory ? (
                  <div className="flex items-center justify-between gap-3 rounded-xl bg-white px-3 py-2 text-sm">
                    <div><span className="font-bold text-gray-900">{publishCategory.cat_name}</span><span className="ml-2 font-mono text-xs text-gray-500">{publishCategory.cat_id} · 频道 {publishCategory.channel_cat_id}</span></div>
                    <button type="button" className="text-xs font-bold text-gray-500 hover:text-red-600" onClick={/* 当前回调处理用户交互或异步状态变化。 */ () => setPublishCategory(null)}>清除，使用自动识别</button>
                  </div>
                ) : <div className="text-xs text-gray-500">当前未指定类目，最终识别失败时会使用电子资料兜底。</div>}
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div className="space-y-2">
                  <label className="block text-sm font-bold text-gray-700">售价</label>
                  <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="99.00" value={publishForm.price} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setPublishForm({...publishForm, price: e.target.value})} />
                </div>
                <div className="space-y-2">
                  <label className="block text-sm font-bold text-gray-700">原价（可选）</label>
                  <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="129.00" value={publishForm.original_price} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setPublishForm({...publishForm, original_price: e.target.value})} />
                </div>
                <div className="space-y-2">
                  <label className="block text-sm font-bold text-gray-700">运费方式</label>
                  <select className="w-full ios-input px-4 py-3 rounded-xl" value={publishForm.postage_mode} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setPublishForm({...publishForm, postage_mode: e.target.value})}>
                    <option value="free">包邮</option>
                    <option value="distance">按距离计费</option>
                    <option value="fixed">一口价邮费</option>
                    <option value="none">无需邮寄</option>
                  </select>
                </div>
              </div>
              {publishForm.postage_mode === 'fixed' && (
                <div className="space-y-2">
                  <label className="block text-sm font-bold text-gray-700">一口价邮费</label>
                  <input className="w-full ios-input px-4 py-3 rounded-xl" placeholder="例如 8.00" value={publishForm.postage} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setPublishForm({...publishForm, postage: e.target.value})} />
                </div>
              )}
			  <div className="rounded-2xl border border-sky-200 bg-sky-50 p-4 space-y-3">
				<div className="flex items-center justify-between gap-3">
				  <div><div className="text-sm font-extrabold text-gray-900">发货地（可选）</div><p className="mt-1 text-xs text-sky-800">虚拟商品无需发货地；发布失败时可再定位并作为补充信息提交。</p></div>
					<div className="flex flex-wrap justify-end gap-2">
              <button type="button" disabled={locationLoading || !publishForm.cookie_id} onClick={/* currentLocationClickAction 获取设备当前位置附近的高德发货地。 */ () => void locateForPublish()} className="ios-btn-primary flex items-center gap-2 rounded-xl px-4 py-2 text-sm font-bold disabled:opacity-50">
					    <LocateFixed className="h-4 w-4" />{locationLoading ? '定位中...' : '获取当前位置'}
					  </button>
					  <button type="button" disabled={locationLoading || !publishForm.cookie_id} onClick={/* manualLocationClickAction 打开普通发布的高德手动选点弹窗。 */ () => setManualLocationTarget('publish')} className="flex items-center gap-2 rounded-xl border border-sky-200 bg-white px-4 py-2 text-sm font-bold text-sky-700 transition-colors hover:bg-sky-50 disabled:opacity-50">
					    <Search className="h-4 w-4" />手动选择
					  </button>
					</div>
				</div>
				{publishLocations.length > 0 && <select className="w-full ios-input rounded-xl bg-white px-4 py-3" value={String(Math.max(0, publishLocations.indexOf(publishLocation!)))} onChange={/* 当前回调处理用户交互或异步状态变化。 */ e => setPublishLocation(publishLocations[Number(e.target.value)] || null)}>
				  {publishLocations.map(/* 当前回调处理集合中的单个元素。 */ (item, index) => <option key={`${item.division_id}-${item.poi_id}-${index}`} value={String(index)}>{[item.province, item.city, item.area, item.poi_name].filter(Boolean).join(' ')}</option>)}
				</select>}
			  </div>
              {ImagesEditor && <ImagesEditor images={publishForm.images} previews={publishImagePreviews} onChange={/* publishImagesChangeAction 将图片追加、删除或排序后的列表写回发布草稿。 */ (images) => setPublishForm(/* previous 保存上一次发布草稿状态。 */ previous => ({ ...previous, images }))} />}
            </div>
            <div className="modal-footer">
              <button disabled={publishing} onClick={handlePublishItem} className="w-full bg-emerald-500 hover:bg-emerald-600 disabled:opacity-60 text-white px-6 py-3.5 rounded-xl font-bold flex items-center justify-center gap-2">
                <PackagePlus className="w-4 h-4" />
                {publishing ? '正在发布...' : '发布到闲鱼'}
              </button>
            </div>
          </div>
        </div>
      , document.body)}

      <ManualLocationPicker
        open={manualLocationTarget !== null}
        onClose={/* manualLocationCloseAction 关闭手动地点弹窗并释放高德查询。 */ () => setManualLocationTarget(null)}
        onConfirm={confirmManualLocation}
      />
    </div>
  );
};

export default ItemList;
