package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSChatCompletionsCacheCreationAsInput_Stream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			wire := `data: {"type":"response.created","response":{"id":"resp_123","model":"gpt-6-luna"}}` + "\n\n" +
				`data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n" +
				`data: {"type":"response.completed","response":{"id":"resp_123","status":"completed","model":"gpt-6-luna","output":[],"usage":` + excelBPSUsageWithCreation + `}}` + "\n\n"

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-6-luna","stream":true}`)))
			if enabled {
				c.Set("excel_bps_cache_creation_as_input", true)
			}

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
					"x-request-id": []string{"test-req-id"},
				},
				Body: io.NopCloser(strings.NewReader(wire)),
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			account := excelAccount()

			result, err := svc.handleChatStreamingResponse(
				resp,
				c,
				account,
				"gpt-6-luna",
				"gpt-6-luna",
				"gpt-6-luna",
				time.Now(),
				0,
			)
			require.NoError(t, err)
			require.NotNil(t, result)
			// Upstream measurement retained for audit/internal billing
			require.Equal(t, 200, result.Usage.CacheCreationInputTokens)

			// Inspect downstream chunks emitted to client
			body := rec.Body.String()
			var lastUsageChunk string
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"usage"`) {
					lastUsageChunk = strings.TrimPrefix(line, "data: ")
				}
			}
			require.NotEmpty(t, lastUsageChunk, "terminal usage chunk must be emitted")

			usageJSON := gjson.Get(lastUsageChunk, "usage")
			require.True(t, usageJSON.Exists())
			details := usageJSON.Get("prompt_tokens_details")

			if enabled {
				require.EqualValues(t, 0, details.Get("cache_write_tokens").Int())
				require.EqualValues(t, 0, details.Get("cache_creation_tokens").Int())
				require.False(t, details.Get("cache_write_tokens").Exists())
				require.False(t, details.Get("cache_creation_tokens").Exists())
			} else {
				require.EqualValues(t, 200, details.Get("cache_write_tokens").Int())
			}
		})
	}
}

func TestExcelBPSChatCompletionsCacheCreationAsInput_Buffered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			wire := `data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n" +
				`data: {"type":"response.completed","response":{"id":"resp_123","status":"completed","model":"gpt-6-luna","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],"usage":` + excelBPSUsageWithCreation + `}}` + "\n\n"

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-6-luna","stream":false}`)))
			if enabled {
				c.Set("excel_bps_cache_creation_as_input", true)
			}

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
					"x-request-id": []string{"test-req-id"},
				},
				Body: io.NopCloser(strings.NewReader(wire)),
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			account := excelAccount()

			result, err := svc.handleChatBufferedStreamingResponse(
				resp,
				c,
				account,
				"gpt-6-luna",
				"gpt-6-luna",
				"gpt-6-luna",
				time.Now(),
			)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 200, result.Usage.CacheCreationInputTokens)

			// Inspect downstream JSON response
			responseBody := rec.Body.Bytes()
			usageJSON := gjson.GetBytes(responseBody, "usage")
			require.True(t, usageJSON.Exists())
			details := usageJSON.Get("prompt_tokens_details")

			if enabled {
				require.EqualValues(t, 0, details.Get("cache_write_tokens").Int())
				require.EqualValues(t, 0, details.Get("cache_creation_tokens").Int())
				require.False(t, details.Get("cache_write_tokens").Exists())
				require.False(t, details.Get("cache_creation_tokens").Exists())
			} else {
				require.EqualValues(t, 200, details.Get("cache_write_tokens").Int())
			}
		})
	}
}
