package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"xianyu-go/internal/db"
	"xianyu-go/internal/xianyu/mtop"
)

// errAutomationDeferred 用于本次流程后续判断的err自动化Deferred
var (
	errAutomationDeferred    = errors.New("自动化动作已持久化等待执行")
	errAutomationNeedsReview = errors.New("自动化动作结果需要人工核对")
	errAutomationQuarantine  = errors.New("自动化人工核对状态保存失败")
	errActionNotPerformed    = errors.New("自动化外部动作明确未执行")
	// ErrMessageNotSent 记录当前操作失败原因发生在调用 WebSocket 发送之前，可以安全重试。
	// engine 通过 errors.Is 标记“连接尚未就绪”等运行时状态，避免被误判为
	// 远端可能已经收到消息。
	ErrMessageNotSent = errors.New("自动化消息确定未发送")
)

// preparationError 用于本次流程后续判断的preparation错误
type preparationError struct{ err error }

// Error 封装错误业务协调。
func (e *preparationError) Error() string { return e.err.Error() }

// Unwrap 封装Unwrap业务协调。
func (e *preparationError) Unwrap() error { return e.err }

// uncertainActionError 用于本次流程后续判断的uncertain动作错误
type uncertainActionError struct{ err error }

// Error 封装错误业务协调。
func (e *uncertainActionError) Error() string { return e.err.Error() }

// Unwrap 封装Unwrap业务协调。
func (e *uncertainActionError) Unwrap() error { return e.err }

// uncertainAction 封装uncertain动作业务协调。
func uncertainAction(err error) error {
	if err == nil {
		return nil
	}
	// existing 用于本次流程后续判断的existing
	var existing *uncertainActionError
	if errors.As(err, &existing) {
		return err
	}
	return &uncertainActionError{err: err}
}

// SenderProvider 根据账号 ID 提供当前在线账号的发送能力。
// 计划任务和 WS 事件都复用这个抽象，避免自动化中心直接依赖 account.Manager。
// SenderProvider 用于本次流程后续判断的SenderProvider
type SenderProvider interface {
	Sender(cookieID string) (MessageSender, bool)
}

// MessageSender 是自动化动作需要的最小发送接口。
type MessageSender interface {
	SendText(ctx context.Context, chatID, toUserID, text string) error
	SendImage(ctx context.Context, chatID, toUserID, imageURL string, cardID int64, width, height int) error
	UpdateCookie(cookieStr string)
}

// automationReadySender 由能够报告实时连接状态的发送器选择性实现。
// 未实现的测试或外部发送器保持向后兼容，视为已经就绪。
// automationReadySender 用于本次流程后续判断的自动化ReadySender
type automationReadySender interface {
	AutomationReady() bool
}

// OrderDetailFetcher 查询闲鱼订单详情。自动发货必须先拿到订单规格和购买数量，
// 再按规则里的规格映射发货。
// OrderDetailFetcher 用于本次流程后续判断的订单DetailFetcher
type OrderDetailFetcher interface {
	FetchOrderDetail(ctx context.Context, cookieID, orderID, itemID, buyerID, cookieStr string) (*OrderDetail, error)
}

// CredentialRecoverer 在平台明确返回 Session 失效时执行一次凭证恢复。
type CredentialRecoverer interface {
	RecoverExpiredCredential(ctx context.Context, cookieID string) bool
}

// Notifier 发货结果通知器（多渠道）。可选依赖，未注入时跳过通知；实现必须按自动化运行和终态持久化去重。
type Notifier interface {
	// NotifyAutomationRun 将自动化运行终态写入可持久化去重的通知链路；runID 和 status 共同标识同一次结果。
	NotifyAutomationRun(ctx context.Context, runID int64, accountID, buyerID, itemID, status, message, chatID string)
}

