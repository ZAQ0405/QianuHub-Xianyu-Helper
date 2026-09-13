package mtop

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
)

// TestItemQueriesRejectTransportFailures 验证详情身份与商品全集在请求、读取及 HTTP 失败时不返回可执行事实；t 管理本地传输夹具。
func TestItemQueriesRejectTransportFailures(t *testing.T) {
	// mode 覆盖发送失败、响应读取失败与非成功 HTTP 状态，不请求真实平台。
	for _, mode := range []string{"network", "read", "http"} {
		t.Run(mode, func(t *testing.T) {
			// failure 是用于验证错误链的确定性本地故障。
			failure := errors.New("合成传输故障")
			// client 将所有请求交给内存传输；响应中的成功业务内容不能覆盖 HTTP 失败。
			client := &ClientImpl{HTTPClient: &http.Client{Transport: cookieSessionRoundTripFunc(func(*http.Request) (*http.Response, error) {
				if mode == "network" {
					return nil, failure
				}
				// body 保存模拟平台成功响应或读取故障。
				var body io.Reader = strings.NewReader(`{"ret":["SUCCESS::调用成功"],"data":{"cardList":[],"sellerDO":{"sellerId":"123"}}}`)
				// status 决定响应是否属于必须拒绝的 HTTP 失败。
				status := http.StatusServiceUnavailable
				if mode == "read" {
					body, status = iotest.ErrReader(failure), http.StatusOK
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(body)}, nil
			})}}
			// publisher、publisherErr 是身份查询结果，失败不得允许自动回复。
			publisher, publisherErr := client.FetchItemPublisher(context.Background(), consignCookies, "item")
			if publisherErr == nil || publisher != "" {
				t.Fatal("传输失败仍返回了发布人")
			}
			// items、itemsErr 是全集查询结果，失败不得提供可用于删除本地商品的空列表。
			items, itemsErr := client.FetchAllItems(context.Background(), consignCookies, 20, 3)
			if itemsErr == nil || items != nil {
				t.Fatal("传输失败仍返回了可同步全集")
			}
			if mode != "http" && (!errors.Is(publisherErr, failure) || !errors.Is(itemsErr, failure)) {
				t.Fatal("网络或读取错误丢失底层错误链")
			}
		})
	}
}

// TestItemPublisherRejectsInvalidQuery 验证空商品与畸形端点不能产生身份事实；t 管理输入边界断言。
func TestItemPublisherRejectsInvalidQuery(t *testing.T) {
	// itemID 选择空商品或可到达 URL 构造阶段的普通商品。
	for _, itemID := range []string{" ", "item"} {
		// client 的无效 URL 保证校验全程不会连接网络。
		client := &ClientImpl{ItemDetailURL: "://invalid"}
		// publisher、err 保存请求校验结果。
		publisher, err := client.FetchItemPublisher(context.Background(), consignCookies, itemID)
		if err == nil || publisher != "" {
			t.Fatal("非法查询仍返回了发布人")
		}
	}
}
