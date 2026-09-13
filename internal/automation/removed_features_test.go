package automation

import (
	"context"
	"testing"

	"xianyu-go/internal/db"
)

// assertRemovedAutomationTrigger 验证个人版不会为已移除的事件创建自动化运行。
func assertRemovedAutomationTrigger(t *testing.T, trigger string) {
	t.Helper()
	store, cleanup := newAutomationTestStore(t)
	defer cleanup()
	center := New(store, nil, nil)
	if err := center.HandleTask(context.Background(), Task{AccountID: "cid", TriggerType: trigger, OrderID: "removed-trigger-order"}); err != nil {
		t.Fatalf("已移除触发器不应执行: trigger=%s err=%v", trigger, err)
	}
	var runs int
	if err := store.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM automation_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("已移除触发器不应创建运行记录: trigger=%s runs=%d", trigger, runs)
	}
}

// assertRemovedAutomationAction 验证已移除动作不会进入付款事件的执行计划。
func assertRemovedAutomationAction(t *testing.T, action string) {
	t.Helper()
	store, cleanup := newAutomationTestStore(t)
	defer cleanup()
	ctx := context.Background()
	admin, err := store.Users.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Automation.Create(ctx, db.AutomationRuleInput{
		UserID: admin.ID, CookieID: "cid", ItemID: "removed-item", Name: "removed-action", TriggerType: TriggerReviewMissingTimeout, Enabled: true,
		Actions: []db.AutomationActionInput{{ActionType: action, Enabled: true}},
	}); err != nil {
		t.Fatal(err)
	}
	center := New(store, nil, nil)
	if err := center.HandleTask(ctx, Task{AccountID: "cid", ItemID: "removed-item", TriggerType: TriggerReviewMissingTimeout, OrderID: "removed-action-order"}); err != nil {
		t.Fatalf("已移除动作不应执行: action=%s err=%v", action, err)
	}
	var runs int
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM automation_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("已移除动作应只创建一次空运行记录并立即收口: action=%s runs=%d", action, runs)
	}
	var status string
	if err := store.DB.QueryRowContext(ctx, `SELECT status FROM automation_runs LIMIT 1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "success" {
		t.Fatalf("已移除动作的空运行应成功收口: action=%s status=%s", action, status)
	}
}
