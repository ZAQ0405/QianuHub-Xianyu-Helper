package automation

import (
	"context"
	"testing"

	"xianyu-go/internal/db"
)

// TestConfirmShipmentWithProofCoversPreflightBranches 验证确认发货在订单号缺失、账号关闭自动确认和数据库失败时的前置分支。
func TestConfirmShipmentWithProofCoversPreflightBranches(t *testing.T) {
	// ctx 是本测试确认发货入口共用的非取消上下文。
	ctx := context.Background()
	// emptyStore、emptyCleanup 保存确认发货前置测试数据库及清理函数。
	emptyStore, emptyCleanup := newAutomationTestStore(t)
	defer emptyCleanup()
	// center 是使用本地存储且不执行平台调用的自动化中心。
	center := New(emptyStore, nil, nil)
	// missingOrderErr 保存订单号缺失时的前置校验错误。
	missingOrderErr := center.confirmShipment(ctx, Task{AccountID: "cid"})
	if missingOrderErr == nil {
		t.Fatal("缺少订单号应拒绝确认发货")
	}
	// disabledAutoConfirm 保存关闭账号自动确认设置的值。
	disabledAutoConfirm := false
	// _, disableErr 保存关闭账号自动确认设置的更新错误。
	_, disableErr := emptyStore.Cookies.UpdateSettings(ctx, "cid", db.AccountSettingsUpdate{UserID: 1, AutoConfirm: &disabledAutoConfirm})
	if disableErr != nil {
		t.Fatal(disableErr)
	}
	// skipErr 保存非强制确认在账号设置关闭时的跳过结果。
	skipErr := center.confirmShipment(ctx, Task{AccountID: "cid", OrderID: "disabled-auto-confirm"})
	if skipErr != nil {
		t.Fatalf("关闭自动确认时应安全跳过：%v", skipErr)
	}

	// closedStore、closedCleanup 保存随后关闭数据库连接的测试存储。
	closedStore, closedCleanup := newAutomationTestStore(t)
	defer closedCleanup()
	// closeErr 保存关闭测试数据库连接的资源释放错误。
	if closeErr := closedStore.DB.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	// closedCenter 是绑定关闭数据库的自动化中心。
	closedCenter := New(closedStore, nil, nil)
	// readErr 保存读取自动确认设置失败的基础设施错误。
	readErr := closedCenter.confirmShipment(ctx, Task{AccountID: "cid", OrderID: "closed-db"})
	if readErr == nil {
		t.Fatal("关闭数据库后读取自动确认设置应返回错误")
	}
}

// TestConfirmShipmentActionSendsConfiguredMessage 验证确认发货成功后会把规则文案单独发送给买家。
func TestConfirmShipmentActionSendsConfiguredMessage(t *testing.T) {
	// store、cleanup 保存确认发货文案测试使用的数据库及清理函数。
	store, cleanup := newAutomationTestStore(t)
	defer cleanup()
	// ctx 保存本测试共用的数据库上下文。
	ctx := context.Background()
	// mtopMock 记录平台收到的确认发货文本，并返回明确成功结果。
	mtopMock := &fakeMTop{consignOk: true, consignRet: []string{"SUCCESS::调用成功"}}
	// sender 记录确认发货成功后发送给买家的补充文案。
	sender := &testSender{}
	// center 是注入测试 MTOP 客户端和买家消息发送器的自动化中心。
	center := NewWithDependencies(store, testSenderProvider{sender: sender}, nil, CenterDependencies{MTop: mtopMock})
	// result、err 保存确认发货动作执行结果。
	result, err := center.actions.executeActionWithProof(ctx, Task{AccountID: "cid", OrderID: "configured-message-order", ChatID: "chat", BuyerID: "buyer"}, db.AutomationAction{
		ActionType: ActionConfirmShipment, MessageTemplate: "  资料已发送，请查收  ", Enabled: true,
	}, shipmentDeliveryProof{tradeText: "卡密内容"})
	if err != nil || result.sent != 0 {
		t.Fatalf("确认发货动作执行异常: result=%+v err=%v", result, err)
	}
	if mtopMock.consignTradeTextIn != "卡密内容" {
		t.Fatalf("确认发货不应改写卡密凭证: %q", mtopMock.consignTradeTextIn)
	}
	if len(sender.texts) != 1 || sender.texts[0] != "资料已发送，请查收" {
		t.Fatalf("确认发货文案未发送给买家: %+v", sender.texts)
	}
}
