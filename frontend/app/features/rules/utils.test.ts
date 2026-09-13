import { describe,expect,test } from 'vitest';
import type { AccountDetail,ShippingRule } from './api';
import { accentClasses,accountLabel,actionSummary,boolFlag,buildConfirmShipmentMessage,buildReviewConfig,cardActionsForTrigger,confirmShipmentMessage,defaultRuleName,emptyVariant,needsAllItemsConfirmation,parseJSONObject,shouldReplaceGeneratedName,statusPill,withAllItemsConfirmation } from './utils';

// rule 是规则工具测试使用的最小规则对象。
const rule = (overrides: Partial<ShippingRule> = {}): ShippingRule => ({
  id: 'rule-1', name: '测试规则', trigger_type: 'order_paid', item_keyword: '', card_group_id: 0,
  priority: 1, enabled: true, actions: [], variants: [], ...overrides,
});

describe('规则工具函数', /* 当前回调处理在用自动发货与评价提醒规则。 */ () => {
  test('安全解析配置并规范化求评价参数', /* 当前回调验证规则配置的容错边界。 */ () => {
    expect(parseJSONObject()).toEqual({});
    expect(parseJSONObject('{bad')).toEqual({});
    expect(parseJSONObject('{"max_attempts":3}')).toEqual({ max_attempts: 3 });
    expect(buildReviewConfig('{"after_shipped_hours":48}')).toBe('{"after_shipped_hours":48,"repeat_interval_hours":24,"max_attempts":1}');
  });

  test('生成在用规则名称和默认动作链', /* 当前回调验证付款发货与评价提醒规则的默认内容。 */ () => {
    expect(defaultRuleName('order_paid')).toBe('付款后自动发货');
    expect(defaultRuleName('review_missing_timeout', '数字商品')).toBe('超时未评价求评价 - 数字商品');
    expect(cardActionsForTrigger('review_missing_timeout')).toEqual([{ action_type: 'send_text', message_template: '亲，商品使用满意的话，麻烦给个评价，谢谢～', enabled: true, sort_order: 1 }]);
    expect(cardActionsForTrigger('order_paid', 7)).toEqual([
      { action_type: 'send_card', card_id: 7, delivery_count: 1, enabled: true, sort_order: 1 },
      { action_type: 'confirm_shipment', enabled: true, sort_order: 2 },
    ]);
  });

  test('读取确认发货文案和规则摘要', /* 当前回调验证自动发货规则的可见摘要。 */ () => {
    const action = { action_type: 'confirm_shipment' as const, enabled: true, sort_order: 2 }; // action 是确认发货动作测试数据。
    expect(confirmShipmentMessage([action])).toBe('');
    const updated = buildConfirmShipmentMessage(action, '资料已发送，请查收'); // updated 是写入文案后的确认发货动作。
    expect(confirmShipmentMessage([updated])).toBe('资料已发送，请查收');
    expect(actionSummary(rule({ actions: [{ action_type: 'send_card', card_name: '资料库存', enabled: true }, { action_type: 'republish_item', enabled: true }] }))).toContain('付款后自动确认发货并重新上架');
  });

  test('保留通用授权配置并提供展示辅助函数', /* 当前回调验证规则列表和主题展示辅助逻辑。 */ () => {
    expect(JSON.parse(withAllItemsConfirmation('{"after_shipped_hours":24}', true))).toEqual({ after_shipped_hours: 24, allow_all_items: true });
    expect(needsAllItemsConfirmation(rule())).toBe(true);
    expect(needsAllItemsConfirmation(rule({ config_json: '{"allow_all_items":true}' }))).toBe(false);
    expect(needsAllItemsConfirmation(rule({ item_id: 'item-a' }))).toBe(false);
    expect(emptyVariant()).toEqual({ card_id: 0, delivery_count: 1, enabled: true, delay_override: false, delay_seconds: 0 });
    expect(accountLabel({ id: 'a', nickname: '昵称' } as AccountDetail)).toBe('昵称');
    expect(accountLabel()).toBe('未知账号');
    expect(shouldReplaceGeneratedName('付款后自动发货')).toBe(true);
    expect(shouldReplaceGeneratedName('我自定义的规则')).toBe(false);
    expect(accentClasses('blue', true)).toContain('border-blue-500');
    expect(statusPill(true)).toContain('bg-emerald-100');
    expect(boolFlag('1')).toBe(true);
  });
});
