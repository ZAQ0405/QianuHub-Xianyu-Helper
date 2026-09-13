package mtop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestFetchItemPublisher 使用本地 HTTP 详情验证发布人字段及错误边界；t 管理服务生命周期。
func TestFetchItemPublisher(t *testing.T) {
	// cases 覆盖实际卖家对象、无效结构、推荐商品及买家字段，避免递归误取无关身份。
	cases := []struct {
		// name 是当前响应场景。
		name string
		// body 是本地模拟的平台响应。
		body string
		// want 是合法发布人标识，空值表示请求必须失败。
		want string
	}{
		{"字符串发布人", `{"ret":["SUCCESS::调用成功"],"data":{"sellerDO":{"sellerId":"123"}}}`, "123"},
		{"数字发布人", `{"ret":["SUCCESS::调用成功"],"data":{"sellerDO":{"sellerId":123}}}`, "123"},
		{"买家不是发布人", `{"ret":["SUCCESS::调用成功"],"data":{"buyerDO":{"sellerId":"123"}}}`, ""},
		{"推荐商品不是会话商品", `{"ret":["SUCCESS::调用成功"],"data":{"recommend":{"sellerDO":{"sellerId":"123"}}}}`, ""},
		{"空详情", `{"ret":["SUCCESS::调用成功"],"data":{}}`, ""},
		{"平台失败", `{"ret":["FAIL_BIZ_ITEM_NOT_FOUND::商品不存在"],"data":{"sellerDO":{"sellerId":"123"}}}`, ""},
		{"格式错误", `broken`, ""},
	}
	// scenario 是当前响应和预期身份组合。
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			// server 返回固定平台详情；w 写入响应，r 供确认客户端使用详情接口。
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("api") != "mtop.taobao.idle.pc.detail" {
					t.Error("未使用商品详情接口")
				}
				fmt.Fprint(w, scenario.body)
			}))
			defer server.Close()
			// client 将详情请求限制到本地 HTTP 服务。
			client := &ClientImpl{HTTPClient: server.Client(), ItemDetailURL: server.URL}
			// publisher、err 保存一次完整的发布人查询结果。
			publisher, err := client.FetchItemPublisher(context.Background(), "_m_h5_tk=test_1", "conversation-item")
			if publisher != scenario.want || (err != nil) != (scenario.want == "") {
				t.Fatalf("发布人结果=%q 错误=%v", publisher, err)
			}
		})
	}
}

// TestPublisherTokenRecovery 验证发布人查询沿用详情接口的 Token 恢复、耗尽和失败语义；t 管理本地 HTTP 生命周期。
func TestPublisherTokenRecovery(t *testing.T) {
	// mode 选择响应下发新 Token、连续过期或主动刷新失败三种确定性场景。
	for _, mode := range []string{"updated", "exhausted", "refresh-failed"} {
		t.Run(mode, func(t *testing.T) {
			// requests 统计跨 HTTP 服务协程的请求次数。
			var requests atomic.Int32
			// server 只在本地模拟详情与 Token 接口；w 输出合成响应，r 用于区分端点。
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// requestNumber 是当前详情或刷新请求的全局顺序。
				requestNumber := requests.Add(1)
				if strings.HasPrefix(r.URL.Path, "/token") {
					fmt.Fprint(w, `{"ret":["FAIL_BIZ_DENIED::本地模拟刷新失败"]}`)
					return
				}
				if mode == "updated" && requestNumber > 1 {
					fmt.Fprint(w, `{"ret":["SUCCESS::调用成功"],"data":{"sellerDO":{"sellerId":"123"}}}`)
					return
				}
				if mode != "refresh-failed" {
					http.SetCookie(w, &http.Cookie{Name: "_m_h5_tk", Value: fmt.Sprintf("synthetic%d_1", requestNumber), Path: "/"})
				}
				fmt.Fprint(w, `{"ret":["FAIL_SYS_TOKEN_EXOIRED::本地模拟签名过期"]}`)
			}))
			defer server.Close()
			// client 将详情与主动 Token 刷新都限制到同一本地服务。
			client := &ClientImpl{HTTPClient: server.Client(), ItemDetailURL: server.URL + "/detail", TokenURL: server.URL + "/token"}
			// ctx 携带已存在的平面 Cookie 会话，覆盖查询复用会话的路径。
			ctx, _ := WithFlatCookieSession(context.Background(), "unb=123; _m_h5_tk=initial_1")
			// publisher、err 是恢复结束后的发布人和错误。
			publisher, err := client.FetchItemPublisher(ctx, "unb=ignored", "item")
			if mode == "updated" {
				if err != nil || publisher != "123" || requests.Load() != 2 {
					t.Fatalf("新 Token 重试失败: publisher=%q err=%v requests=%d", publisher, err, requests.Load())
				}
			} else if err == nil || publisher != "" {
				t.Fatal("恢复失败后不能返回商品发布人")
			}
			if mode == "exhausted" && requests.Load() != 4 {
				t.Fatalf("应在四次详情请求后停止: %d", requests.Load())
			}
		})
	}
}

