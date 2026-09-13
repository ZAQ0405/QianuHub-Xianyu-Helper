package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"xianyu-go/internal/automation"
)

// outgoingObserverHandler 用于本次流程后续判断的outgoingObserverHandler
type outgoingObserverHandler struct {
	messages []OutgoingChatMessage
}

// HandleChatMessage 处理聊天消息。
func (h *outgoingObserverHandler) HandleChatMessage(context.Context, ChatMessage) error { return nil }

// HandleSystemEvent 处理系统Event。
func (h *outgoingObserverHandler) HandleSystemEvent(context.Context, automation.Task) error {
	return nil
}

// OnPasswordLoginRefresh 封装On密码登录Refresh业务协调。
func (h *outgoingObserverHandler) OnPasswordLoginRefresh(context.Context, string) bool { return false }

// OnAccountAlert 封装On账号Alert业务协调。
func (h *outgoingObserverHandler) OnAccountAlert(context.Context, string, string, string, string) {}

// HandleOutgoingChatMessage 处理Outgoing聊天消息。
func (h *outgoingObserverHandler) HandleOutgoingChatMessage(_ context.Context, message OutgoingChatMessage) error {
	h.messages = append(h.messages, message)
	return nil
}

// TestSendTextEmitsCorrelatedOutgoingObservation 封装TestSend文本EmitsCorrelatedOutgoingObservation业务协调。
func TestSendTextEmitsCorrelatedOutgoingObservation(t *testing.T) {
	// handler 用于本次流程后续判断的handler
	handler := &outgoingObserverHandler{}
	// account 用于本次流程后续判断的账号
	account := New(Config{CookieID: "account-1", CookieStr: "unb=me", Handler: handler})
	// conn 用于本次流程后续判断的conn
	conn := &fakeWSConn{}
	account.mu.Lock()
	account.conn = conn
	account.mu.Unlock()
	// ctx 用于本次流程后续判断的ctx
	ctx := WithOutgoingMessageKey(context.Background(), "local-1")
	if // err 用于本次流程后续判断的err
	err := account.SendText(ctx, "chat-1", "buyer-1", "您好"); err != nil {
		t.Fatal(err)
	}
	if len(handler.messages) != 1 {
		t.Fatalf("messages=%+v", handler.messages)
	}
	// got 用于本次流程后续判断的got
	got := handler.messages[0]
	if got.AccountID != "account-1" || got.ChatID != "chat-1" || got.BuyerID != "buyer-1" || got.Text != "您好" || got.MessageKey != "local-1" {
		t.Fatalf("observation=%+v", got)
	}
}

