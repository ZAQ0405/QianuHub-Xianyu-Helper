package ws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"xianyu-go/internal/xianyu/protocol"
)

// handleSyncExtra 处理服务端增量同步帧并在需要时回传确认，返回解码或发送错误。
func (c *Conn) handleSyncExtra(ctx context.Context, msg map[string]any) error {
	// body 用于本次流程后续判断的请求体
	body, _ := msg["body"].(map[string]any)
	// extra 用于本次流程后续判断的extra
	extra, _ := body["syncExtraType"].(map[string]any)
	// typeCode、ok 用于本次流程后续判断的类型Code、ok
	typeCode, ok := responseCode(extra["type"])
	if !ok || (typeCode != 1 && typeCode != 2) {
		return nil
	}
	// state、err 用于本次流程后续判断的state、err
	state, err := c.request(ctx, "/r/SyncStatus/getState", map[string]any{}, []any{map[string]any{"topic": "sync"}}, regResponseTimeout)
	if err != nil {
		return fmt.Errorf("getState: %w", err)
	}
	if // code、ok 用于本次流程后续判断的code、ok
	code, ok := responseCode(state["code"]); !ok || code != http.StatusOK || state["body"] == nil {
		return fmt.Errorf("getState 返回异常: code=%v", state["code"])
	}
	// response、err 用于本次流程后续判断的response、err
	response, err := c.request(ctx, "/r/SyncStatus/ackDiff", map[string]any{}, []any{state["body"]}, regResponseTimeout)
	if err != nil {
		return fmt.Errorf("ackDiff: %w", err)
	}
	if // code、ok 用于本次流程后续判断的code、ok
	code, ok := responseCode(response["code"]); ok && code != http.StatusOK {
		return fmt.Errorf("ackDiff 返回异常: code=%d", code)
	}
	return nil
}

// sendACK 回复 {"code":200, headers:<服务端完整 headers>}。
func (c *Conn) sendACK(ctx context.Context, msg map[string]any) {
	// headers 用于本次流程后续判断的headers
	headers, _ := msg["headers"].(map[string]any)
	// ackHeaders 用于本次流程后续判断的ackHeaders
	ackHeaders := make(map[string]any, len(headers))
	// key、value 表示当前遍历过程中的key、value
	for key, value := range headers {
		ackHeaders[key] = value
	}
	// ack 用于本次流程后续判断的ack
	ack := map[string]any{
		"code":    200,
		"headers": ackHeaders,
	}
	// ACK 失败不阻塞主循环。
	ackCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	_ = c.sendJSON(ackCtx, ack)
	cancel()
}

// extractSyncPayload 取出 body.syncPushPackage.data[0].data（字符串）。
func extractSyncPayload(msg map[string]any) (string, bool) {
	// body 用于本次流程后续判断的请求体
	body, _ := msg["body"].(map[string]any)
	if body == nil {
		return "", false
	}
	// pkg 用于本次流程后续判断的pkg
	pkg, _ := body["syncPushPackage"].(map[string]any)
	if pkg == nil {
		return "", false
	}
	// arr 用于本次流程后续判断的arr
	arr, _ := pkg["data"].([]any)
	if len(arr) == 0 {
		return "", false
	}
	// first 用于本次流程后续判断的first
	first, _ := arr[0].(map[string]any)
	if first == nil {
		return "", false
	}
	// d、ok 用于本次流程后续判断的d、ok
	d, ok := first["data"].(string)
	return d, ok && d != ""
}

// decodeSyncData 先尝试 base64+JSON（未加密系统消息），失败则 base64+msgpack 解密。
func decodeSyncData(data string) (map[string]any, error) {
	// 1) base64 解码后尝试解析 JSON。
	if dec, err := base64.StdEncoding.DecodeString(data); err == nil {
		// parsed 用于本次流程后续判断的解析结果
		var parsed map[string]any
		if // jsonErr 用于本次流程后续判断的jsonErr
		jsonErr := json.Unmarshal(dec, &parsed); jsonErr == nil {
			return parsed, nil
		}
	}
	// 2) JSON 解析失败 → msgpack 解密
	out, err := protocol.Decrypt(data)
	if err != nil {
		return nil, err
	}
	// parsed 用于本次流程后续判断的解析结果
	var parsed map[string]any
	if // err 用于本次流程后续判断的err
	err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return nil, fmt.Errorf("解密后非 JSON: %w", err)
	}
	return parsed, nil
}

