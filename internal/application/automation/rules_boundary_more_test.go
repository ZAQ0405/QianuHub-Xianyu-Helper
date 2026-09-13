package automation

import (
	"context"
	"strings"
	"testing"
)

// TestRuleServiceCoversOwnershipAndRemovedFeatureBranches 验证规则规范化的账号归属、商品归属和已删除触发器分支。
func TestRuleServiceCoversOwnershipAndRemovedFeatureBranches(t *testing.T) {
	// accountService 保存账号归属被拒绝的规则服务。
	accountService := NewRuleService(&ruleRepositoryFake{}, &ruleOwnershipFake{accountOwnedSet: true, accountOwned: false})
	// accountErr 保存账号不归属时的规则错误。
	_, accountErr := accountService.Normalize(context.Background(), 1, RuleDraft{CookieID: "account", TriggerType: TriggerBuyerReviewed, Actions: []ActionDraft{{ActionType: ActionSendText, MessageTemplate: "text"}}})
	if accountErr == nil {
		t.Fatal("不属于当前用户的账号不应通过规则规范化")
	}
	// itemService 保存商品归属被拒绝的规则服务。
	itemService := NewRuleService(&ruleRepositoryFake{}, &ruleOwnershipFake{itemOwnedSet: true, itemOwned: false})
	// itemErr 保存商品不归属时的规则错误。
	_, itemErr := itemService.Normalize(context.Background(), 1, RuleDraft{CookieID: "account", ItemID: "item", TriggerType: TriggerBuyerReviewed, Actions: []ActionDraft{{ActionType: ActionSendText, MessageTemplate: "text"}}})
	if itemErr == nil {
		t.Fatal("不属于当前用户的商品不应通过规则规范化")
	}
	// removedResultErr 保存已删除拍下未付款改价规则的拒绝结果。
	_, removedResultErr := itemService.Normalize(context.Background(), 1, RuleDraft{CookieID: "account", TriggerType: TriggerOrderCreated, Enabled: true, Actions: []ActionDraft{{ActionType: ActionAdjustPrice, ConfigJSON: `{"target_price":"1.00"}`}}})
	if removedResultErr == nil || !strings.Contains(removedResultErr.Error(), "不支持的触发类型") {
		t.Fatalf("已删除触发器应被拒绝，错误=%v", removedResultErr)
	}
}

// TestRulePureHelpersCoverRemainingValidationBranches 验证求评价文本缺失和小数格式错误的纯校验分支。
func TestRulePureHelpersCoverRemainingValidationBranches(t *testing.T) {
	// missingTextErr 保存求评价规则缺少启用文本动作的错误。
	missingTextErr := validateTriggerActionCombination(TriggerReviewMissingTimeout, ruleActionFlags{})
	if missingTextErr == nil {
		t.Fatal("缺少求评价文本动作不应通过")
	}
	// invalidFractionErr 保存目标价格小数部分不是数字时的错误。
	invalidFractionErr := validateAdjustPriceConfig(`{"target_price":"1.a"}`)
	if invalidFractionErr == nil {
		t.Fatal("非法金额小数不应通过")
	}
}