// TestAutomationSendTextWaitsForOwnEcho 验证自动化文本只有收到匹配自身回显后才返回成功。
func TestAutomationSendTextWaitsForOwnEcho(t *testing.T) {
	// handler 是接收本地出站旁路的测试处理器；本测试只关注发送确认，不依赖数据库。
	handler := &outgoingObserverHandler{}
	// account 是绑定回显确认器的账号运行时。
	account := New(Config{CookieID: "echo-account", CookieStr: "unb=self", Handler: handler})
	// conn 是只记录发送参数的 WebSocket 替身。
	conn := &fakeWSConn{}
	account.runtimeMu.Lock()
	account.conn = conn
	account.runtimeMu.Unlock()
	account.outgoing.echoWaitTimeout = time.Second
	// result 保存异步自动化发送等待回显的结果。
	result := make(chan error, 1)
	go func() {
		result <- account.SendText(WithOutgoingEchoConfirmation(context.Background()), "chat-echo", "buyer-echo", "赠品内容")
	}()
	// deadline 限制测试等待发送写入的最长时间，避免测试替身异常时永久阻塞。
	deadline := time.Now().Add(time.Second)
	// sentBeforeEcho 表示测试替身在注入回显前是否确实收到了一次文本发送。
	sentBeforeEcho := false
	for time.Now().Before(deadline) {
		conn.mu.Lock()
		// sent 表示测试 WebSocket 已经收到的文本数量。
		sent := len(conn.sentTexts)
		conn.mu.Unlock()
		if sent == 1 {
			sentBeforeEcho = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !sentBeforeEcho {
		t.Fatal("自动化文本未写入 WebSocket")
	}
	// echo 是与发送参数一致的账号自身回显摘要。
	account.outgoing.echoTracker.observe(OutgoingChatMessage{ChatID: "chat-echo@goofish", BuyerID: "buyer-echo@goofish", MessageType: "text", Text: "赠品内容"})
	select {
	// err 是自动化发送在收到自身回显后的最终结果。
	case err := <-result:
		if err != nil {
			t.Fatalf("收到自身回显后发送仍失败: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("收到自身回显后发送未结束")
	}
}

// TestAutomationSendTextEchoTimeoutIsUncertain 验证回显超时不会返回可安全重试的确定未发送错误。
func TestAutomationSendTextEchoTimeoutIsUncertain(t *testing.T) {
	// account 是配置极短确认预算的测试账号。
	account := New(Config{CookieID: "echo-timeout", CookieStr: "unb=self"})
	// conn 是记录文本写入但不产生回显的 WebSocket 替身。
	conn := &fakeWSConn{}
	account.runtimeMu.Lock()
	account.conn = conn
	account.runtimeMu.Unlock()
	account.outgoing.echoWaitTimeout = 10 * time.Millisecond
	// err 是发送已写入 WebSocket 但未观察到自身回显的结果。
	err := account.SendText(WithOutgoingEchoConfirmation(context.Background()), "chat-timeout", "buyer-timeout", "待确认赠品")
	if err == nil || !errors.Is(err, errOutgoingEchoUnconfirmed) {
		t.Fatalf("回显超时错误=%v，期望不确定错误", err)
	}
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.sentTexts) != 1 {
		t.Fatalf("回显超时不应重复写入，发送次数=%d", len(conn.sentTexts))
	}
}

// TestMessageDispatcherWakesOutgoingEchoWaiter 验证消息分发器先把自身回显交给确认器，再执行本地旁路。
func TestMessageDispatcherWakesOutgoingEchoWaiter(t *testing.T) {
	// tracker 是账号级回显确认状态。
	tracker := newOutgoingEchoTracker()
	// waiter 是预先登记的自动化文本等待项。
	waiter := tracker.register("chat-dispatch", "buyer-dispatch", "text", "回显文本")
	// dispatcher 是只保留回显观察能力的消息分发器。
	dispatcher := newMessageDispatcher(messageDispatcherConfig{
		CookieID:        "echo-dispatch-account",
		CurrentCookie:   func() string { return "unb=self-dispatch" },
		ObserveOutgoing: tracker.observe,
	})
	// raw 是账号自身普通文本回显的最小协议夹具。
	raw := map[string]any{"1": map[string]any{
		"2": "chat-dispatch@goofish",
		"10": map[string]any{
			"reminderContent": "回显文本",
			"senderUserId":    "self-dispatch",
			"reminderUrl":     "fleamarket://message_chat?peerUserId=buyer-dispatch",
		},
	}}
	dispatcher.handleMessageContext(context.Background(), raw)
	// waitErr 是分发器处理回显后等待项的结果。
	waitErr := waiter.wait(context.Background(), 100*time.Millisecond)
	if waitErr != nil {
		t.Fatalf("分发器未唤醒自身回显等待项: %v", waitErr)
	}
}

// TestExtractOwnWebSocketEchoExtractsImageContent 验证图片自身回显可以提取媒体 URL 用于确认。
func TestExtractOwnWebSocketEchoExtractsImageContent(t *testing.T) {
	// raw 是包含 contentType=2 图片正文的自身回显夹具。
	raw := map[string]any{"1": map[string]any{
		"2": "chat-image@goofish",
		"10": map[string]any{
			"reminderContent": "[图片]",
			"senderUserId":    "self-image",
			"extJson":         `{"contentType":"2"}`,
		},
		"6": map[string]any{"3": map[string]any{
			"4": 2,
			"5": `{"contentType":2,"image":{"pics":[{"url":"https://cdn.example/gift.png"}]}}`,
		}},
	}}
	// echo 是解析后的非敏感图片回显摘要。
	echo := extractOwnWebSocketEcho(raw, "image-account", "unb=self-image")
	if echo == nil || echo.MessageType != "image" || echo.Content != "https://cdn.example/gift.png" {
		t.Fatalf("图片回显解析错误: %+v", echo)
	}
}