// sendJSON 发送一条 JSON 文本帧。
func (c *Conn) sendJSON(ctx context.Context, v any) error {
	// b、err 用于本次流程后续判断的b、err
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if // recorder 用于本次流程后续判断的recorder
	recorder := c.recorderSnapshot(); recorder != nil {
		recorder("out", string(b), string(b), "json", "")
	}
	select {
	case c.sendGate <- struct{}{}:
		defer func() { <-c.sendGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	return c.ws.Write(ctx, websocket.MessageText, b)
}

// SendText 发送一条闲鱼聊天文本消息。
func (c *Conn) SendText(ctx context.Context, myID, cid, toID, text string) error {
	// content 用于本次流程后续判断的内容
	content := map[string]any{
		"contentType": 1,
		"text": map[string]any{
			"text": text,
		},
	}
	return c.sendChatContent(ctx, myID, cid, toID, content)
}

// MarkChatRead 将当前会话的 PNM 消息 ID 上报为已读。
// ctx 控制远端请求生命周期；cid 仅用于本地可观测日志；messageIDs 为待上报消息对象。
// 返回值仅报告远端调用失败，平台拒绝会被记录为告警以保留既有调用兼容性。
func (c *Conn) MarkChatRead(ctx context.Context, cid string, messageIDs []map[string]any) error {
	// ids 是剔除空值后的 PNM ID 列表，按平台 MessageStatusService 的参数格式发送。
	ids := make([]string, 0, len(messageIDs))
	// item 为调用方传入的一条待读消息对象，可能缺少平台消息 ID。
	for _, item := range messageIDs {
		// id 是当前对象中可上报的非空 PNM 消息 ID。
		if id := strings.TrimSpace(fmt.Sprint(item["messageId"])); id != "" && id != "<nil>" {
			ids = append(ids, id)
		}
	}
	c.logger.Debug("准备上报闲鱼已读", "cid", cid, "message_count", len(ids), "message_ids", ids)
	// response 保存平台响应；err 表示请求或传输失败。服务只接受一个 string 列表参数。
	response, err := c.request(ctx, "/r/MessageStatus/read", map[string]any{}, []any{ids}, regResponseTimeout)
	if err == nil {
		// code 是平台业务状态码；ok 表示响应中的状态码可被规范解析。
		if code, ok := responseCode(response["code"]); ok && code >= 400 {
			c.logger.Warn("闲鱼已读上报被拒绝", "cid", cid, "message_count", len(ids), "code", code, "body", response["body"])
		} else {
			c.logger.Debug("闲鱼已读上报成功", "cid", cid, "message_count", len(ids), "message_ids", ids, "code", response["code"])
		}
	}
	return err
}

// SendImage 发送一条闲鱼聊天图片消息。imageURL 应为闲鱼可访问的 CDN/公网 URL。
func (c *Conn) SendImage(ctx context.Context, myID, cid, toID, imageURL string, width, height int) error {
	if width <= 0 {
		width = 800
	}
	if height <= 0 {
		height = 600
	}
	// content 用于本次流程后续判断的内容
	content := map[string]any{
		"contentType": 2,
		"image": map[string]any{
			"pics": []map[string]any{{
				"height": height,
				"type":   0,
				"url":    imageURL,
				"width":  width,
			}},
		},
	}
	return c.sendChatContent(ctx, myID, cid, toID, content)
}

// SendItemCard 发送一条个人会话商品卡片，载荷与闲鱼 PC IM contentType=7 协议保持一致。
func (c *Conn) SendItemCard(ctx context.Context, myID, cid, toID, itemID, title, imageURL, price string) error {
	// normalizedItemID 和 normalizedTitle 是去除首尾空白的商品身份字段。
	normalizedItemID, normalizedTitle := strings.TrimSpace(itemID), strings.TrimSpace(title)
	// normalizedImageURL 和 normalizedPrice 是去除首尾空白及已有货币符号的展示字段。
	normalizedImageURL, normalizedPrice := strings.TrimSpace(imageURL), strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(price), "¥"))
	if normalizedItemID == "" || normalizedTitle == "" || normalizedImageURL == "" || normalizedPrice == "" {
		return fmt.Errorf("发送商品卡片缺少必要字段")
	}
	// content 是官网个人会话商品卡片的内层消息正文。
	content := map[string]any{
		"contentType": 7,
		"itemCard": map[string]any{
			"itemTip": "我想要",
			"item":    map[string]any{"itemId": normalizedItemID, "mainPic": normalizedImageURL, "price": "¥" + normalizedPrice, "title": normalizedTitle},
		},
	}
	return c.sendChatContent(ctx, myID, cid, toID, content)
}

