package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"xianyu-go/internal/db"
)

// ManualFullDelivery 对已存在订单执行完整发货，复用付款后发货规则但隔离人工与自动运行的幂等键。
func (c *Center) ManualFullDelivery(ctx context.Context, order *db.Order) (int, error) {
	// task、taskErr 保存校验、补全后的人工订单发货任务及其失败原因。
	task, taskErr := c.prepareManualDeliveryTask(ctx, order)
	if taskErr != nil {
		return 0, taskErr
	}
	// manualTriggerKey 与自动付款事件使用不同的幂等键，避免闲鱼官方已确认的空自动运行阻断补发。
	manualTriggerKey := buildManualDeliveryTriggerKey(task)
	// handled、sent、priorErr 分别表示历史运行是否已处理请求、已补发数量及处理失败原因。
	handled, sent, priorErr := c.handlePriorManualDelivery(ctx, task, manualTriggerKey)
	if priorErr != nil || handled {
		return sent, priorErr
	}
	return c.executeManualDeliveryRules(ctx, task, manualTriggerKey)
}

// prepareManualDeliveryTask 校验人工发货输入、账号状态并补齐订单详情，返回可执行的付款发货任务。
func (c *Center) prepareManualDeliveryTask(ctx context.Context, order *db.Order) (Task, error) {
	if c == nil || c.store == nil || order == nil {
		return Task{}, fmt.Errorf("自动化中心未初始化或订单为空")
	}
	if strings.TrimSpace(order.OrderID) == "" {
		return Task{}, fmt.Errorf("订单缺少订单ID")
	}
	// paused、until、pauseErr 保存账号暂停状态、恢复时间和查询错误。
	paused, until, pauseErr := c.store.Cookies.IsPaused(ctx, order.CookieID)
	if pauseErr != nil {
		return Task{}, fmt.Errorf("读取账号暂停状态: %w", pauseErr)
	}
	if paused {
		return Task{}, fmt.Errorf("账号暂停处理中，恢复时间 %d", until)
	}
	// enabled、statusErr 保存账号启用状态和查询错误，停用账号不得消耗卡密。
	enabled, statusErr := c.store.Cookies.Status(ctx, order.CookieID)
	if statusErr != nil {
		return Task{}, fmt.Errorf("读取账号启用状态: %w", statusErr)
	}
	if !enabled {
		return Task{}, fmt.Errorf("账号已停用，无法执行完整发货")
	}
	if strings.TrimSpace(order.CookieID) == "" {
		return Task{}, fmt.Errorf("订单缺少账号ID")
	}
	if strings.TrimSpace(order.ItemID) == "" {
		return Task{}, fmt.Errorf("订单缺少商品ID，无法匹配自动化规则")
	}
	if strings.TrimSpace(order.ChatID) == "" || strings.TrimSpace(order.BuyerID) == "" {
		return Task{}, fmt.Errorf("订单缺少 chat_id 或 buyer_id，无法发送卡券")
	}
	// task 保存由订单事实构成、强制确认闲鱼发货状态的人工付款发货任务。
	task := Task{
		Source:               "manual",
		AccountID:            order.CookieID,
		TriggerType:          TriggerOrderPaid,
		ChatID:               order.ChatID,
		OrderID:              order.OrderID,
		ItemID:               order.ItemID,
		BuyerID:              order.BuyerID,
		SpecName:             order.SpecName,
		SpecValue:            order.SpecValue,
		Quantity:             order.Quantity,
		Amount:               order.Amount,
		OrderStatus:          order.OrderStatus,
		ForceConfirmShipment: true,
		Raw:                  map[string]any{"manual": true},
	}
	// preparedTask、prepareErr 保存订单详情补全后的任务和失败原因。
	preparedTask, prepareErr := c.prepareTask(ctx, task)
	if prepareErr != nil {
		return Task{}, prepareErr
	}
	return preparedTask, nil
}

