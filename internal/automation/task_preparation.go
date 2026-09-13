package automation

import (
	"context"
	"errors"
	"fmt"

	"xianyu-go/internal/db"
)

// resolvePaidTaskOrder 为只有会话标识的简化付款消息回填本账号最近待发货订单，避免缺少订单号时无法匹配商品规则。
func (c *Center) resolvePaidTaskOrder(ctx context.Context, task Task) (Task, error) {
	if c == nil || c.store == nil || c.store.Orders == nil || task.TriggerType != TriggerOrderPaid || task.OrderID != "" || task.ChatID == "" {
		return task, nil
	}
	// order 保存按账号、会话以及可选买家和商品条件命中的待发货订单。
	order, err := c.store.Orders.FindLatestPendingByChat(ctx, task.AccountID, task.ChatID, task.BuyerID, task.ItemID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return task, nil
		}
		return task, fmt.Errorf("按会话回填自动发货订单: %w", err)
	}
	if order == nil {
		return task, nil
	}
	return mergeOrderIntoTask(task, order), nil
}

// prepareBuyerNickname 补齐模板渲染所需的买家昵称摘要。
func (c *Center) prepareBuyerNickname(ctx context.Context, task Task) (Task, error) {
	if task.BuyerNickname != "" || task.ChatID == "" {
		return task, nil
	}
	// nickname 保存聊天会话中可用于模板渲染的买家昵称。
	nickname, nicknameErr := c.store.Chats.BuyerNicknameForAutomation(ctx, task.AccountID, task.ChatID)
	if nicknameErr != nil {
		return task, fmt.Errorf("读取买家昵称: %w", nicknameErr)
	}
	task.BuyerNickname = nickname
	return task, nil
}

// mergeOrderIntoTask 用本地订单事实补全自动化任务中尚未获得的字段。
func mergeOrderIntoTask(task Task, order *db.Order) Task {
	if task.OrderID == "" {
		task.OrderID = order.OrderID
	}
	if task.ItemID == "" {
		task.ItemID = order.ItemID
	}
	if task.BuyerID == "" {
		task.BuyerID = order.BuyerID
	}
	if task.ChatID == "" {
		task.ChatID = order.ChatID
	}
	if task.SpecName == "" {
		task.SpecName = order.SpecName
	}
	if task.SpecValue == "" {
		task.SpecValue = order.SpecValue
	}
	if task.Quantity == "" {
		task.Quantity = order.Quantity
	}
	if task.Amount == "" {
		task.Amount = order.Amount
	}
	if task.OrderStatus == "" {
		task.OrderStatus = order.OrderStatus
	}
	return task
}
