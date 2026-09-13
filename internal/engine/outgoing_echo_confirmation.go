package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// outgoingEchoConfirmationTimeout 限制自动化发送等待自身 WebSocket 回显的最长时间；超时结果必须人工核对。
const outgoingEchoConfirmationTimeout = 5 * time.Second

// errOutgoingEchoUnconfirmed 表示平台发送请求结束后，在确认窗口内没有观察到匹配的自身回显。
var errOutgoingEchoUnconfirmed = errors.New("闲鱼出站消息回显未确认")

// outgoingEchoKey 按会话、消息类型和实际正文索引待确认的出站消息，避免不同会话的回显互相消费。
type outgoingEchoKey struct {
	// chatID 是平台会话标识，已去除闲鱼协议后缀。
	chatID string
	// messageType 是 text 或 image；未提供类型时按 text 兼容。
	messageType string
	// content 是文本正文或图片 URL，不保存到日志。
	content string
}

// outgoingEchoWaiter 表示一条已经登记、等待账号自身 WebSocket 回显的消息。
type outgoingEchoWaiter struct {
	// tracker 是拥有待确认集合的账号级状态；所有字段只在 tracker.mu 保护下修改。
	tracker *outgoingEchoTracker
	// key 是本等待项在 tracker.pending 中的索引。
	key outgoingEchoKey
	// buyerID 是预期接收人；平台回显缺失该字段时允许通过会话和正文继续确认。
	buyerID string
	// done 在匹配回显或等待项被取消时关闭，等待方只读该 channel。
	done chan struct{}
}

// outgoingEchoTracker 由单个账号运行时拥有，串行管理并发发送对应的自身回显等待项。
// tracker.mu 只保护 pending；不在锁内执行 Context 等待、日志或外部 I/O。
type outgoingEchoTracker struct {
	// mu 保护同一账号所有待确认消息的登记、匹配和移除。
	mu sync.Mutex
	// pending 按会话和正文保存尚未确认的自动化出站消息；切片保留相同正文的发送顺序。
	pending map[outgoingEchoKey][]*outgoingEchoWaiter
}

// newOutgoingEchoTracker 创建账号级出站回显确认器。
func newOutgoingEchoTracker() *outgoingEchoTracker {
	return &outgoingEchoTracker{pending: make(map[outgoingEchoKey][]*outgoingEchoWaiter)}
}

// register 登记一条待确认消息；调用方必须在写入 WebSocket 前调用，避免回显先到造成竞态。
func (t *outgoingEchoTracker) register(chatID, buyerID, messageType, content string) *outgoingEchoWaiter {
	if t == nil {
		return nil
	}
	// normalizedType、normalizedContent 保存与回显解析器一致的非空比较字段。
	normalizedType, normalizedContent := normalizeOutgoingEchoType(messageType), strings.TrimSpace(content)
	// waiter 保存本次发送的匹配键和完成信号；它的生命周期不超过一次发送调用。
	waiter := &outgoingEchoWaiter{tracker: t, key: outgoingEchoKey{chatID: normalizeOutgoingIdentity(chatID), messageType: normalizedType, content: normalizedContent}, buyerID: normalizeOutgoingIdentity(buyerID), done: make(chan struct{})}
	t.mu.Lock()
	t.pending[waiter.key] = append(t.pending[waiter.key], waiter)
	t.mu.Unlock()
	return waiter
}

// wait 在有限确认窗口内等待匹配回显；Context 取消同样返回未确认，禁止调用方安全重放。
func (w *outgoingEchoWaiter) wait(ctx context.Context, timeout time.Duration) error {
	if w == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = outgoingEchoConfirmationTimeout
	}
	// timer 是本次回显确认的有限等待计时器；返回时必须释放底层计时资源。
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errOutgoingEchoUnconfirmed
	}
}

// cancel 移除尚未匹配的等待项；发送失败或确认超时后必须调用，避免账号级状态无界增长。
func (w *outgoingEchoWaiter) cancel() {
	if w == nil || w.tracker == nil {
		return
	}
	// t 是当前等待项所属的账号级跟踪器；其锁保护待确认列表。
	t := w.tracker
	t.mu.Lock()
	defer t.mu.Unlock()
	// waiters 是同一会话、消息类型和正文下仍未完成的等待项。
	waiters := t.pending[w.key]
	// index 标识当前候选在待确认列表中的位置；candidate 是待比较的等待项。
	for index, candidate := range waiters {
		if candidate != w {
			continue
		}
		waiters = append(waiters[:index], waiters[index+1:]...)
		if len(waiters) == 0 {
			delete(t.pending, w.key)
		} else {
			t.pending[w.key] = waiters
		}
		return
	}
}

// observe 接收消息分发器识别出的自身回显，并唤醒第一个匹配的自动化发送等待项。
// message 只包含已脱敏的会话、接收人和消息正文摘要，不在此处记录日志。
func (t *outgoingEchoTracker) observe(message OutgoingChatMessage) {
	if t == nil {
		return
	}
	// key 是当前回显按同一会话、类型和正文构造的索引。
	key := outgoingEchoKey{chatID: normalizeOutgoingIdentity(message.ChatID), messageType: normalizeOutgoingEchoType(message.MessageType), content: outgoingEchoContent(message)}
	t.mu.Lock()
	defer t.mu.Unlock()
	// waiters 是当前回显键对应的等待项；只消费一个最早登记的匹配项。
	waiters := t.pending[key]
	// index 标识候选等待项的位置；waiter 是将被本次回显唤醒的对象。
	for index, waiter := range waiters {
		if waiter.buyerID != "" && normalizeOutgoingIdentity(message.BuyerID) != "" && waiter.buyerID != normalizeOutgoingIdentity(message.BuyerID) {
			continue
		}
		waiters = append(waiters[:index], waiters[index+1:]...)
		if len(waiters) == 0 {
			delete(t.pending, key)
		} else {
			t.pending[key] = waiters
		}
		close(waiter.done)
		return
	}
}

// normalizeOutgoingIdentity 统一会话和用户标识的闲鱼协议后缀，保证发送参数与回显字段可比较。
func normalizeOutgoingIdentity(value string) string {
	return strings.TrimSuffix(strings.TrimSpace(value), "@goofish")
}

// normalizeOutgoingEchoType 统一回显消息类型；历史文本观察缺少类型时按 text 处理。
func normalizeOutgoingEchoType(messageType string) string {
	messageType = strings.TrimSpace(messageType)
	if messageType == "" {
		return "text"
	}
	return messageType
}

// outgoingEchoContent 返回回显的实际比较正文；文本优先使用 Text，图片和其他媒体使用 Content。
func outgoingEchoContent(message OutgoingChatMessage) string {
	if normalizeOutgoingEchoType(message.MessageType) == "text" {
		return strings.TrimSpace(message.Text)
	}
	return strings.TrimSpace(message.Content)
}