// Center 是统一自动化处理中心。
// 它只接收已经分类好的系统事件或计划任务，不处理用户消息；用户消息由 engine 的回复链处理。
// Center 用于本次流程后续判断的Center
type Center struct {
	// facts 记录已解析的自动化事件事实，不执行规则动作。
	facts eventFactRecorder
	// rules 查询适用规则，不执行规则动作。
	rules ruleMatcher
	// planner 生成不可变动作计划，不执行外部 I/O。
	planner actionPlanner
	// runs 协调自动化运行创建、动作检查点、延迟恢复和结果隔离。
	runs automationRunCoordinator
	// actions 执行确认发货、卡密和消息动作，集中维护外部副作用边界。
	actions automationActionExecutor
	// notifications 负责把运行结果转换为可选的发货通知。
	notifications deliveryNotifier
	// taskRunner 协调账号任务执行、凭证阻断和账号状态门禁。
	taskRunner accountTaskCoordinator
	store      *db.Store
	senders    SenderProvider
	logger     *slog.Logger
	// dependencies 拥有生产构造时固定的外部依赖快照，运行期间不可替换。
	dependencies centerDependencies
}

// centerDependencies 集中拥有自动化中心构造后不可变的外部依赖。
type centerDependencies struct {
	// mtop 是确认发货使用的 MTOP 客户端。
	mtop mtop.Client
	// accountTaskClient 是账号任务使用的平台客户端。
	accountTaskClient AccountTaskClient
	// fetcher 是订单详情查询器。
	fetcher OrderDetailFetcher
	// recoverer 是凭证恢复器。
	recoverer CredentialRecoverer
	// notifier 是发货结果通知器。
	notifier Notifier
	// cookieSrc 是构造期注入的 Cookie 读取函数。
	cookieSrc func(context.Context, string) (string, error)
	// apiFetcher 提供普通 API 卡发货请求能力。
	apiFetcher APICardFetcher
	// republisher 提供付款后原商品重新上架能力。
	republisher ItemRepublisher
}

// New 构造使用默认协议实现的自动化中心。
func New(store *db.Store, senders SenderProvider, logger *slog.Logger) *Center {
	return NewWithDependencies(store, senders, logger, CenterDependencies{})
}

// NewWithDependencies 构造依赖固定的自动化中心，生产装配应通过该入口注入外部能力。
func NewWithDependencies(store *db.Store, senders SenderProvider, logger *slog.Logger, dependencies CenterDependencies) *Center {
	if logger == nil {
		logger = slog.Default()
	}
	// client 保存确认发货使用的固定 MTOP 客户端。
	client := dependencies.MTop
	// defaultTaskClient 保存默认 MTOP 实现同时提供的账号任务能力。
	var defaultTaskClient AccountTaskClient
	if client == nil {
		// defaultClient 保存默认协议客户端的具体实现，供两个消费者共享。
		defaultClient := mtop.NewClient()
		client = defaultClient
		defaultTaskClient = defaultClient
	}
	// accountTaskClient 保存自动评价和商品擦亮使用的固定协议客户端。
	accountTaskClient := dependencies.AccountTaskClient
	if accountTaskClient == nil {
		accountTaskClient = defaultTaskClient
	}
	// center 用于本次流程后续判断的center
	center := &Center{
		facts:   newEventFactRecorder(store),
		rules:   newRuleMatcher(store),
		planner: actionPlanner{},
		store:   store,
		senders: senders,
		dependencies: centerDependencies{
			mtop:              client,
			accountTaskClient: accountTaskClient,
			fetcher:           dependencies.OrderDetailFetcher,
			notifier:          dependencies.Notifier,
			cookieSrc:         dependencies.CookieSource,
			apiFetcher:        dependencies.APICardFetcher,
			republisher:       dependencies.Republisher,
		},
		logger: logger.With("subsys", "automation"),
	}
	if // recoverer、ok 保存订单详情查询器提供的凭证恢复能力及类型判断结果
	recoverer, ok := center.dependencies.fetcher.(CredentialRecoverer); ok {
		center.dependencies.recoverer = recoverer
	}
	center.actions = automationActionExecutor{
		store:   store,
		senders: senders,
		mtop: func() mtop.Client {
			return center.dependencies.mtop
		},
		recoverer: func() CredentialRecoverer {
			return center.dependencies.recoverer
		},
		logger: center.logger,
		cookieSource: func(ctx context.Context, cookieID string) (string, error) {
			// source 是构造期固定的 Cookie 读取函数；为空时回退到仓储。
			if source := center.dependencies.cookieSrc; source != nil {
				return source(ctx, cookieID)
			}
			return center.store.Cookies.GetValue(ctx, cookieID)
		},
		apiFetcher:            func() APICardFetcher { return center.dependencies.apiFetcher },
		republisher:           func() ItemRepublisher { return center.dependencies.republisher },
		wakeCredentialBlocked: center.wakeCredentialBlockedAutomation,
	}
	center.notifications = deliveryNotifier{
		current: func() Notifier {
			return center.dependencies.notifier
		},
		logger: center.logger,
	}
	center.taskRunner = accountTaskCoordinator{
		repository: newStoreAccountTaskRepository(store),
		client:     func() AccountTaskClient { return center.dependencies.accountTaskClient },
		senders:    senders,
		recoverer: func() CredentialRecoverer {
			return center.dependencies.recoverer
		},
		logger: center.logger,
	}
	center.runs = automationRunCoordinator{
		store:                    store,
		planner:                  center.planner,
		logger:                   center.logger,
		prepareTask:              center.prepareTask,
		actionDelaySeconds:       center.actionDelaySeconds,
		accountAutomationAllowed: center.accountAutomationAllowed,
		accountSenderReady:       center.accountSenderReady,
		deferTask:                center.deferTask,
		executeAction:            center.executeActionWithProof,
		hasNotifier:              func() bool { return center.dependencies.notifier != nil },
		notifyResult:             center.notifyResult,
	}
	return center
}

