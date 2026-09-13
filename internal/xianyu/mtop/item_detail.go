// Package mtop: 商品详情域 — 补充商品列表未返回的多规格信息。
package mtop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xianyu-go/internal/xianyu/protocol"
)

// ItemDetailFetcher 是商品同步使用的可选详情能力。
type ItemDetailFetcher interface {
	DetectItemMultiSpec(ctx context.Context, cookies, itemID string) (bool, error)
}

var _ ItemDetailFetcher = (*ClientImpl)(nil)

// DetectItemMultiSpec 查询 itemID 的多规格事实；ctx 控制取消，cookies 不得写入日志，错误沿用平台分类。
func (c *ClientImpl) DetectItemMultiSpec(ctx context.Context, cookies, itemID string) (bool, error) {
	// detail、err 保存经过 Token 重试的商品详情和请求错误。
	detail, err := c.fetchItemDetail(ctx, cookies, itemID)
	return detectItemMultiSpec(detail), err
}

// FetchItemPublisher 读取 itemID 商品发布人的平台用户标识；ctx 控制取消，cookies 仅用于请求认证。
// 只接受商品详情的卖家对象，绝不把买家、消息发送者或推荐商品的用户标识作为发布人；字段缺失返回错误。
func (c *ClientImpl) FetchItemPublisher(ctx context.Context, cookies, itemID string) (string, error) {
	// detail、err 保存平台详情及错误；失败时不能推断当前账号的交易身份。
	detail, err := c.fetchItemDetail(ctx, cookies, itemID)
	if err != nil {
		return "", err
	}
	// seller 是该商品详情的发布人对象，不递归搜索其他用户。
	seller, _ := detail["sellerDO"].(map[string]any)
	// publisher 是平台详情提供的非敏感用户标识。
	publisher := strings.TrimSpace(mtopString(seller["sellerId"]))
	if publisher == "" {
		return "", errors.New("商品详情缺少发布人身份")
	}
	return publisher, nil
}

// fetchItemDetail 使用 ctx 的取消预算读取 itemID 商品详情，cookies 仅用于签名；返回平台事实或可分类错误。
// 当 ctx 携带 CookieSession 时会像浏览器一样吸收响应 Cookie。
func (c *ClientImpl) fetchItemDetail(ctx context.Context, cookies, itemID string) (map[string]any, error) {
	// currentCookies 保存本轮商品详情请求实际使用的 Cookie，不向日志或错误输出。
	currentCookies := cookies
	if // session 用于吸收没有显式会话调用方收到的 Set-Cookie。
	session := cookieSessionFromContext(ctx); session != nil {
		currentCookies, _, _ = session.State()
	} else {
		ctx, _ = WithFlatCookieSession(ctx, currentCookies)
	}
	for // attempt 表示含首次请求在内的 Token 重试序号。
	attempt := 0; attempt < 4; attempt++ {
		// previousCookies 记录本次请求前的 Cookie，用于优先采用响应下发的 Token。
		previousCookies := currentCookies
		// detail、err 保存一次商品详情请求的事实对象及失败原因。
		detail, err := c.fetchItemDetailOnce(ctx, currentCookies, itemID)
		if err == nil {
			return detail, nil
		}
		if !IsMTopTokenExpiredErr(err) {
			return nil, err
		}
		if // session 用于读取响应刚吸收的最新 Cookie。
		session := cookieSessionFromContext(ctx); session != nil {
			currentCookies, _, _ = session.State()
		}
		if attempt == 3 {
			return nil, fmt.Errorf("商品详情接口 Token 重试失败: %w", err)
		}
		if currentCookies == previousCookies {
			// refreshed、refreshErr 保存主动刷新 MTOP 签名 Token 的结果及错误。
			refreshed, refreshErr := c.RefreshTokenContext(ctx, currentCookies)
			if refreshErr != nil {
				return nil, fmt.Errorf("商品详情 Token 过期且刷新失败: %w", refreshErr)
			}
			currentCookies = refreshed.UpdatedCookies
		}
		if // sleepErr 表示两次平台请求之间等待期间的取消错误。
		sleepErr := sleepCtx(ctx, MTopRetryGap); sleepErr != nil {
			return nil, sleepErr
		}
	}
	return nil, errors.New("商品详情 Token 重试失败")
}