// handlePriorManualDelivery 根据同订单最近运行决定继续全新发货、原样补发或安全拒绝。
// handled 为 true 时 sent 和 err 已是本次人工请求的最终结果。
func (c *Center) handlePriorManualDelivery(ctx context.Context, task Task, manualTriggerKey string) (handled bool, sent int, err error) {
	// priorRun、priorErr 保存同订单既有自动或人工付款发货运行及读取失败原因。
	priorRun, priorErr := c.store.Automation.GetLatestOrderDeliveryRun(ctx, task.AccountID, task.OrderID)
	if priorErr != nil {
		if errors.Is(priorErr, db.ErrNotFound) {
			return false, 0, nil
		}
		return true, 0, priorErr
	}
	if priorRun.TriggerKey == manualTriggerKey {
		return c.handleExistingManualDelivery(ctx, task, priorRun)
	}
	if priorRun.Status == "running" {
		return true, 0, fmt.Errorf("该订单的自动发货仍在执行，请等待其结束后再人工补发")
	}
	if strings.HasPrefix(priorRun.ErrorMessage, db.NoRetryErrorPrefix) {
		return true, 0, fmt.Errorf("该订单此前向卡密接口请求的结果不确定，已停止自动补发以避免重复扣费，请先在自动化异常中核对")
	}
	if deliveryProofPresent(priorRun.DeliveryProof) {
		if priorRun.Status == "failed" || priorRun.Status == "needs_review" {
			// sent、replayErr 保存快照补发数量和补发失败原因，失败后运行仍保留原快照。
			sent, replayErr := c.replayDeliveryProof(ctx, task, priorRun)
			return true, sent, replayErr
		}
		if priorRun.Status == "success" {
			return true, 0, fmt.Errorf("该订单已存在成功的发货内容快照；如闲鱼状态未同步，请选择仅修改闲鱼发货状态")
		}
	}
	if priorRun.Status == "success" {
		// v1.0.10 会在仅完成 WebSocket 写入、尚未验证自身回显时记为成功，并在确认发货后清空快照。
		// 此类无快照历史记录不能证明买家内容已送达，允许独立的人工运行重新发货；新版本成功运行均保留快照。
		return false, 0, nil
	}
	if priorRun.Status == "needs_review" || priorRun.SentCount > 0 {
		return true, 0, fmt.Errorf("该订单此前发货结果不确定且没有可安全重发的内容快照，请先在自动化异常中核对")
	}
	return false, 0, nil
}

// handleExistingManualDelivery 处理同一人工幂等键的运行，禁止已完成或运行中的请求再次领卡。
func (c *Center) handleExistingManualDelivery(ctx context.Context, task Task, run *db.AutomationRun) (handled bool, sent int, err error) {
	if run.Status == "running" {
		return true, 0, fmt.Errorf("该订单的人工完整发货正在执行，请勿重复提交")
	}
	if run.Status == "failed" || run.Status == "needs_review" {
		if deliveryProofPresent(run.DeliveryProof) {
			// sent、replayErr 保存人工运行快照的补发数量和失败原因。
			sent, replayErr := c.replayDeliveryProof(ctx, task, run)
			return true, sent, replayErr
		}
		if run.Status == "failed" && run.SentCount == 0 {
			return true, 0, fmt.Errorf("上次人工发货尚未发送任何内容，正在等待安全重试；请稍后重新发起完整发货")
		}
		return true, 0, fmt.Errorf("上次人工发货结果不确定且没有可安全重发的内容快照，请先在自动化异常中核对")
	}
	return true, 0, fmt.Errorf("该订单已完成完整发货；如仅需补记闲鱼状态，请选择仅修改闲鱼发货状态")
}

// executeManualDeliveryRules 匹配订单付款规则并执行第一个可发卡规则，未命中时返回明确错误。
func (c *Center) executeManualDeliveryRules(ctx context.Context, task Task, manualTriggerKey string) (int, error) {
	// rules、matchErr 保存与订单匹配的付款发货规则和查询错误。
	rules, matchErr := c.rules.match(ctx, task)
	if matchErr != nil {
		return 0, matchErr
	}
	if len(rules) == 0 {
		return 0, fmt.Errorf("未匹配到付款后自动发货规则")
	}
	// rule 保存当前候选付款发货规则。
	for _, rule := range rules {
		if !c.planner.hasMatchingSendCard(task, rule.Actions) {
			continue
		}
		// sent、executeErr 保存当前规则的发货数量和执行失败原因，首个有效规则即结束人工流程。
		sent, executeErr := c.executeManualDeliveryRule(ctx, task, rule, manualTriggerKey)
		if executeErr != nil || sent > 0 {
			return sent, executeErr
		}
	}
	return 0, fmt.Errorf("未匹配到订单规格对应的卡密动作")
}

