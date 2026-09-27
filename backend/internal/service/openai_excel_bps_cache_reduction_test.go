package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSCacheReductionAndCacheCreationAsInputIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Usage with 4096 input, 2048 cached, 1024 cache creation
	usageJSON := `{"input_tokens":4096,"output_tokens":100,"total_tokens":4196,"input_tokens_details":{"cached_tokens":2048,"cache_creation_tokens":1024},"prompt_tokens_details":{"cached_tokens":2048,"cache_creation_tokens":1024},"cache_creation_input_tokens":1024,"cache_read_input_tokens":2048}`

	for _, stream := range []bool{false, true} {
		for _, cacheCreationAsInput := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/creationAsInput=%t", stream, cacheCreationAsInput), func(t *testing.T) {
				wire := `data: {"type":"response.completed","response":{"id":"resp_test_cr","status":"completed","model":"gpt-6-astra","output":[],"usage":` + usageJSON + `}}` + "\n\n"
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader(wire)),
				}}

				svc := openAIClientToolsTestService(upstream)
				account := excelAccount()
				account.Extra["openai_excel_bps_cache_creation_as_input"] = cacheCreationAsInput

				group := &Group{
					ID:                     18,
					Platform:               PlatformOpenAI,
					CacheReductionEnabled:  true,
					CacheReductionMinRatio: 0.10, // 10% reduction
					CacheReductionMaxRatio: 0.10,
				}

				body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"input":"test cache reduction"}`, stream))
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				ctx := context.WithValue(req.Context(), ctxkey.Group, group)
				c.Request = req.WithContext(ctx)

				result, err := svc.Forward(c.Request.Context(), c, account, body)
				require.NoError(t, err)

				// 1. Upstream usage captured in result.Usage:
				// 2048 reduced by 10% (204.8) -> 1843.2 -> floored to 128 multiple -> 1792
				require.Equal(t, 1792, result.Usage.CacheReadInputTokens, "result.Usage must reflect reduced cached tokens for correct billing")
				require.Equal(t, 1024, result.Usage.CacheCreationInputTokens, "result.Usage must retain upstream creation tokens for internal audit")
				require.Equal(t, 4096, result.Usage.InputTokens)
				require.Equal(t, 100, result.Usage.OutputTokens)

				// 2. Downstream client payload verification:
				var responsePayload []byte
				if stream {
					for _, line := range strings.Split(rec.Body.String(), "\n") {
						if strings.HasPrefix(line, "data: ") {
							responsePayload = []byte(gjson.Get(strings.TrimPrefix(line, "data: "), "response").Raw)
						}
					}
				} else {
					responsePayload = rec.Body.Bytes()
				}

				downstreamUsage, ok := extractOpenAIUsageFromJSONBytes(responsePayload)
				require.True(t, ok)

				// Downstream cached_tokens must be reduced to 1792 (128-aligned)
				require.Equal(t, 1792, downstreamUsage.CacheReadInputTokens, "downstream cached_tokens must be reduced")
				require.Equal(t, 0, downstreamUsage.CacheReadInputTokens%128)

				// Downstream cache creation must be 0 if cacheCreationAsInput is true
				if cacheCreationAsInput {
					require.Equal(t, 0, downstreamUsage.CacheCreationInputTokens)
				} else {
					require.Equal(t, 1024, downstreamUsage.CacheCreationInputTokens)
				}

				// Total prompt tokens must remain invariant (4096)
				require.Equal(t, 4096, downstreamUsage.InputTokens)
				require.Equal(t, 100, downstreamUsage.OutputTokens)
			})
		}
	}
}
