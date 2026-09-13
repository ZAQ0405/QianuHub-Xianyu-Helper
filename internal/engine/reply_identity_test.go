package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"xianyu-go/internal/db"
)

// publisherFixture 让测试在本地控制平台发布人查询与取消行为。
type publisherFixture struct {
	// fetch 是每个案例提供的同步查询，不拥有后台任务。
	fetch func(context.Context, string, string) (string, error)
}

// roleHandlerFixture 在内存中保存会话角色，并复用 recordingHandler 观察消息业务旁路。
type roleHandlerFixture struct {
	*recordingHandler
	// roleMu 保护角色快照及读写次数。
	roleMu sync.Mutex
	// role 保存当前会话与商品绑定的角色结论。
	role db.ChatSession
	// roleReads 和 roleWrites 分别记录本地角色读取与持久化次数。
	roleReads, roleWrites int
	// roleWriteErr 是角色持久化夹具按场景返回的错误。
	roleWriteErr error
	// handled 在消息业务处理完成后接收通知，供时序测试建立屏障。
	handled chan struct{}
	// handledOnce 保证多条消息只发送一次完成通知。
	handledOnce sync.Once
}

// HandleChatMessage 先记录业务消息，再通知时序测试本地处理已经完成。
func (fixture *roleHandlerFixture) HandleChatMessage(ctx context.Context, message ChatMessage) error {
	// err 保存基础记录处理器的执行结果。
	err := fixture.recordingHandler.HandleChatMessage(ctx, message)
	if fixture.handled != nil {
		fixture.handledOnce.Do(func() { fixture.handled <- struct{}{} })
	}
	return err
}

// ChatSessionRole 返回内存角色；角色绑定商品不一致时返回 unknown。
func (fixture *roleHandlerFixture) ChatSessionRole(_ context.Context, accountID, chatID, itemID string) (db.ChatSession, error) {
	fixture.roleMu.Lock()
	defer fixture.roleMu.Unlock()
	fixture.roleReads++
	if fixture.role.RoleItemID != itemID {
		return db.ChatSession{CookieID: accountID, ChatID: chatID, ItemID: itemID, AccountRole: "unknown"}, nil
	}
	return fixture.role, nil
}

// SaveChatSessionRole 固定首次平台核验结论，供后续消息只读本地缓存。
func (fixture *roleHandlerFixture) SaveChatSessionRole(_ context.Context, accountID, chatID, itemID, accountRole, buyerUserID, sellerUserID, roleSource string) error {
	fixture.roleMu.Lock()
	defer fixture.roleMu.Unlock()
	fixture.roleWrites++
	if fixture.roleWriteErr != nil {
		return fixture.roleWriteErr
	}
	fixture.role = db.ChatSession{CookieID: accountID, ChatID: chatID, ItemID: itemID, AccountRole: accountRole, BuyerUserID: buyerUserID, SellerUserID: sellerUserID, RoleItemID: itemID, RoleSource: roleSource}
	return nil
}

// TestBuyerMessagesRemainVisibleWithoutAutoReply 验证真实防抖分发链保留买家侧聊天观察，并在 API 和默认回复之前检查身份；t 管理夹具。
func TestBuyerMessagesRemainVisibleWithoutAutoReply(t *testing.T) {
	// seller 表示当前账号是否是会话商品发布人。
	for _, seller := range []bool{false, true} {
		// apiReply 表示本次使用 API 回复或默认回复，验证身份门禁位于整个回复链之前。
		for _, apiReply := range []bool{false, true} {
			// store、cleanup 提供独立数据库，防止默认回复发送记录跨场景污染。
			store, cleanup := newReplyStore(t)
			// fixtureErr 是默认回复配置写入结果。
			if _, fixtureErr := store.DB.ExecContext(context.Background(), `INSERT INTO default_replies (cookie_id,enabled,reply_content) VALUES ('cid',1,'默认回复')`); fixtureErr != nil {
				cleanup()
				t.Fatal(fixtureErr)
			}
			// sender 记录真实回复链发送的文本和图片；主测试仅在完成通道关闭后读取。
			sender := &recordingSender{}
			// api 提供可计数的 API 回复，禁止买家侧执行外部回复查询。
			api := &fakeAPIReplier{result: &ReplyResult{Text: "API 回复"}}
			// reply 是待测回复链，默认场景不装配 API 优先层。
			reply := NewReplyService("cid", store, sender, nil, nil, nil)
			if apiReply {
				reply.api = api
			}
			// observer 记录发给业务旁路的聊天，买卖双方均应保留。
			observer := &roleHandlerFixture{recordingHandler: &recordingHandler{}}
			// done 在防抖任务完全结束后关闭，建立异步写入与断言的同步关系。
			done := make(chan struct{})
			// publisherCalls 记录首次未知角色触发的平台核验次数。
			publisherCalls := 0
			// dispatcher 注入纯本地发布人查询和完成通知，不启动账号网络连接。
			dispatcher := newMessageDispatcher(messageDispatcherConfig{
				CookieID: "cid", Reply: reply,
				CurrentCookie:  func() string { return "unb=123" },
				CurrentHandler: func() Handler { return observer },
				BeginTask: func() (context.Context, func(), bool) {
					return context.Background(), func() { close(done) }, true
				},
				ItemPublisher: publisherFixture{fetch: func(context.Context, string, string) (string, error) {
					publisherCalls++
					if seller {
						return "123", nil
					}
					return "other-seller", nil
				}},
			})
			dispatcher.scheduleDebouncedReply(chatMsg("你好", "conversation-item", "conversation"))
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("防抖消息未完成")
			}
			dispatcher.stop()
			cleanup()
			if len(observer.chats) != 1 {
				t.Fatal("身份门禁不应丢弃买家侧聊天消息")
			}
			if seller && len(sender.texts) != 1 {
				t.Fatal("卖家没有收到预期的自动回复")
			}
			if !seller && (len(sender.texts) != 0 || len(sender.images) != 0 || api.called != 0) {
				t.Fatal("买家侧错误进入自动回复或发送消息")
			}
			if publisherCalls != 1 || observer.roleWrites != 2 {
				t.Fatalf("首次未知角色应先登记核验再保存明确结果: query=%d save=%d", publisherCalls, observer.roleWrites)
			}
		}
	}
}