// fetchItemDetailOnce 用 ctx 和仅限请求作用域的 cookies 查询 itemID；返回详情数据，失败不返回部分事实。
func (c *ClientImpl) fetchItemDetailOnce(ctx context.Context, cookies, itemID string) (map[string]any, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("item_id 不能为空")
	}
	// endpoint 是商品详情 API，测试可指向本地 HTTP 服务。
	endpoint := c.ItemDetailURL
	if endpoint == "" {
		endpoint = ItemDetailAPI
	}
	// documentURL 是当前商品页面，用于 Cookie 作用域及 Referer。
	documentURL := "https://www.goofish.com/item?id=" + url.QueryEscape(itemID)
	// signingCookies 和 requestCookies 分别是签名与请求所需的明文 Cookie，仅限本次调用且不得输出。
	signingCookies, requestCookies := mtopRequestCookies(ctx, cookies, documentURL, endpoint)
	// token 是从签名 Cookie 提取的敏感签名密钥，不得记录。
	token := protocol.SignToken(signingCookies)
	if token == "" {
		return nil, fmt.Errorf("cookie 缺少 _m_h5_tk，无法获取商品详情")
	}
	// dataVal 是只包含会话商品 ID 的平台请求体。
	dataVal := `{"itemId":` + strconv.Quote(itemID) + `}`
	// timestamp 是平台签名要求的 Unix 毫秒时间。
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	// sign 是本次详情请求的签名，不得暴露到业务日志。
	sign := protocol.GenerateSign(timestamp, token, dataVal)
	// req、err 保存可由 ctx 取消的详情请求及 URL 构造错误。
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?"+buildItemDetailQuery(timestamp, sign), strings.NewReader("data="+url.QueryEscape(dataVal)))
	if err != nil {
		return nil, err
	}
	setCommonHeaders(req, requestCookies)
	req.Header.Set("Origin", "https://www.goofish.com")
	req.Header.Set("Referer", documentURL)

	// hc 是注入的 HTTP 传输，沿用已有超时和出站配置。
	hc := c.httpClient()
	// resp、err 保存响应及网络错误，响应体由本函数关闭。
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("商品详情请求失败: %w", err)
	}
	defer resp.Body.Close()
	absorbMTopResponseCookies(ctx, cookies, resp)
	// raw、err 保存有界读取的平台响应及读取错误，正文不得直接写入日志。
	raw, err := readMTopBody(resp)
	if err != nil {
		return nil, c.mtopResponseFailureWithCause("商品详情接口", resp.StatusCode, nil, "读取响应失败", err)
	}
	// decoded 保存平台 ret 分类和内部详情事实，失败响应不返回部分事实。
	var decoded struct {
		// Ret 是平台业务结果编码。
		Ret []string `json:"ret"`
		// Data 是内部使用的商品详情事实，不能直接暴露为业务 HTTP DTO。
		Data map[string]any `json:"data"`
	}
	if // err 是平台响应 JSON 解析错误，按既有分类脱敏返回。
	err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, c.mtopResponseFailureWithCause("商品详情接口", resp.StatusCode, nil, "JSON 解析失败", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, c.mtopResponseFailure("商品详情接口", resp.StatusCode, decoded.Ret, "HTTP 状态异常")
	}
	if !hasMTopSuccess(decoded.Ret) {
		return nil, c.mtopResponseFailure("商品详情接口", resp.StatusCode, decoded.Ret, "平台 ret 未包含 SUCCESS")
	}
	return decoded.Data, nil
}

// buildItemDetailQuery 封装build商品Detail查询业务协调。
func buildItemDetailQuery(timestamp, sign string) string {
	// values 用于本次流程后续判断的values
	values := url.Values{
		"jsv":           {"2.7.2"},
		"appKey":        {protocol.SignAppKey},
		"t":             {timestamp},
		"sign":          {sign},
		"v":             {"1.0"},
		"type":          {"originaljson"},
		"accountSite":   {"xianyu"},
		"dataType":      {"json"},
		"timeout":       {"20000"},
		"api":           {"mtop.taobao.idle.pc.detail"},
		"sessionOption": {"AutoLoginOnly"},
		"spm_cnt":       {"a21ybx.item.0.0"},
	}
	return values.Encode()
}

// detectItemMultiSpec 封装detect商品MultiSpec业务协调。
func detectItemMultiSpec(value any) bool {
	return detectMultiSpecValue(value, false, 0)
}

// detectMultiSpecValue 封装detectMultiSpec值业务协调。
func detectMultiSpecValue(value any, skuContext bool, depth int) bool {
	if depth > 16 {
		return false
	}
	switch // typed 用于本次流程后续判断的typed
	typed := value.(type) {
	case map[string]any:
		// key、child 表示当前遍历过程中的key、child
		for key, child := range typed {
			// normalized 用于本次流程后续判断的normalized
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			switch normalized {
			case "multisku", "ismultisku", "ismultispec", "multiplesku":
				if mtopBool(child) {
					return true
				}
			case "skulist", "skus":
				if // list、ok 用于本次流程后续判断的list、ok
				list, ok := child.([]any); ok && len(list) > 1 {
					return true
				}
			case "skuprops", "skuproperties", "specprops", "specifications":
				if // list、ok 用于本次流程后续判断的list、ok
				list, ok := child.([]any); ok && len(list) > 0 {
					return true
				}
			case "props", "properties":
				if skuContext {
					if // list、ok 用于本次流程后续判断的list、ok
					list, ok := child.([]any); ok && len(list) > 0 {
						return true
					}
				}
			}
			// nextSKUContext 用于本次流程后续判断的nextSKU上下文
			nextSKUContext := skuContext || normalized == "skudo" || normalized == "skubase" || normalized == "skumodel"
			if detectMultiSpecValue(child, nextSKUContext, depth+1) {
				return true
			}
		}
	case []any:
		// child 表示当前遍历过程中的child
		for _, child := range typed {
			if detectMultiSpecValue(child, skuContext, depth+1) {
				return true
			}
		}
	}
	return false
}
