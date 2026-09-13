package automation

import (
	"context"
	"strings"
	"testing"
)

// TestRuleServiceAcceptsItemRepublishAction 验证重新上架动作只允许出现在具体商品的付款规则中。
func TestRuleServiceAcceptsItemRepublishAction(t *testing.T) {
	// service 使用现有规则测试归属替身，确保本用例只验证动作契约。
	service := NewRuleService(&ruleRepositoryFake{}, &ruleOwnershipFake{})
	// input、err 保存商品级付款规则的规范化结果。
	input, err := service.Normalize(context.Background(), 42, RuleDraft{
		CookieID: "account-1", ItemID: "item-1", TriggerType: TriggerOrderPaid,
		Actions: []ActionDraft{{ActionType: ActionSendCard, CardID: 1}, {ActionType: ActionRepublishItem}},
	})
	if err != nil || len(input.Actions) != 3 || input.Actions[2].ActionType != ActionConfirmShipment {
		t.Fatalf("合法自动上架规则被拒绝: input=%+v err=%v", input, err)
	}
}

// TestRuleServiceRejectsAccountRepublishAction 验证账号通用付款规则不能启用自动上架。
func TestRuleServiceRejectsAccountRepublishAction(t *testing.T) {
	// service 使用规则应用服务的隔离归属替身。
	service := NewRuleService(&ruleRepositoryFake{}, &ruleOwnershipFake{})
	// err 保存缺少商品绑定时的校验错误。
	_, err := service.Normalize(context.Background(), 42, RuleDraft{
		CookieID: "account-1", TriggerType: TriggerOrderPaid,
		Actions: []ActionDraft{{ActionType: ActionSendCard, CardID: 1}, {ActionType: ActionRepublishItem}},
	})
	if err == nil || !strings.Contains(err.Error(), "必须绑定具体商品") {
		t.Fatalf("账号通用自动上架规则未被拒绝: %v", err)
	}
}
