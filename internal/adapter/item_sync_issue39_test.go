package adapter

import (
	"context"
	"errors"
	"testing"

	itemapp "xianyu-go/internal/application/items"
	"xianyu-go/internal/db"
	"xianyu-go/internal/xianyu/mtop"
)

// TestIncompleteItemSyncPreservesItemsAndRules 验证分页层报告错误时，部分列表不能删除现有商品或规则；t 管理本地 SQLite 生命周期。
func TestIncompleteItemSyncPreservesItemsAndRules(t *testing.T) {
	// store、cleanup 是隔离数据库及关闭函数，不读取用户业务数据库。
	store, cleanup := newAdapterTestStore(t)
	defer cleanup()
	// ctx 是仓储夹具与同步用例的共同上下文。
	ctx := context.Background()
	// owner、ownerErr 保存合成账号的所有者。
	owner, ownerErr := store.Users.GetByUsername(ctx, "admin")
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if itemErr := store.Items.Upsert(ctx, &db.ItemInfoRow{CookieID: "cid", ItemID: "keep", ItemTitle: "必须保留"}); itemErr != nil { // itemErr 是既有商品夹具写入结果。
		t.Fatal(itemErr)
	}
	// ruleID、ruleErr 创建一个在同步失败时必须保持启用的商品规则。
	ruleID, ruleErr := store.Automation.Create(ctx, db.AutomationRuleInput{UserID: owner.ID, CookieID: "cid", ItemID: "keep", Name: "保留规则", TriggerType: "order_paid", Enabled: true})
	if ruleErr != nil {
		t.Fatal(ruleErr)
	}
	// client 故意同时返回部分数据和失败，验证错误优先于部分列表。
	client := &itemSyncListClient{allResult: &mtop.ItemListResult{Items: []mtop.ItemListItem{{ID: "partial"}}}, allErr: errors.New("分页不完整")}
	// repository 使用真实事务仓储和本地平台替身。
	repository := NewItemSyncRepository(store, func() mtop.Client { return client }, nil, nil, nil)
	if _, syncErr := repository.SyncAll(ctx, itemapp.SyncQuery{UserID: owner.ID, CookieID: "cid"}); syncErr == nil { // syncErr 必须向应用和页面传播失败。
		t.Fatal("不完整同步未向用户返回失败")
	}
	// item、itemErr 验证旧商品仍可被正常管理查询读取。
	item, itemErr := store.Items.Get(ctx, "cid", "keep")
	if itemErr != nil || item.ItemTitle != "必须保留" {
		t.Fatal("同步失败错误删除或改写了既有商品")
	}
	// rule、loadErr 验证关联发货规则没有被停用或软删除。
	rule, loadErr := store.Automation.Get(ctx, ruleID)
	if loadErr != nil || !rule.Enabled {
		t.Fatal("同步失败错误停用了关联规则")
	}
	if _, partialErr := store.Items.Get(ctx, "cid", "partial"); !errors.Is(partialErr, db.ErrNotFound) { // partialErr 验证部分列表没有被提前写入。
		t.Fatal("同步失败仍写入了部分商品")
	}
}