// executeManualDeliveryRule 为一个已匹配的规则创建人工运行、执行即时动作并原子收口运行状态。
func (c *Center) executeManualDeliveryRule(ctx context.Context, task Task, rule db.AutomationRule, manualTriggerKey string) (int, error) {
	// plannedTask 保存仅含人工即时动作的任务副本，延迟动作不得进入人工发货流程。
	plannedTask := task
	plannedTask.ActionPlan = c.planner.plan(task, c.planner.immediateManualActions(rule.Actions))
	// rawTask、rawJSON、marshalErr 分别保存脱敏任务快照、其 JSON 与序列化失败原因。
	rawTask := plannedTask
	rawTask.CookieStr = ""
	// rawJSON、marshalErr 保存脱敏任务序列化结果及失败原因，失败时不能创建可执行运行。
	rawJSON, marshalErr := json.Marshal(rawTask)
	if marshalErr != nil {
		return 0, fmt.Errorf("保存完整发货运行快照: %w", marshalErr)
	}
	// runID、started、startErr 保存创建或占用人工运行的结果。
	runID, started, startErr := c.store.Automation.TryStartRun(ctx, db.AutomationRun{
		RuleID: rule.ID, CookieID: plannedTask.AccountID, ItemID: plannedTask.ItemID, OrderID: plannedTask.OrderID,
		BuyerID: plannedTask.BuyerID, ChatID: plannedTask.ChatID, TriggerType: TriggerOrderPaid, TriggerKey: manualTriggerKey,
		RawEventJSON: string(rawJSON), LeaseExpiresAt: time.Now().UTC().Add(5 * time.Minute).Unix(),
	})
	if startErr != nil {
		return 0, startErr
	}
	if !started {
		// existingRun、existingErr 保存本规则的人工幂等运行及其读取失败原因。
		existingRun, existingErr := c.store.Automation.GetRunByRuleAndTrigger(ctx, rule.ID, manualTriggerKey)
		if existingErr != nil {
			return 0, existingErr
		}
		// sent、handlingErr 保存已有人工运行的快照补发数量和处理失败原因。
		_, sent, handlingErr := c.handleExistingManualDelivery(ctx, plannedTask, existingRun)
		return sent, handlingErr
	}
	// run、runErr 保存刚创建运行的可比较代次及读取失败原因。
	run, runErr := c.store.Automation.GetRun(ctx, runID)
	if runErr != nil {
		return 0, runErr
	}
	// sent、deferred、executeErr 保存运行执行的发货数量、延迟标志和动作失败原因。
	sent, deferred, executeErr := c.executeRunActions(ctx, plannedTask, rule.ID, run, plannedTask.ActionPlan, true)
	if deferred {
		return sent, errors.New("手动完整发货不应进入延迟队列")
	}
	if errors.Is(executeErr, errAutomationNeedsReview) {
		return sent, executeErr
	}
	return sent, c.finishManualDeliveryRun(runID, run.AttemptCount, sent, executeErr)
}

// finishManualDeliveryRun 将人工运行收口为成功或失败；状态写入失败时隔离运行，避免重放外部动作。
func (c *Center) finishManualDeliveryRun(runID int64, attempt int, sent int, executeErr error) error {
	// status、errMsg 保存需要落库的终态及对用户可见的失败原因。
	status, errMsg := "success", ""
	if executeErr != nil {
		status, errMsg = "failed", executeErr.Error()
	}
	// finishCtx、cancel 为结果收口分配独立短时上下文，调用者请求取消不能中断安全收尾。
	finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// finishErr 保存写入运行终态失败原因。
	finishErr := c.store.Automation.FinishRun(finishCtx, runID, attempt, status, sent, errMsg)
	if finishErr == nil {
		return executeErr
	}
	// reason 说明外部动作已经可能执行，但运行结果未能收口，必须禁止自动重放并转人工核对。
	reason := "完整发货外部动作可能已执行，但运行结果保存失败，已停止自动重放，请人工核对: " + finishErr.Error()
	// quarantineErr 保存把手动运行转为人工核对状态时的失败原因。
	quarantineErr := c.store.Automation.QuarantineRunResult(finishCtx, runID, attempt, sent, reason)
	if quarantineErr != nil {
		return errors.Join(errAutomationNeedsReview, fmt.Errorf("保存完整发货执行结果: %w", finishErr), fmt.Errorf("保存完整发货人工核对状态: %w", quarantineErr))
	}
	return errors.Join(errAutomationNeedsReview, fmt.Errorf("保存完整发货执行结果: %w", finishErr))
}

