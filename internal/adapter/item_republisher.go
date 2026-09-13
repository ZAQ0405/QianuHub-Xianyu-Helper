package adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xianyu-go/internal/automation"
)

// ItemRepublisher 负责付款后调用原商品同 ID 的重新上架能力。
type ItemRepublisher struct {
	// relister 提供原商品重新上架能力，不包含复制发布或新商品落库逻辑。
	relister interface {
		RelistItem(context.Context, string, string) error
	}
}

// NewItemRepublisher 构造原商品重新上架适配器。
func NewItemRepublisher(relister interface {
	RelistItem(context.Context, string, string) error
}) *ItemRepublisher {
	return &ItemRepublisher{relister: relister}
}

// RepublishItem 重新上架原商品，不创建新商品、不修改商品 ID，也不迁移自动化规则。
func (r *ItemRepublisher) RepublishItem(ctx context.Context, request automation.RepublishItemRequest) error {
	if r == nil || r.relister == nil {
		return errors.New("原商品重新上架能力未初始化")
	}
	// accountID、itemID、orderID 分别表示执行账号、原商品和触发订单的非敏感平台标识。
	accountID := strings.TrimSpace(request.AccountID)
	// itemID 保存需要恢复上架的原商品标识。
	itemID := strings.TrimSpace(request.ItemID)
	// orderID 保存触发本次动作的已付款订单标识。
	orderID := strings.TrimSpace(request.OrderID)
	if accountID == "" || itemID == "" || orderID == "" {
		return errors.New("原商品重新上架缺少账号、商品或订单ID")
	}
	// err 保存原商品重新上架调用的失败结果。
	if err := r.relister.RelistItem(ctx, accountID, itemID); err != nil {
		return fmt.Errorf("原商品 %s 重新上架失败: %w", itemID, err)
	}
	return nil
}

// 编译期确保重新上架适配器实现自动化中心定义的最小能力。
var _ automation.ItemRepublisher = (*ItemRepublisher)(nil)
