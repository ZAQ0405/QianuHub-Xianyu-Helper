package mtop

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ReshelfItem 读取原商品编辑详情，再以原 itemId 提交编辑，从而重新上架同一个商品。
func (c *ClientImpl) ReshelfItem(ctx context.Context, cookiesStr, itemID string) (*AccountTaskResult, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("商品 ID 不能为空")
	}
	// publishReferer 对齐闲鱼网页端从发布页进入原商品编辑的请求上下文。
	publishReferer := "https://www.goofish.com/publish?itemId=" + url.QueryEscape(itemID)
	// detail、updated、err 保存编辑详情响应、会话 Cookie 和请求错误。
	detail, updated, err := c.accountTaskRequest(ctx, cookiesStr, firstNonEmptyURL(c.ItemEditDetailURL, ItemEditDetailAPI),
		"mtop.idle.pc.idleitem.editDetail", "1.0", map[string]any{"itemId": itemID}, publishReferer)
	if err != nil {
		return nil, err
	}
	// payload 保存从原商品编辑详情提取并补齐的同商品编辑载荷。
	payload := buildReshelfPayload(detail.Data, itemID)
	// edit、editUpdated、editErr 保存同商品编辑提交响应、最终 Cookie 和请求错误。
	edit, editUpdated, editErr := c.accountTaskRequest(ctx, updated, firstNonEmptyURL(c.ItemEditURL, ItemEditAPI),
		"mtop.idle.pc.idleitem.edit", "1.0", payload, publishReferer)
	if editErr != nil {
		// verified、verifyErr 用于确认平台虽返回业务异常但原商品实际上已经进入在售列表的最终状态。
		verified, verifyErr := c.itemIsActiveAfterReshelf(ctx, firstNonEmptyCookie(editUpdated, updated), itemID)
		if verifyErr == nil && verified {
			return &AccountTaskResult{Success: true, Message: "原商品已上架，已按在售状态确认成功", UpdatedCookies: firstNonEmptyCookie(editUpdated, updated)}, nil
		}
		if verifyErr != nil {
			return nil, fmt.Errorf("%w；核验商品最终上架状态失败: %v", editErr, verifyErr)
		}
		return nil, editErr
	}
	return &AccountTaskResult{Success: true, Message: firstRet(edit.Ret), UpdatedCookies: firstNonEmptyCookie(editUpdated, updated)}, nil
}

// itemIsActiveAfterReshelf 通过实时在售列表确认原商品是否已经完成重新上架。
func (c *ClientImpl) itemIsActiveAfterReshelf(ctx context.Context, cookiesStr, itemID string) (bool, error) {
	// result 保存平台返回的当前在售商品全集。
	result, err := c.FetchAllItems(ctx, cookiesStr, 20, 0)
	if err != nil {
		return false, err
	}
	for /* item 表示当前在售商品。 */ _, item := range result.Items {
		if strings.TrimSpace(item.ID) == itemID {
			return true, nil
		}
	}
	return false, nil
}