// RunAccountTask 执行指定账号任务，并保持 Center 的公开兼容入口。
func (c *Center) RunAccountTask(ctx context.Context, accountID, taskType string) (AccountTaskSummary, error) {
	return c.taskRunner.runAccountTask(ctx, accountID, taskType)
}

// scanAccountTasks 扫描启用的账号任务，并委托给账号任务协调器。
func (c *Center) scanAccountTasks(ctx context.Context) {
	c.taskRunner.scanAccountTasks(ctx)
}

// HandleTask 处理一条自动化任务。无匹配规则时安全忽略。
func (c *Center) HandleTask(ctx context.Context, task Task) error {
	// err 用于本次流程后续判断的err
	_, err := c.handleTask(ctx, task)
	return err
}

// handleTask 封装handle任务业务协调。
func (c *Center) handleTask(ctx context.Context, task Task) (bool, error) {
	if c == nil || c.store == nil || c.store.Automation == nil {
		return false, nil
	}
	task.TriggerType = strings.TrimSpace(task.TriggerType)
	if task.TriggerType == "" || task.AccountID == "" {
		return false, nil
	}
	// 简化付款消息只有会话标识，先从本账号待发货订单回填订单事实，再记录事件和匹配规则。
	if // resolveErr 保存简化消息订单事实回填错误
	resolvedTask, resolveErr := c.resolvePaidTaskOrder(ctx, task); resolveErr != nil {
		return false, resolveErr
	} else {
		task = resolvedTask
	}
	if // err 用于本次流程后续判断的err
	err := c.facts.record(ctx, task); err != nil {
		if errors.Is(err, db.ErrForbidden) {
			// 同一平台交易卡片可能同时送到卖家和买家登录账号；订单已归属其他账号时，绝不能覆盖或执行该副本。
			c.logger.Debug("跨账号重复订单事件，忽略", "account", task.AccountID, "trigger", task.TriggerType, "order_id", task.OrderID)
			return false, nil
		}
		return false, err
	}
	if task.OrderID != "" {
		if // order、orderErr 用于本次流程后续判断的order、orderErr
		order, orderErr := c.store.Orders.Get(ctx, task.OrderID); orderErr == nil && order != nil {
			task = mergeOrderIntoTask(task, order)
		} else if orderErr != nil && !errors.Is(orderErr, db.ErrNotFound) {
			return false, fmt.Errorf("读取自动化事件订单事实: %w", orderErr)
		}
	}
	// paused、until、err 用于本次流程后续判断的paused、until、err
	paused, until, err := c.store.Cookies.IsPaused(ctx, task.AccountID)
	if err != nil {
		return false, err
	}
	if paused {
		if // err 用于本次流程后续判断的err
		err := c.deferTask(ctx, task, until); err != nil {
			return false, err
		}
		c.logger.Info("账号已暂停，自动化事件已持久化等待恢复", "account", task.AccountID, "trigger", task.TriggerType, "due_at", until)
		return true, nil
	}
	// enabled、err 用于本次流程后续判断的enabled、err
	enabled, err := c.store.Cookies.Status(ctx, task.AccountID)
	if err != nil {
		return false, err
	}
	if !enabled {
		c.logger.Info("账号已停用，记录事件事实但不执行自动化", "account", task.AccountID, "trigger", task.TriggerType)
		return false, nil
	}
	// rules、err 用于本次流程后续判断的rules、err
	rules, err := c.rules.match(ctx, task)
	if err != nil {
		return false, err
	}
	if len(rules) == 0 {
		c.logger.Warn("未匹配商品规则或已确认的账号通用规则，未执行自动化", "trigger", task.TriggerType, "order_id", task.OrderID, "item_id", task.ItemID)
		return false, nil
	}
	if task.TriggerType == TriggerOrderPaid && !task.ForceConfirmShipment && !rulesHaveExplicitShipmentAction(rules) {
		// autoConfirm、autoConfirmErr 分别保存没有显式确认动作时的账号级自动发货开关和读取错误。
		autoConfirm, autoConfirmErr := c.store.Cookies.GetAutoConfirm(ctx, task.AccountID)
		if autoConfirmErr != nil {
			return false, fmt.Errorf("读取自动确认发货设置: %w", autoConfirmErr)
		}
		if !autoConfirm {
			c.logger.Info("账号未启用自动确认发货，跳过付款自动发货", "account", task.AccountID, "order_id", task.OrderID)
			return false, nil
		}
	}
	// firstErr 用于本次流程后续判断的firstErr
	var firstErr error
	// rule 表示当前遍历过程中的规则
	for _, rule := range rules {
		if // err 用于本次流程后续判断的err
		err := c.executeRule(ctx, task, rule); err != nil {
			// prepErr 用于本次流程后续判断的prepErr
			var prepErr *preparationError
			if errors.As(err, &prepErr) && !isDeferredReplay(task) {
				// dueAt 用于本次流程后续判断的dueAt
				dueAt := time.Now().UTC().Add(time.Minute).Unix()
				if // deferErr 用于本次流程后续判断的deferErr
				deferErr := c.deferTaskWithError(ctx, task, dueAt, prepErr.Error()); deferErr != nil {
					return false, errors.Join(err, fmt.Errorf("持久化前置失败任务: %w", deferErr))
				}
				c.logger.Warn("自动化前置处理失败，任务已持久化等待恢复", "account", task.AccountID,
					"order_id", task.OrderID, "trigger", task.TriggerType, "retry_at", dueAt, "err", err)
				return true, nil
			}
			if errors.Is(err, errAutomationDeferred) {
				return true, nil
			}
			c.logger.Error("自动化规则执行失败", "rule_id", rule.ID, "trigger", task.TriggerType, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return false, firstErr
}

// rulesHaveExplicitShipmentAction 判断匹配规则是否明确包含自动确认发货动作。
func rulesHaveExplicitShipmentAction(rules []db.AutomationRule) bool {
	// rule 表示当前匹配到的自动化规则。
	for _, rule := range rules {
		// action 表示当前规则中的一个动作。
		for _, action := range rule.Actions {
			if action.Enabled && action.ActionType == ActionConfirmShipment {
				return true
			}
		}
	}
	return false
}

// taskAutomationRunID 封装任务自动化运行ID业务协调。
func taskAutomationRunID(task Task) int64 {
	if task.Raw == nil {
		return 0
	}
	// value 用于本次流程后续判断的值
	value := fmt.Sprint(task.Raw["automation_run_id"])
	// id 用于本次流程后续判断的标识
	id, _ := strconv.ParseInt(value, 10, 64)
	return id
}

// taskDelayCursor 封装任务延迟游标业务协调。
func taskDelayCursor(task Task) int {
	if task.Raw == nil {
		return -1
	}
	// value 用于本次流程后续判断的值
	value := fmt.Sprint(task.Raw["automation_delay_cursor"])
	// cursor、err 用于本次流程后续判断的cursor、err
	cursor, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return cursor
}

// isDeferredReplay 封装isDeferredReplay业务协调。
func isDeferredReplay(task Task) bool {
	return task.Raw != nil && task.Raw["automation_deferred_replay"] == true
}

// deferTask 封装defer任务业务协调。
func (c *Center) deferTask(ctx context.Context, task Task, dueAt int64) error {
	return c.deferTaskWithError(ctx, task, dueAt, "")
}

// deferTaskWithError 封装defer任务With错误业务协调。
func (c *Center) deferTaskWithError(ctx context.Context, task Task, dueAt int64, errMsg string) error {
	// key 用于本次流程后续判断的key
	key := buildTriggerKey(task)
	if key == "" {
		return fmt.Errorf("暂停期间的自动化事件缺少可持久化防重键")
	}
	task.CookieStr = ""
	// raw、err 用于本次流程后续判断的raw、err
	raw, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return c.store.Automation.DeferTask(ctx, db.DeferredAutomationTask{
		TaskKey: task.AccountID + ":" + key, CookieID: task.AccountID,
		TriggerType: task.TriggerType, TaskJSON: string(raw), DueAt: dueAt, ErrorMessage: errMsg,
	})
}

// executeRule 将规则执行委托给运行协调器，保持 Center 的兼容调用入口。
func (c *Center) executeRule(ctx context.Context, task Task, rule db.AutomationRule) error {
	return c.runs.executeRule(ctx, task, rule)
}

// executeRunActions 将动作执行委托给运行协调器，保持手动发货等调用方的兼容入口。
func (c *Center) executeRunActions(ctx context.Context, task Task, ruleID int64, run *db.AutomationRun, actions []db.AutomationAction, skipDelays bool) (int, bool, error) {
	return c.runs.executeRunActions(ctx, task, ruleID, run, actions, skipDelays)
}

// notifyResult 根据规则执行结果发送通知。成功且实际发出了内容才通知，
// 避免无匹配动作的空跑刷屏；每次通知都以运行 ID 和终态持久化去重。
// notifyResult 封装notify结果业务协调。
func (c *Center) notifyResult(ctx context.Context, task Task, runID int64, status string, sent int, errMsg string) {
	c.notifications.notifyResult(ctx, task, runID, status, sent, errMsg)
}

// notifyRunNeedsReview 封装notify运行NeedsReview业务协调。
func (c *Center) notifyRunNeedsReview(ctx context.Context, run db.AutomationRun, reason string) {
	if c == nil {
		return
	}
	c.notifications.notifyRunNeedsReview(ctx, run, reason)
}

// actionDelaySeconds 统一卡密默认延时和动作覆盖语义。旧动作没有
// delay_override 字段时自动使用卡密上的默认延时。
// actionDelaySeconds 封装动作延迟秒数业务协调。
func (c *Center) actionDelaySeconds(ctx context.Context, action db.AutomationAction) (int, error) {
	if action.ActionType != ActionSendCard || action.CardID <= 0 {
		return action.DelaySeconds, nil
	}
	// cfg 用于本次流程后续判断的cfg
	var cfg struct {
		DelayOverride bool `json:"delay_override"`
	}
	_ = json.Unmarshal([]byte(action.ConfigJSON), &cfg)
	// card、err 用于本次流程后续判断的card、err
	card, err := c.store.Cards.GetSummary(ctx, action.CardID)
	if err != nil {
		return 0, err
	}
	if !card.Enabled {
		return 0, fmt.Errorf("卡密组 %d 已停用", card.ID)
	}
	if cfg.DelayOverride {
		return action.DelaySeconds, nil
	}
	return card.DelaySeconds, nil
}

// prepareTask 封装prepare任务业务协调。
func (c *Center) prepareTask(ctx context.Context, task Task) (Task, error) {
	// task、err 分别表示补全买家信息后的任务快照与准备阶段错误。
	task, err := c.prepareBuyerNickname(ctx, task)
	if err != nil {
		return task, err
	}
	if task.OrderID == "" {
		return task, nil
	}
	// upsertErr 保存自动化准备阶段订单事实写入结果；失败时禁止继续执行外部动作。
	if err := c.store.Orders.Upsert(ctx, task.OrderID, db.OrderUpsertOpts{
		CookieID: task.AccountID,
		ItemID:   task.ItemID,
		BuyerID:  task.BuyerID,
		ChatID:   task.ChatID,
	}); err != nil {
		return task, fmt.Errorf("保存自动化准备阶段订单事实: %w", err)
	}
	// needsDetail 用于本次流程后续判断的needsDetail
	needsDetail := task.TriggerType == TriggerOrderPaid
	if // existing、err 用于本次流程后续判断的existing、err
	existing, err := c.store.Orders.Get(ctx, task.OrderID); err == nil && existing != nil {
		task = mergeOrderIntoTask(task, existing)
		if needsDetail && (existing.Quantity == "" || existing.Amount == "") {
			needsDetail = true
		}
		// 规则是否多规格由 action.config_json 决定；这里无法提前知道命中的 action，
		// 因此交易类事件统一补齐规格，确保后续规格映射有事实依据。
		if needsDetail && (existing.SpecName == "" || existing.SpecValue == "") {
			needsDetail = true
		}
	}
	// fetcher 是构造期固定的订单详情查询器；执行过程中不允许替换依赖。
	fetcher := c.dependencies.fetcher
	if !needsDetail || fetcher == nil {
		return task, nil
	}
	// cookieStr 用于本次流程后续判断的登录凭证Str
	cookieStr := task.CookieStr
	if strings.TrimSpace(cookieStr) == "" {
		// err 用于本次流程后续判断的err
		var err error
		cookieStr, err = c.cookieValue(ctx, task.AccountID)
		if err != nil {
			return task, err
		}
	}
	// detail、err 用于本次流程后续判断的detail、err
	detail, err := fetcher.FetchOrderDetail(ctx, task.AccountID, task.OrderID, task.ItemID, task.BuyerID, cookieStr)
	if err != nil {
		return task, err
	}
	if detail == nil {
		return task, nil
	}
	if detail.Quantity != "" {
		task.Quantity = detail.Quantity
	}
	if detail.SpecName != "" {
		task.SpecName = detail.SpecName
	}
	if detail.SpecValue != "" {
		task.SpecValue = detail.SpecValue
	}
	if detail.Amount != "" {
		task.Amount = detail.Amount
	}
	if detail.OrderStatus != "" {
		task.OrderStatus = detail.OrderStatus
	}
	// upsertErr 保存补齐订单详情后的事实写入结果，失败时不允许进入动作执行阶段。
	if err := c.store.Orders.Upsert(ctx, task.OrderID, db.OrderUpsertOpts{
		CookieID:    task.AccountID,
		ItemID:      task.ItemID,
		BuyerID:     task.BuyerID,
		ChatID:      task.ChatID,
		SpecName:    task.SpecName,
		SpecValue:   task.SpecValue,
		Quantity:    task.Quantity,
		Amount:      task.Amount,
		OrderStatus: task.OrderStatus,
	}); err != nil {
		return task, fmt.Errorf("保存订单详情事实: %w", err)
	}
	return task, nil
}

// executeAction 将具体动作委托给发货动作执行器。
func (c *Center) executeAction(ctx context.Context, task Task, action db.AutomationAction) (int, error) {
	return c.actions.executeAction(ctx, task, action)
}

// executeActionWithProof 执行动作并把当前运行内已成功投递的凭证传给确认发货动作。
func (c *Center) executeActionWithProof(ctx context.Context, task Task, action db.AutomationAction, proof shipmentDeliveryProof) (actionExecutionResult, error) {
	return c.actions.executeActionWithProof(ctx, task, action, proof)
}

// confirmShipment 将确认发货委托给发货动作执行器。
func (c *Center) confirmShipment(ctx context.Context, task Task) error {
	return c.actions.confirmShipment(ctx, task)
}

// wakeCredentialBlockedAutomation 在 Cookie 更新后唤醒凭证阻塞的自动化任务。
func (c *Center) wakeCredentialBlockedAutomation(ctx context.Context, accountID string) {
	if c == nil || c.store == nil || c.store.Automation == nil {
		return
	}
	if // err 用于本次流程后续判断的err
	err := c.store.Automation.WakeCredentialBlocked(ctx, accountID); err != nil {
		c.logger.Warn("Cookie 更新后唤醒自动化任务失败", "account", accountID, "err", err)
	}
}

// sendCard 将卡密发送委托给发货动作执行器。
func (c *Center) sendCard(ctx context.Context, task Task, action db.AutomationAction) (int, error) {
	return c.actions.sendCard(ctx, task, action)
}

// accountAutomationAllowed 判断账号是否仍允许执行自动化动作。
func (c *Center) accountAutomationAllowed(ctx context.Context, accountID string) (bool, error) {
	return c.taskRunner.accountAutomationAllowed(ctx, accountID)
}

// accountSenderReady 判断账号是否具备可发送自动化消息的在线连接。
func (c *Center) accountSenderReady(accountID string) bool {
	if c == nil || c.senders == nil {
		return false
	}
	// sender、ok 用于本次流程后续判断的sender、ok
	sender, ok := c.senders.Sender(accountID)
	if !ok {
		return false
	}
	if // ready、ok 用于本次流程后续判断的ready、ok
	ready, ok := sender.(automationReadySender); ok {
		return ready.AutomationReady()
	}
	return true
}

// cardContent 获取卡密组内容的兼容入口。
func (c *Center) cardContent(ctx context.Context, card *db.CardFull) (text, imageURL string, err error) {
	return c.actions.cardContent(ctx, card)
}

// sendImage 将图片消息发送委托给发货动作执行器。
func (c *Center) sendImage(ctx context.Context, task Task, imageURL string, cardID int64) error {
	return c.actions.sendImage(ctx, task, imageURL, cardID)
}

// cookieValue 读取账号 Cookie 的兼容入口。
func (c *Center) cookieValue(ctx context.Context, cookieID string) (string, error) {
	return c.actions.cookieValue(ctx, cookieID)
}

// buildTriggerKey 封装buildTriggerKey业务协调。
func buildTriggerKey(task Task) string {
	if task.TriggerType == TriggerReviewMissingTimeout && task.OrderID != "" {
		if // attempt、ok 用于本次流程后续判断的attempt、ok
		attempt, ok := task.Raw["attempt"]; ok {
			return fmt.Sprintf("%s:%s:%v", task.TriggerType, task.OrderID, attempt)
		}
	}
	if task.OrderID != "" {
		return task.TriggerType + ":" + task.OrderID
	}
	if task.UpdateKey != "" {
		return task.TriggerType + ":" + task.UpdateKey
	}
	return ""
}
