package adapter

import (
	"context"
	"errors"
	"testing"

	"xianyu-go/internal/automation"
)

// itemRelisterStub 记录原商品重新上架调用，用于验证商品 ID 不会被替换。
type itemRelisterStub struct {
	accountID string
	itemID    string
	err       error
}

// RelistItem 记录调用参数并返回预置的外部结果。
func (s *itemRelisterStub) RelistItem(_ context.Context, accountID, itemID string) error {
	s.accountID, s.itemID = accountID, itemID
	return s.err
}

// TestRepublishUsesOriginalItemID 验证自动上架调用的是原商品，而不是复制发布新商品。
func TestRepublishUsesOriginalItemID(t *testing.T) {
	// relister 记录原商品重新上架请求。
	relister := &itemRelisterStub{}
	// republisher 保存只依赖原商品重新上架端口的自动化适配器。
	republisher := NewItemRepublisher(relister)
	// err 保存原商品重新上架结果。
	err := republisher.RepublishItem(context.Background(), automation.RepublishItemRequest{
		AccountID: "account-1", ItemID: "item-original", OrderID: "order-1",
	})
	if err != nil {
		t.Fatalf("RepublishItem: %v", err)
	}
	if relister.accountID != "account-1" || relister.itemID != "item-original" {
		t.Fatalf("重新上架参数=%+v", relister)
	}
}

// TestRepublishReturnsRelistError 验证平台重新上架失败会保留错误链交给自动化补偿流程。
func TestRepublishReturnsRelistError(t *testing.T) {
	// expected 保存平台重新上架失败原因。
	expected := errors.New("平台拒绝重新上架")
	// relister 记录带失败结果的重新上架请求。
	republisher := NewItemRepublisher(&itemRelisterStub{err: expected})
	// err 保存适配器包装后的失败结果。
	err := republisher.RepublishItem(context.Background(), automation.RepublishItemRequest{
		AccountID: "account-1", ItemID: "item-original", OrderID: "order-1",
	})
	if !errors.Is(err, expected) {
		t.Fatalf("错误链未保留: %v", err)
	}
}

// TestRepublishRequiresOrderID 验证缺少触发订单时不会调用原商品重新上架。
func TestRepublishRequiresOrderID(t *testing.T) {
	// relister 记录不应发生的重新上架调用。
	relister := &itemRelisterStub{}
	// err 保存缺少审计订单号时的校验结果。
	err := NewItemRepublisher(relister).RepublishItem(context.Background(), automation.RepublishItemRequest{
		AccountID: "account-1", ItemID: "item-original",
	})
	if err == nil || relister.itemID != "" {
		t.Fatalf("缺少订单号仍调用了重新上架: err=%v relister=%+v", err, relister)
	}
}