// FetchItemPublisher 把 ctx、cookies 和 itemID 转交本地夹具，返回发布人标识或查询错误。
func (fixture publisherFixture) FetchItemPublisher(ctx context.Context, cookies, itemID string) (string, error) {
	return fixture.fetch(ctx, cookies, itemID)
}

// TestReplyIdentityGate 验证仅当前账号发布的会话商品可以自动回复；t 负责测试生命周期。
func TestReplyIdentityGate(t *testing.T) {
	// cases 包含卖家、买家、未知身份、查询失败、取消和请求期间身份切换。
	cases := []struct {
		// name 是场景名称。
		name string
		// publisher 是平台返回的商品发布人。
		publisher string
		// failure 控制平台查询失败。
		failure bool
		// cancel 控制请求期间账号生命周期取消。
		cancel bool
		// switchAccount 控制请求期间当前账号身份变化。
		switchAccount bool
		// allow 是是否允许发送自动回复的预期。
		allow bool
	}{
		{"卖家", "self", false, false, false, true},
		{"卖家后缀", "self@goofish", false, false, false, true},
		{"买家", "other", false, false, false, false},
		{"缺少发布人", "", false, false, false, false},
		{"查询失败", "self", true, false, false, false},
		{"账号取消", "self", false, true, false, false},
		{"账号切换", "self", false, false, true, false},
	}
	// scenario 是待验证的身份核验边界。
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			// ctx、cancel 模拟账号运行时拥有的取消预算。
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// cookies 是本地合成身份，查询回调可能模拟账号切换。
			cookies := "unb=self"
			// observer 提供 unknown 初始角色并记录平台核验后的持久化结果。
			observer := &roleHandlerFixture{recordingHandler: &recordingHandler{}}
			// dispatcher 通过构造依赖注入平台夹具，不访问真实闲鱼账号。
			dispatcher := newMessageDispatcher(messageDispatcherConfig{
				CurrentCookie:  func() string { return cookies },
				CurrentHandler: func() Handler { return observer },
				ItemPublisher: publisherFixture{fetch: func(queryCtx context.Context, queryCookies, itemID string) (string, error) {
					if queryCookies != cookies || itemID != "conversation-item" {
						t.Error("未使用当前账号和会话商品")
					}
					if _, bounded := queryCtx.Deadline(); !bounded { // bounded 确认查询具有有限超时。
						t.Error("查询缺少超时")
					}
					if scenario.cancel {
						cancel()
					}
					if scenario.switchAccount {
						cookies = "unb=changed"
					}
					if scenario.failure {
						return "", errors.New("本地模拟查询失败")
					}
					return scenario.publisher, nil
				}},
			})
			if dispatcher.canAutoReply(ctx, ChatMessage{AccountID: "cid", ChatID: "chat", ItemID: "conversation-item", SenderUserID: "peer"}) != scenario.allow {
				t.Fatal("自动回复身份门禁结果不符")
			}
		})
	}
}

