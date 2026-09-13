import type { ComponentType } from 'react';
import type { AccountDetail,Item,ShippingRule } from './api';
import type { PublishImagesEditorProps } from './components/PublishImagesEditor';

// ItemListProps 描述商品页面从父级接收的规则配置回调。
export interface ItemListProps {
  // onConfigureDelivery 打开指定商品的自动化发货规则编辑器。
  onConfigureDelivery: (item: Item) => void;
  // publishImagesEditor 是应用壳按需注入的可排序商品图片编辑器组件。
  publishImagesEditor?: ComponentType<PublishImagesEditorProps>;
}

// PublishCategory 描述普通单商品发布使用的闲鱼类目。
export interface PublishCategory {
  // cat_id 是闲鱼类目标识。
  cat_id: string;
  // cat_name 是类目展示名称。
  cat_name: string;
  // channel_cat_id 是闲鱼频道类目标识。
  channel_cat_id?: string;
  // tb_cat_id 是可选的淘宝类目标识。
  tb_cat_id?: string;
}

// ItemReferenceState 描述商品页面 Hook 需要的共享列表状态。
export interface ItemReferenceState {
  // items 是商品列表。
  items: Item[];
  // shippingRules 是商品关联的自动化规则。
  shippingRules: ShippingRule[];
  // accounts 是账号列表。
  accounts: AccountDetail[];
}