// TestItemSyncTotalConsistency 验证准确总数允许完整同步，分页途中总数变化则拒绝提交；t 管理场景。
func TestItemSyncTotalConsistency(t *testing.T) {
	// changes 表示平台总数是否在第二页发生变化。
	for _, changes := range []bool{false, true} {
		// pages 统计本地平台返回的页号，避免重复 ID 遮蔽总数校验。
		var pages atomic.Int32
		// server 按序返回两页商品，w 输出带准确总数的响应。
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// page 是本次返回页号；total 是这次响应宣告的总条数。
			page, total := pages.Add(1), 2
			if changes && page == 2 {
				total = 3
			}
			fmt.Fprintf(w, `{"ret":["SUCCESS::调用成功"],"data":{"totalCount":%d,"pageCount":2,"cardList":[{"cardData":{"id":"item-%d"}}]}}`, total, page)
		}))
		// client 只连接当前本地列表服务。
		client := &ClientImpl{HTTPClient: &http.Client{Transport: &rewriteTransport{base: server.Client().Transport, target: server.URL}}}
		// result、err 是完整性校验后的全集结果。
		result, err := client.FetchAllItems(context.Background(), consignCookies, 1, 3)
		server.Close()
		if changes {
			if err == nil || result != nil {
				t.Fatal("总数变化仍返回了可同步全集")
			}
		} else if err != nil || result == nil || len(result.Items) != 2 {
			t.Fatalf("完整分页应成功: %v", err)
		}
	}
}

// TestItemSyncRejectsMalformedPages 验证异常或不完整页不能变成远端全集，导致本地商品软删除；t 管理本地服务。
func TestItemSyncRejectsMalformedPages(t *testing.T) {
	// body 是不能安全参与全量同步的页面内容。
	for _, body := range []string{`{}`, `{"cardList":null}`, `{"cardList":[null]}`, `{"cardList":[{}]}`, `{"cardList":[{"cardData":{}}]}`, `{"cardList":[],"pageCount":3}`, `{"cardList":[],"totalCount":1}`, `{"cardList":[{"cardData":{"id":"item"}}],"pageCount":1,"totalCount":2}`} {
		// server 按 body 返回平台成功状态下的异常数据，w 写入响应。
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"ret":["SUCCESS::调用成功"],"data":%s}`, body)
		}))
		// client 将固定商品 API 地址改写到本地服务，不请求真实平台。
		client := &ClientImpl{HTTPClient: &http.Client{Transport: &rewriteTransport{base: server.Client().Transport, target: server.URL}}}
		// result、err 是全量同步结果，失败时必须没有可提交的部分列表。
		result, err := client.FetchAllItems(context.Background(), consignCookies, 20, 3)
		server.Close()
		if err == nil || result != nil {
			t.Fatal("畸形或提前结束的分页不能作为完整列表")
		}
	}
}

// TestItemSyncRejectsRepeatedPages 验证远端重复返回同一页时及时失败，不能无限翻页或把重复列表用于删除；t 管理断言。
func TestItemSyncRejectsRepeatedPages(t *testing.T) {
	// server 总是返回同一个商品，w 输出固定列表。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"ret":["SUCCESS::调用成功"],"data":{"cardList":[{"cardData":{"id":"same-item"}}],"pageCount":3}}`)
	}))
	defer server.Close()
	// client 通过本地重写传输隔离真实闲鱼网络。
	client := &ClientImpl{HTTPClient: &http.Client{Transport: &rewriteTransport{base: server.Client().Transport, target: server.URL}}}
	// result、err 是重复页的全量读取结果。
	result, err := client.FetchAllItems(context.Background(), consignCookies, 1, 3)
	if err == nil || result != nil {
		t.Fatal("重复商品分页未被拒绝")
	}
}
