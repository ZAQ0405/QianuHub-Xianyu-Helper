package engine

import (
	"context"
	"strings"
	"time"

	"xianyu-go/internal/db"
	"xianyu-go/internal/xianyu/protocol"
)

// replyItemPublisher 是自动回复消费者需要的最小商品身份查询能力。
type replyItemPublisher interface {
	// FetchItemPublisher 用 ctx 取消请求；cookies 仅限认证，itemID 为当前会话商品，返回发布人 ID 或错误。
	FetchItemPublisher(ctx context.Context, cookies, itemID string) (string, error)
}

// replySessionRoleStore 是消息处理器读取和保存会话级买卖角色所需的最小本地能力。
type replySessionRoleStore interface {
	// ChatSessionRole 读取账号、会话和商品共同绑定的角色结论。
	ChatSessionRole(ctx context.Context, accountID, chatID, itemID string) (db.ChatSession, error)
	// SaveChatSessionRole 保存首次平台核验结果，商品发生变化时不得覆盖新会话状态。
	SaveChatSessionRole(ctx context.Context, accountID, chatID, itemID, accountRole, buyerUserID, sellerUserID, roleSource string) error
}

// canAutoReply 核验 message 的会话商品是否由当前账号发布；ctx 来自账号生命周期。
// 查询不持锁，最长十秒；失败、缺少身份或请求期间账号切换均禁止自动回复，聊天消息仍正常观察和保存。
func (d *messageDispatcher) canAutoReply(ctx context.Context, message ChatMessage) bool {
	if strings.TrimSpace(message.ItemID) == "" {
		return false
	}
	// handler 是已经完成消息落库的业务处理器；缺少角色仓储时采用拒绝回复的保守语义。
	handler := d.currentHandler()
	// roleStore、supported 保存会话角色仓储及能力标识。
	roleStore, supported := handler.(replySessionRoleStore)
	if !supported {
		return false
	}
	// cookies 是当前凭证快照；凭证明文只用于必要的平台核验。
	cookies := d.currentCookie()
	// selfID 是凭证快照中的非敏感平台用户标识。
	selfID := protocol.TransCookies(cookies)["unb"]
	if strings.TrimSpace(selfID) == "" {
		return false
	}
	// session、roleErr 是与当前商品绑定的本地角色结论及查询错误。
	session, roleErr := roleStore.ChatSessionRole(ctx, message.AccountID, message.ChatID, message.ItemID)
	if roleErr != nil {
		d.logger.Warn("读取聊天会话角色失败，跳过自动回复", "chat_id", message.ChatID)
		return false
	}
	if session.AccountRole == "seller" {
		return isSelfUserID(session.SellerUserID, selfID)
	}
	if session.AccountRole == "buyer" || session.RoleSource == "platform_unresolved" || d.itemPublisher == nil {
		return false
	}
	// claimErr 在外部查询前固定当前商品已经开始核验；写入失败时不访问平台，避免错误路径反复请求。
	claimErr := roleStore.SaveChatSessionRole(ctx, message.AccountID, message.ChatID, message.ItemID, "unknown", "", "", "platform_unresolved")
	if claimErr != nil {
		d.logger.Warn("保存聊天会话身份核验标记失败，跳过自动回复", "chat_id", message.ChatID)
		return false
	}
	// requestCtx、cancel 将平台身份核验限制在账号生命周期和十秒预算内。
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// publisher、err 保存会话商品的发布人和查询结果；错误详情可能包含平台请求信息，不输出。
	publisher, err := d.itemPublisher.FetchItemPublisher(requestCtx, cookies, message.ItemID)
	if err != nil || requestCtx.Err() != nil {
		d.logger.Warn("商品发布人身份核验失败，跳过自动回复")
		return false
	}
	if strings.TrimSpace(publisher) == "" {
		d.logger.Warn("商品发布人身份为空，跳过自动回复")
		return false
	}
	// accountRole、buyerUserID、sellerUserID 是首次平台核验后要持久化的明确参与方结论。
	accountRole, buyerUserID, sellerUserID := "buyer", selfID, publisher
	if isSelfUserID(publisher, selfID) {
		accountRole, buyerUserID, sellerUserID = "seller", message.SenderUserID, selfID
	}
	// saveErr 保存角色持久化结果；失败时不回复，避免下一步使用未能固定的身份结论。
	saveErr := roleStore.SaveChatSessionRole(ctx, message.AccountID, message.ChatID, message.ItemID, accountRole, buyerUserID, sellerUserID, "platform_verified")
	if saveErr != nil {
		d.logger.Warn("保存聊天会话角色失败，跳过自动回复", "chat_id", message.ChatID)
		return false
	}
	return accountRole == "seller" && isSelfUserID(selfID, protocol.TransCookies(d.currentCookie())["unb"])
}