// buildManualDeliveryTriggerKey 为人工完整发货生成独立且稳定的幂等键。
func buildManualDeliveryTriggerKey(task Task) string {
	if task.OrderID == "" {
		return ""
	}
	return "manual_delivery:" + task.OrderID
}

// deliveryProofPresent 判断加密快照解密后是否至少包含一条可重发内容，兼容旧版文本和图片字段。
func deliveryProofPresent(proof db.AutomationDeliveryProof) bool {
	return len(proof.Messages) > 0 || strings.TrimSpace(proof.TradeText) != "" || len(proof.PicList) > 0
}

// replayDeliveryProof 把失败运行已持久化的订单内容按原始顺序重新发送，并再次确认闲鱼发货状态。
func (c *Center) replayDeliveryProof(ctx context.Context, task Task, run *db.AutomationRun) (int, error) {
	if run == nil || !deliveryProofPresent(run.DeliveryProof) {
		return 0, fmt.Errorf("订单没有可重发的发货内容快照")
	}
	// allowed、allowedErr 保存账号自动化门禁结果，账号暂停或停用时不能发送补发消息。
	allowed, allowedErr := c.accountAutomationAllowed(ctx, task.AccountID)
	if allowedErr != nil {
		return 0, allowedErr
	}
	if !allowed {
		return 0, fmt.Errorf("账号已暂停或停用，无法补发订单内容")
	}
	// messages 保存按原顺序恢复的消息，旧快照没有 Messages 时兼容图片后文本的历史顺序。
	messages := append([]db.AutomationDeliveryMessage(nil), run.DeliveryProof.Messages...)
	if len(messages) == 0 {
		// imageURL 保存旧版快照的一张图片地址；旧版没有跨类型消息顺序，因此仅按历史图片列表恢复。
		for _, imageURL := range run.DeliveryProof.PicList {
			messages = append(messages, db.AutomationDeliveryMessage{Kind: "image", Content: imageURL})
		}
		if strings.TrimSpace(run.DeliveryProof.TradeText) != "" {
			messages = append(messages, db.AutomationDeliveryMessage{Kind: "text", Content: run.DeliveryProof.TradeText})
		}
	}
	// messageIndex、message 分别表示当前重发消息的顺序和内容，失败时保留同一快照供下次重发。
	for messageIndex, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			return messageIndex, fmt.Errorf("订单发货快照第 %d 条内容为空", messageIndex+1)
		}
		// sendErr 保存当前原样重发消息的发送结果，失败时不得重新领取卡密。
		var sendErr error
		switch message.Kind {
		case "text":
			sendErr = c.actions.sendText(ctx, task, message.Content)
		case "image":
			sendErr = c.actions.sendImage(ctx, task, message.Content, 0)
		default:
			return messageIndex, fmt.Errorf("订单发货快照第 %d 条消息类型无效", messageIndex+1)
		}
		if sendErr != nil {
			return messageIndex, fmt.Errorf("原样补发订单内容失败: %w", sendErr)
		}
	}
	// proof 保存确认闲鱼发货接口所需历史文本和图片凭证，不由重发次数重新拼装。
	proof := shipmentDeliveryProof{tradeText: run.DeliveryProof.TradeText, picList: append([]string(nil), run.DeliveryProof.PicList...)}
	// confirmErr 保存闲鱼确认发货或本地订单事实写入失败原因，消息已发送时不得重新取卡。
	if confirmErr := c.actions.confirmShipmentWithProof(ctx, task, proof); confirmErr != nil {
		return len(messages), confirmErr
	}
	// completeErr 保存补发运行收口失败原因，此时仅允许人工核对，不能重新发送快照。
	if completeErr := c.store.Automation.CompleteDeliveryReplay(ctx, run.ID, run.AttemptCount); completeErr != nil {
		return len(messages), fmt.Errorf("补发内容已发送但保存运行结果失败: %w", completeErr)
	}
	return len(messages), nil
}