// sendChatContent 封装send聊天内容业务协调。
func (c *Conn) sendChatContent(ctx context.Context, myID, cid, toID string, content any) error {
	// err 保存发送开始前的取消状态。
	if err := ctx.Err(); err != nil {
		return &SendError{Kind: SendNotSent, Err: err}
	}
	myID = stripGoofish(myID)
	cid = stripGoofish(cid)
	toID = stripGoofish(toID)
	if myID == "" || cid == "" || toID == "" {
		return &SendError{Kind: SendNotSent, Err: fmt.Errorf("发送消息缺少必要参数")}
	}
	// raw、err 用于本次流程后续判断的raw、err
	raw, err := json.Marshal(content)
	if err != nil {
		return &SendError{Kind: SendNotSent, Err: err}
	}
	// encoded 用于本次流程后续判断的encoded
	encoded := base64.StdEncoding.EncodeToString(raw)
	// headers 保存一次平台发送使用的 mid；请求确认必须使用同一个 mid。
	headers := map[string]any{"mid": protocol.GenerateMid()}
	// body 保存与既有协议完全一致的消息载荷。
	body := []any{
		map[string]any{
			"uuid":             protocol.GenerateUUID(),
			"cid":              cid + "@goofish",
			"conversationType": 1,
			"content": map[string]any{
				"contentType": 101,
				"custom": map[string]any{
					"type": 1,
					"data": encoded,
				},
			},
			"redPointPolicy": 0,
			"extension": map[string]any{
				"extJson": "{}",
			},
			"ctx": map[string]any{
				"appVersion": "1.0",
				"platform":   "web",
			},
			"mtags":                map[string]any{},
			"msgReadStatusSetting": 1,
		},
		map[string]any{
			"actualReceivers": []string{
				toID + "@goofish",
				myID + "@goofish",
			},
		},
	}
	// response 保存平台对本次发送的确认；该请求不会额外创建连接或增加闲鱼调用次数。
	response, err := c.request(ctx, "/r/MessageSend/sendByReceiverScope", headers, body, regResponseTimeout)
	if err != nil {
		return &SendError{Kind: SendUncertain, Err: err}
	}
	// code 和 ok 保存平台发送确认状态码及其严格可解析性。
	code, ok := strictChatSendResponseCode(response["code"])
	if !ok {
		return &SendError{Kind: SendUncertain, Code: code}
	}
	if code == http.StatusOK {
		return nil
	}
	if code >= http.StatusBadRequest && code < http.StatusInternalServerError && code != http.StatusRequestTimeout {
		return &SendError{Kind: SendRejected, Code: code}
	}
	return &SendError{Kind: SendUncertain, Code: code}
}

// strictChatSendResponseCode 只接受完整整数形式的聊天发送状态码，避免截断浮点数或接受带尾随字符的字符串。
func strictChatSendResponseCode(value any) (int, bool) {
	switch // code 是当前待严格校验的平台状态码具体类型和值。
	code := value.(type) {
	case int:
		return code, true
	case float64:
		if math.IsNaN(code) || math.IsInf(code, 0) || code != math.Trunc(code) || code > float64(math.MaxInt) || code < float64(math.MinInt) {
			return 0, false
		}
		return int(code), true
	case json.Number:
		// parsed 和 err 保存不允许小数形式的 JSON 整数及解析错误。
		parsed, err := code.Int64()
		if err != nil || int64(int(parsed)) != parsed {
			return 0, false
		}
		return int(parsed), true
	case string:
		// raw 保存去除协议外围空白后的完整状态码文本。
		raw := strings.TrimSpace(code)
		if raw == "" {
			return 0, false
		}
		// digit 是当前参与严格数字校验的字符，不允许符号、小数点或尾随文本。
		for _, digit := range raw {
			if digit < '0' || digit > '9' {
				return 0, false
			}
		}
		// parsed 和 err 保存完整十进制状态码及溢出错误。
		parsed, err := strconv.Atoi(raw)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// stripGoofish 封装stripGoofish业务协调。
func stripGoofish(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimSuffix(s, "@goofish")
}

// Close 关闭连接。
func (c *Conn) Close() error {
	c.ensureReadPump()
	c.readCancel()
	return c.ws.Close(websocket.StatusNormalClosure, "bye")
}