// TestReplyIdentityUsesPersistedRole 验证首次核验后同一会话商品只读取本地角色，不再访问平台。
func TestReplyIdentityUsesPersistedRole(t *testing.T) {
	// observer 保存首次平台核验写入的卖家角色。
	observer := &roleHandlerFixture{recordingHandler: &recordingHandler{}}
	// publisherCalls 记录平台商品发布者查询次数。
	publisherCalls := 0
	// dispatcher 使用固定账号身份和可计数平台查询。
	dispatcher := newMessageDispatcher(messageDispatcherConfig{
		CurrentCookie:  func() string { return "unb=self" },
		CurrentHandler: func() Handler { return observer },
		ItemPublisher: publisherFixture{fetch: func(context.Context, string, string) (string, error) {
			publisherCalls++
			return "self", nil
		}},
	})
	// message 是绑定固定账号、会话和商品的入站消息。
	message := ChatMessage{AccountID: "cid", ChatID: "chat", ItemID: "item", SenderUserID: "buyer"}
	// firstAllowed 是首次未知角色经平台核验后的自动回复结论。
	firstAllowed := dispatcher.canAutoReply(context.Background(), message)
	// secondAllowed 是同一会话后续只读本地角色的自动回复结论。
	secondAllowed := dispatcher.canAutoReply(context.Background(), message)
	if !firstAllowed || !secondAllowed {
		t.Fatal("持久卖家角色应允许后续自动回复")
	}
	if publisherCalls != 1 || observer.roleWrites != 2 || observer.roleReads != 2 {
		t.Fatalf("角色缓存没有消除重复平台查询: query=%d read=%d write=%d", publisherCalls, observer.roleReads, observer.roleWrites)
	}
}

// TestReplyIdentityUnresolvedResultSkipsRepeatedPlatformLookup 验证失败或空结果会固定为不可回复状态，后续消息不再访问平台。
func TestReplyIdentityUnresolvedResultSkipsRepeatedPlatformLookup(t *testing.T) {
	// scenarios 描述平台明确失败和缺失发布人两类无法建立角色的结果。
	scenarios := []struct {
		// name 是子测试名称。
		name string
		// publisher 是本地平台夹具返回的发布人标识。
		publisher string
		// err 是本地平台夹具返回的查询错误。
		err error
	}{
		{name: "查询失败", err: errors.New("本地模拟查询失败")},
		{name: "发布人为空"},
	}
	// scenario 是当前无法确认角色的测试输入。
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// observer 保存无法确认的持久标记，模拟真实会话仓储。
			observer := &roleHandlerFixture{recordingHandler: &recordingHandler{}}
			// publisherCalls 记录同一会话商品的平台身份查询次数。
			publisherCalls := 0
			// dispatcher 注入可计数查询，所有操作均为本地夹具。
			dispatcher := newMessageDispatcher(messageDispatcherConfig{
				CurrentCookie:  func() string { return "unb=self" },
				CurrentHandler: func() Handler { return observer },
				ItemPublisher: publisherFixture{fetch: func(context.Context, string, string) (string, error) {
					publisherCalls++
					return scenario.publisher, scenario.err
				}},
			})
			// message 是连续到达两次的同一会话商品消息。
			message := ChatMessage{AccountID: "cid", ChatID: "chat", ItemID: "item", SenderUserID: "peer"}
			// firstAllowed 是首次平台核验后的拒绝结果。
			firstAllowed := dispatcher.canAutoReply(context.Background(), message)
			// secondAllowed 是同一商品后续仅查本地标记后的拒绝结果。
			secondAllowed := dispatcher.canAutoReply(context.Background(), message)
			if firstAllowed || secondAllowed {
				t.Fatal("无法确认角色时不应允许自动回复")
			}
			if publisherCalls != 1 || observer.roleWrites != 1 || observer.role.RoleSource != "platform_unresolved" {
				t.Fatalf("无法确认结果没有阻止重复平台查询: query=%d save=%d source=%q", publisherCalls, observer.roleWrites, observer.role.RoleSource)
			}
		})
	}
}

// TestReplyIdentityClaimFailureSkipsPlatform 验证核验标记无法落库时不会访问商品详情。
func TestReplyIdentityClaimFailureSkipsPlatform(t *testing.T) {
	// observer 模拟本地角色标记写入失败。
	observer := &roleHandlerFixture{recordingHandler: &recordingHandler{}, roleWriteErr: errors.New("本地模拟写入失败")}
	// dispatcher 的平台夹具一旦被调用就立即使测试失败。
	dispatcher := newMessageDispatcher(messageDispatcherConfig{
		CurrentCookie:  func() string { return "unb=self" },
		CurrentHandler: func() Handler { return observer },
		ItemPublisher: publisherFixture{fetch: func(context.Context, string, string) (string, error) {
			t.Fatal("核验标记写入失败后不应访问平台")
			return "", nil
		}},
	})
	// allowed 是本地写入失败时的自动回复结论。
	allowed := dispatcher.canAutoReply(context.Background(), ChatMessage{AccountID: "cid", ChatID: "chat", ItemID: "item", SenderUserID: "peer"})
	if allowed || observer.roleWrites != 1 {
		t.Fatalf("核验标记写入失败边界不符: allow=%v writes=%d", allowed, observer.roleWrites)
	}
}