// buildReshelfPayload 只复制编辑详情中平台需要的商品字段，并强制保留原商品 ID。
func buildReshelfPayload(detail map[string]any, itemID string) map[string]any {
	// payload 保存补齐后的同商品编辑载荷。
	payload := make(map[string]any)
	// scalarKeys、objectKeys、listKeys 分别表示编辑详情中的标量、对象和数组字段白名单。
	scalarKeys := []string{"attribute_biz_line", "bizcode", "bucketId", "canBargain", "defaultPrice", "errorTipsMsg", "freebies", "itemStatus", "itemTypeStr", "quantity", "scene", "simpleItem", "supportBargainPrice", "topics"}
	// objectKeys 保存编辑详情中需要原样保留的嵌套对象字段。
	objectKeys := []string{"asyncSecurityInfo", "baseParams", "itemAddrDTO", "itemCatDTO", "itemGroupDTO", "itemPostFeeDTO", "itemPriceDTO", "itemTextDTO", "itemTopicParams", "yhbItemInfoDTO"}
	// listKeys 保存编辑详情中需要原样保留的列表字段。
	listKeys := []string{"imageInfoDOList", "itemLabelExtList", "itemProperties", "itemSkuList", "userRightsProtocols"}
	for /* key 表示当前白名单字段。 */ _, key := range append(append(scalarKeys, objectKeys...), listKeys...) {
		if /* value、ok 保存字段值及其存在标记。 */ value, ok := detail[key]; ok {
			payload[key] = value
		}
	}
	// itemId、sourceId 绑定到原商品，避免平台按新商品发布处理。
	payload["itemId"] = itemID
	payload["sourceId"] = firstNonEmptyCookie(mtopString(detail["sourceId"]), itemID)
	payload["uniqueCode"] = strconv.FormatInt(nowUnixMicro(), 10)
	payload["bizcode"] = firstNonEmptyCookie(mtopString(payload["bizcode"]), "pcMainPublish")
	payload["publishScene"] = firstNonEmptyCookie(mtopString(detail["publishScene"]), "pcMainPublish")
	payload["quantity"] = firstNonEmptyCookie(mtopString(payload["quantity"]), "1")
	if /* ok 表示场景字段是否已由详情提供。 */ _, ok := payload["scene"]; !ok {
		payload["scene"] = ""
	}
	if /* ok 表示商品类型字段是否已由详情提供。 */ _, ok := payload["itemTypeStr"]; !ok {
		payload["itemTypeStr"] = "b"
	}
	if /* ok 表示分组字段是否已由详情提供。 */ _, ok := payload["itemGroupDTO"]; !ok {
		payload["itemGroupDTO"] = map[string]any{"groupId": ""}
	}
	if /* ok 表示话题字段是否已由详情提供。 */ _, ok := payload["topics"]; !ok {
		payload["topics"] = []any{}
	}
	if /* ok 表示风控字段是否已由详情提供。 */ _, ok := payload["asyncSecurityInfo"]; !ok {
		payload["asyncSecurityInfo"] = map[string]any{"securityStrategyHitResult": map[string]any{"FORBIDDEN": []any{}, "WARN": []any{}}}
	}
	normalizeReshelfPayload(payload)
	return payload
}

// normalizeReshelfPayload 兼容编辑详情中字符串布尔值和 JSON 布尔值的混用格式。
func normalizeReshelfPayload(payload map[string]any) {
	// postFee 保存商品运费配置对象。
	if /* postFee、ok 保存运费对象及其类型标记。 */ postFee, ok := payload["itemPostFeeDTO"].(map[string]any); ok {
		for /* key 表示当前运费布尔字段。 */ _, key := range []string{"canFreeShipping", "supportFreight", "onlyTakeSelf"} {
			if /* value、exists 保存字段值及其存在标记。 */ value, exists := postFee[key]; exists {
				postFee[key] = normalizeReshelfBool(value)
			}
		}
	}
	// protocols 保存平台权益协议列表。
	if /* protocols、ok 保存权益协议列表及其类型标记。 */ protocols, ok := payload["userRightsProtocols"].([]any); ok {
		for /* raw 表示当前权益协议。 */ _, raw := range protocols {
			if /* protocol、ok 保存协议对象及其类型标记。 */ protocol, ok := raw.(map[string]any); ok {
				if /* value、exists 保存启用字段及其存在标记。 */ value, exists := protocol["enable"]; exists {
					protocol["enable"] = normalizeReshelfBool(value)
				}
			}
		}
	}
}

// normalizeReshelfBool 将编辑详情中的多种布尔表示统一成 JSON 布尔值。
func normalizeReshelfBool(value any) bool {
	if /* text、ok 保存字符串值及其类型标记。 */ text, ok := value.(string); ok {
		return strings.EqualFold(strings.TrimSpace(text), "true")
	}
	if /* number、ok 保存数字值及其类型标记。 */ number, ok := value.(float64); ok {
		return number != 0
	}
	return value == true
}

// firstNonEmptyCookie 选择第一个非空平台字段，避免与通用文本业务函数混淆。
func firstNonEmptyCookie(values ...string) string {
	for /* value 表示当前待选择的平台字段。 */ _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// nowUnixMicro 返回当前微秒时间，作为编辑请求的幂等追踪值。
func nowUnixMicro() int64 { return time.Now().UnixMicro() }
