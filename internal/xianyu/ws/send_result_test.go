package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestSendTextRequiresExactSuccessCode 验证聊天发送只有严格的 200 回执能够确认成功，且每种结果只发送一次业务帧。
func TestSendTextRequiresExactSuccessCode(t *testing.T) {
	// cases 保存平台回执状态与预期安全分类。
	cases := []struct {
		// name 是当前回执场景名称。
		name string
		// code 是本地平台替身返回的状态码。
		code int
		// want 是调用方应收到的发送结果分类；空值表示明确成功。
		want SendErrorKind
	}{
		{name: "success", code: http.StatusOK},
		{name: "other_2xx", code: http.StatusCreated, want: SendUncertain},
		{name: "redirect", code: http.StatusFound, want: SendUncertain},
		{name: "rejected", code: http.StatusBadRequest, want: SendRejected},
		{name: "request_timeout", code: http.StatusRequestTimeout, want: SendUncertain},
		{name: "last_4xx", code: 499, want: SendRejected},
		{name: "server_error", code: http.StatusInternalServerError, want: SendUncertain},
	}
	// testCase 是当前执行的聊天发送回执场景。
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// connection 和 requests 保存隔离的本地 WebSocket 连接及其业务帧记录。
			connection, requests := newAPIResponseConn(t, nil, testCase.code)
			// sendErr 保存一次文字发送得到的平台确认分类。
			sendErr := connection.SendText(context.Background(), "100", "chat-1", "200", "测试消息")
			// got 保存上层从发送错误中读取的安全结果分类。
			got := SendResultKind(sendErr)
			if got != testCase.want {
				t.Fatalf("code=%d result=%q, want=%q err=%v", testCase.code, got, testCase.want, sendErr)
			}
			if len(requests) != 1 {
				t.Fatalf("code=%d sent %d business frames, want 1", testCase.code, len(requests))
			}
		})
	}
}

// TestStrictChatSendResponseCodeRejectsMalformedValues 验证聊天发送确认不会截断小数或接受带符号、尾随字符的状态码。
func TestStrictChatSendResponseCodeRejectsMalformedValues(t *testing.T) {
	// cases 保存严格状态码解析的输入、结果和有效性。
	cases := []struct {
		// value 是平台响应中的原始 code 字段。
		value any
		// want 是有效输入应解析出的整数状态码。
		want int
		// valid 表示输入是否是完整整数形式。
		valid bool
	}{
		{value: 200, want: 200, valid: true},
		{value: float64(200), want: 200, valid: true},
		{value: json.Number("200"), want: 200, valid: true},
		{value: " 200 ", want: 200, valid: true},
		{value: 200.5},
		{value: json.Number("200.0")},
		{value: "200abc"},
		{value: "+200"},
		{value: nil},
	}
	// testCase 是当前执行的严格解析场景。
	for _, testCase := range cases {
		// got 和 valid 保存被测解析结果及其有效性。
		got, valid := strictChatSendResponseCode(testCase.value)
		if got != testCase.want || valid != testCase.valid {
			t.Fatalf("value=%v result=(%d,%v), want=(%d,%v)", testCase.value, got, valid, testCase.want, testCase.valid)
		}
	}
}
