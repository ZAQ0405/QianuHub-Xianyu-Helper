package mtop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReshelfItemKeepsOriginalID 验证重新上架先读取编辑详情，再用原 itemId 提交编辑。
func TestReshelfItemKeepsOriginalID(t *testing.T) {
	// calls 记录编辑详情和编辑提交的调用顺序。
	var calls []string
	// server 模拟闲鱼同商品重新上架的两个 MTOP 接口。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// api 保存当前模拟请求调用的 MTOP 接口名称。
		api := r.URL.Query().Get("api")
		calls = append(calls, api)
		if r.Header.Get("Referer") != "https://www.goofish.com/publish?itemId=item-original" {
			t.Fatalf("referer=%q", r.Header.Get("Referer"))
		}
		if r.URL.Query().Get("spm_cnt") != "a21ybx.publish.0.0" || r.URL.Query().Get("spm_pre") != "a21ybx.publish.0.0" {
			t.Fatalf("publish tracking missing: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		if api == "mtop.idle.pc.idleitem.editDetail" {
			if r.URL.Query().Get("log_id") != "xianyu_item_edit_detail" {
				t.Fatalf("edit detail log_id=%q", r.URL.Query().Get("log_id"))
			}
			_, _ = w.Write([]byte(`{"ret":["SUCCESS::调用成功"],"data":{"itemId":"item-original","quantity":"1","itemTextDTO":{"title":"资料","desc":"说明"},"userRightsProtocols":[{"enable":"true"}]}}`))
			return
		}
		if api != "mtop.idle.pc.idleitem.edit" {
			t.Fatalf("unexpected api: %s", api)
		}
		// form 保存编辑提交的 JSON 表单数据。
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		// payload 保存同商品编辑提交载荷。
		var payload map[string]any
		if /* err 保存编辑载荷 JSON 解析错误。 */ err := json.Unmarshal([]byte(r.Form.Get("data")), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["itemId"] != "item-original" || payload["sourceId"] != "item-original" {
			t.Fatalf("商品 ID 被替换: %v", payload)
		}
		if r.URL.Query().Get("log_id") != "xianyu_item_edit" {
			t.Fatalf("edit log_id=%q", r.URL.Query().Get("log_id"))
		}
		_, _ = w.Write([]byte(`{"ret":["SUCCESS::调用成功"],"data":{}}`))
	}))
	defer server.Close()
	// client 使用本地服务替代平台端点，验证真实请求链路。
	client := &ClientImpl{HTTPClient: server.Client(), ItemEditDetailURL: server.URL, ItemEditURL: server.URL}
	// result、err 保存同商品重新上架结果。
	// result、err 保存同商品重新上架结果及请求错误。
	result, err := client.ReshelfItem(context.Background(), "unb=123; _m_h5_tk=token_1", "item-original")
	if err != nil || result == nil || !result.Success {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(calls) != 2 || calls[0] != "mtop.idle.pc.idleitem.editDetail" || calls[1] != "mtop.idle.pc.idleitem.edit" {
		t.Fatalf("calls=%v", calls)
	}
}

// TestReshelfItemVerifiesActiveStateAfterBusinessError 验证平台返回业务异常但商品已上架时按最终状态确认成功。
func TestReshelfItemVerifiesActiveStateAfterBusinessError(t *testing.T) {
	// calls 记录详情、编辑和在售列表核验的调用顺序。
	var calls []string
	// server 模拟编辑接口返回异常，但在售商品列表已经包含原商品。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// api 保存当前模拟请求的 MTOP 接口名称。
		api := r.URL.Query().Get("api")
		calls = append(calls, api)
		w.Header().Set("Content-Type", "application/json")
		switch api {
		case "mtop.idle.pc.idleitem.editDetail":
			_, _ = w.Write([]byte(`{"ret":["SUCCESS::调用成功"],"data":{"itemId":"item-original","quantity":"1","itemTextDTO":{"title":"资料"}}}`))
		case "mtop.idle.pc.idleitem.edit":
			_, _ = w.Write([]byte(`{"ret":["FAIL_BIZ_PC_NOT_SUPPORT_PUBLISH_OR_EDIT::该商品不支持网页发布或编辑"],"data":{}}`))
		case "mtop.idle.web.xyh.item.list":
			_, _ = w.Write([]byte(`{"ret":["SUCCESS::调用成功"],"data":{"cardList":[{"cardData":{"id":"item-original","title":"资料","itemStatus":0,"auctionType":"b","detailParams":{"itemId":"item-original"},"priceInfo":{"price":"1","preText":"¥"}}}]}}`))
		default:
			t.Fatalf("unexpected api: %s", api)
		}
	}))
	defer server.Close()
	// client 将编辑和在售列表端点都指向本地替身，验证失败后的最终状态核验链路。
	client := &ClientImpl{HTTPClient: server.Client(), ItemEditDetailURL: server.URL, ItemEditURL: server.URL, ItemListURL: server.URL}
	// result、err 保存按最终在售状态确认后的重新上架结果。
	result, err := client.ReshelfItem(context.Background(), "unb=123; _m_h5_tk=token_1", "item-original")
	if err != nil || result == nil || !result.Success {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(calls) != 3 || calls[0] != "mtop.idle.pc.idleitem.editDetail" || calls[1] != "mtop.idle.pc.idleitem.edit" || calls[2] != "mtop.idle.web.xyh.item.list" {
		t.Fatalf("calls=%v", calls)
	}
}

// TestBuildReshelfPayloadDefaultsOriginalID 验证编辑详情字段不足时仍补齐原商品 ID 和编辑默认字段。
func TestBuildReshelfPayloadDefaultsOriginalID(t *testing.T) {
	// payload 保存字段不完整的编辑详情转换结果。
	payload := buildReshelfPayload(map[string]any{}, "item-original")
	if payload["itemId"] != "item-original" || payload["sourceId"] != "item-original" || payload["quantity"] != "1" {
		t.Fatalf("payload=%v", payload)
	}
}
