package automation

import (
	"context"
	"testing"

	"xianyu-go/internal/db"
)

// republisherFake 记录自动上架动作收到的订单上下文。
type republisherFake struct {
	// request 保存最近一次自动上架请求。
	request RepublishItemRequest
}

// RepublishItem 保存测试动作传入的自动上架请求。
func (f *republisherFake) RepublishItem(_ context.Context, request RepublishItemRequest) error {
	f.request = request
	return nil
}

// TestActionExecutorDispatchesRepublishItem 验证执行器会把新动作委托给商品重新发布端口。
func TestActionExecutorDispatchesRepublishItem(t *testing.T) {
	// fake 保存自动上架动作的调用记录。
	fake := &republisherFake{}
	// executor 保存只装配商品重新发布能力的动作执行器。
	executor := automationActionExecutor{republisher: func() ItemRepublisher { return fake }}
	// result、err 保存动作执行结果。
	result, err := executor.executeActionWithProof(context.Background(), Task{AccountID: "account-1", ItemID: "item-1", OrderID: "order-1"}, db.AutomationAction{ActionType: ActionRepublishItem, Enabled: true}, shipmentDeliveryProof{})
	if err != nil || result.sent != 1 {
		t.Fatalf("自动上架动作执行异常: result=%+v err=%v", result, err)
	}
	if fake.request.AccountID != "account-1" || fake.request.ItemID != "item-1" || fake.request.OrderID != "order-1" {
		t.Fatalf("自动上架请求上下文异常: %+v", fake.request)
	}
}