// TestReplyIdentityKnownRolesSkipPlatform 验证本地已确认的卖家或买家角色都不会再次查询商品发布者。
func TestReplyIdentityKnownRolesSkipPlatform(t *testing.T) {
	// accountRole 和 allow 分别表示本地角色及自动回复预期。
	for accountRole, allow := range map[string]bool{"seller": true, "buyer": false} {
		t.Run(accountRole, func(t *testing.T) {
			// role 保存与当前商品绑定的完整参与方结论。
			role := db.ChatSession{CookieID: "cid", ChatID: "chat", ItemID: "item", AccountRole: accountRole, BuyerUserID: "buyer", SellerUserID: "self", RoleItemID: "item", RoleSource: "local_item"}
			// observer 返回已持久化角色并记录本地读取次数。
			observer := &roleHandlerFixture{recordingHandler: &recordingHandler{}, role: role}
			// dispatcher 注入会让测试立即失败的平台查询，证明已知角色路径保持零调用。
			dispatcher := newMessageDispatcher(messageDispatcherConfig{
				CurrentCookie:  func() string { return "unb=self" },
				CurrentHandler: func() Handler { return observer },
				ItemPublisher: publisherFixture{fetch: func(context.Context, string, string) (string, error) {
					t.Fatal("已知会话角色不应查询平台发布者")
					return "", nil
				}},
			})
			// message 是与角色缓存完全一致的会话商品消息。
			message := ChatMessage{AccountID: "cid", ChatID: "chat", ItemID: "item", SenderUserID: "peer"}
			if dispatcher.canAutoReply(context.Background(), message) != allow {
				t.Fatalf("known role %s allow mismatch", accountRole)
			}
		})
	}
}

// TestMessagePersistencePrecedesPublisherLookup 验证慢速平台核验不会延迟消息进入本地业务处理链。
func TestMessagePersistencePrecedesPublisherLookup(t *testing.T) {
	// handled 和 releasePublisher 分别通知消息已处理并控制平台核验返回。
	handled, releasePublisher := make(chan struct{}, 1), make(chan struct{})
	// observer 提供 unknown 角色并报告消息处理完成。
	observer := &roleHandlerFixture{recordingHandler: &recordingHandler{}, handled: handled}
	// done 表示完整防抖任务已经退出。
	done := make(chan struct{})
	// dispatcher 的发布者查询会阻塞，模拟慢速外部调用而不接触真实平台。
	dispatcher := newMessageDispatcher(messageDispatcherConfig{
		CookieID: "cid", CurrentCookie: func() string { return "unb=self" }, CurrentHandler: func() Handler { return observer },
		BeginTask: func() (context.Context, func(), bool) { return context.Background(), func() { close(done) }, true },
		ItemPublisher: publisherFixture{fetch: func(context.Context, string, string) (string, error) {
			<-releasePublisher
			return "other", nil
		}},
	})
	dispatcher.scheduleDebouncedReply(chatMsg("你好", "item", "chat"))
	select {
	case <-handled:
	case <-time.After(3 * time.Second):
		t.Fatal("消息处理被平台身份查询阻塞")
	}
	close(releasePublisher)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("释放平台查询后任务未退出")
	}
	dispatcher.stop()
}

// TestReplyIdentityMissingInputs 验证缺少商品、查询能力或当前账号身份时不得猜测卖家身份；t 管理断言。
func TestReplyIdentityMissingInputs(t *testing.T) {
	// dispatcher 故意没有平台身份查询能力。
	dispatcher := newMessageDispatcher(messageDispatcherConfig{})
	if dispatcher.canAutoReply(context.Background(), ChatMessage{ItemID: "item"}) {
		t.Fatal("缺少查询能力仍允许回复")
	}
	dispatcher.itemPublisher = publisherFixture{fetch: func(context.Context, string, string) (string, error) {
		t.Fatal("缺少身份或商品时不应调用平台")
		return "", nil
	}}
	if dispatcher.canAutoReply(context.Background(), ChatMessage{}) || dispatcher.canAutoReply(context.Background(), ChatMessage{ItemID: "item"}) {
		t.Fatal("缺少身份或商品仍允许回复")
	}
}
