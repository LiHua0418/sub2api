package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCalculateReducedCachedTokens(t *testing.T) {
	tests := []struct {
		name         string
		cachedTokens int
		ratio        float64
		expectedVal  int
		expectedOk   bool
	}{
		{
			name:         "below 1024 threshold - 500 tokens",
			cachedTokens: 500,
			ratio:        0.10,
			expectedVal:  500,
			expectedOk:   false,
		},
		{
			name:         "below 1024 threshold - 1023 tokens",
			cachedTokens: 1023,
			ratio:        0.10,
			expectedVal:  1023,
			expectedOk:   false,
		},
		{
			name:         "zero ratio - no reduction",
			cachedTokens: 2048,
			ratio:        0.0,
			expectedVal:  2048,
			expectedOk:   false,
		},
		{
			name:         "negative ratio - no reduction",
			cachedTokens: 2048,
			ratio:        -0.05,
			expectedVal:  2048,
			expectedOk:   false,
		},
		{
			name:         "exact 1024 tokens with 5% reduction (1024*0.95=972.8 -> 972/128*128=896)",
			cachedTokens: 1024,
			ratio:        0.05,
			expectedVal:  896,
			expectedOk:   true,
		},
		{
			name:         "2048 tokens with 10% reduction (2048*0.90=1843.2 -> 1843/128*128=1792)",
			cachedTokens: 2048,
			ratio:        0.10,
			expectedVal:  1792,
			expectedOk:   true,
		},
		{
			name:         "1500 tokens with 10% reduction (1500*0.90=1350 -> 1350/128*128=1280)",
			cachedTokens: 1500,
			ratio:        0.10,
			expectedVal:  1280,
			expectedOk:   true,
		},
		{
			name:         "100% reduction (ratio=1.0)",
			cachedTokens: 1024,
			ratio:        1.0,
			expectedVal:  0,
			expectedOk:   true,
		},
		{
			name:         "over 100% reduction (clamped to 1.0)",
			cachedTokens: 2048,
			ratio:        1.5,
			expectedVal:  0,
			expectedOk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := CalculateReducedCachedTokens(tt.cachedTokens, tt.ratio)
			assert.Equal(t, tt.expectedOk, ok)
			assert.Equal(t, tt.expectedVal, val)
			if ok {
				assert.Equal(t, 0, val%CacheReductionChunkSize, "reduced tokens must be multiple of 128")
				assert.LessOrEqual(t, val, tt.cachedTokens, "reduced tokens must be <= original")
				assert.GreaterOrEqual(t, val, 0, "reduced tokens must be non-negative")
			}
		})
	}
}

func TestApplyOpenAICacheReductionWithRatio_ChatCompletion(t *testing.T) {
	inputJSON := []byte(`{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"model": "gpt-4o",
		"usage": {
			"prompt_tokens": 2048,
			"completion_tokens": 100,
			"total_tokens": 2148,
			"prompt_tokens_details": {
				"cached_tokens": 1024
			}
		}
	}`)

	// With ratio = 0.05, 1024 * 0.95 = 972.8 -> 896
	outputJSON, changed := ApplyOpenAICacheReductionWithRatio(inputJSON, 0.05)
	require.True(t, changed)

	// Check token conservation: prompt_tokens and total_tokens must NOT change
	assert.Equal(t, int64(2048), gjson.GetBytes(outputJSON, "usage.prompt_tokens").Int())
	assert.Equal(t, int64(100), gjson.GetBytes(outputJSON, "usage.completion_tokens").Int())
	assert.Equal(t, int64(2148), gjson.GetBytes(outputJSON, "usage.total_tokens").Int())

	// Check reduced cached_tokens: floored to multiple of 128
	cachedTokens := gjson.GetBytes(outputJSON, "usage.prompt_tokens_details.cached_tokens").Int()
	assert.Equal(t, int64(896), cachedTokens)
	assert.Equal(t, int64(0), cachedTokens%128)
}

func TestApplyOpenAICacheReductionWithRatio_ResponsesStreamEvent(t *testing.T) {
	inputJSON := []byte(`{
		"type": "response.completed",
		"response": {
			"id": "resp_001",
			"status": "completed",
			"usage": {
				"input_tokens": 4096,
				"output_tokens": 200,
				"total_tokens": 4296,
				"input_tokens_details": {
					"cached_tokens": 2048
				}
			}
		}
	}`)

	// With ratio = 0.10: 2048 * 0.90 = 1843.2 -> 1792
	outputJSON, changed := ApplyOpenAICacheReductionWithRatio(inputJSON, 0.10)
	require.True(t, changed)

	assert.Equal(t, int64(4096), gjson.GetBytes(outputJSON, "response.usage.input_tokens").Int())
	assert.Equal(t, int64(200), gjson.GetBytes(outputJSON, "response.usage.output_tokens").Int())
	assert.Equal(t, int64(4296), gjson.GetBytes(outputJSON, "response.usage.total_tokens").Int())

	cachedTokens := gjson.GetBytes(outputJSON, "response.usage.input_tokens_details.cached_tokens").Int()
	assert.Equal(t, int64(1792), cachedTokens)
	assert.Equal(t, int64(0), cachedTokens%128)
}

func TestApplyOpenAICacheReductionWithRatio_BelowThreshold(t *testing.T) {
	inputJSON := []byte(`{
		"usage": {
			"prompt_tokens": 800,
			"completion_tokens": 50,
			"total_tokens": 850,
			"prompt_tokens_details": {
				"cached_tokens": 512
			}
		}
	}`)

	outputJSON, changed := ApplyOpenAICacheReductionWithRatio(inputJSON, 0.10)
	assert.False(t, changed)
	assert.Equal(t, inputJSON, outputJSON)
}

func TestApplyOpenAICacheReductionToJSONBytes_ContextGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	group := &Group{
		ID:                     1,
		Platform:               PlatformOpenAI,
		CacheReductionEnabled:  true,
		CacheReductionMinRatio: 0.05,
		CacheReductionMaxRatio: 0.05,
	}
	ctx := context.WithValue(req.Context(), ctxkey.Group, group)
	req = req.WithContext(ctx)
	c.Request = req

	inputJSON := []byte(`{
		"usage": {
			"prompt_tokens": 2048,
			"prompt_tokens_details": {
				"cached_tokens": 1024
			}
		}
	}`)

	outputJSON, changed := ApplyOpenAICacheReductionToJSONBytes(c, inputJSON)
	require.True(t, changed)

	assert.Equal(t, int64(896), gjson.GetBytes(outputJSON, "usage.prompt_tokens_details.cached_tokens").Int())
	assert.Equal(t, int64(2048), gjson.GetBytes(outputJSON, "usage.prompt_tokens").Int())

	// Calling again with the same gin.Context should reuse the cached ratio
	ratio := ResolveCacheReductionRatio(c, group)
	assert.Equal(t, 0.05, ratio)
}

func TestApplyOpenAICacheReductionToJSONBytes_DisabledGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	group := &Group{
		ID:                     1,
		Platform:               PlatformOpenAI,
		CacheReductionEnabled:  false,
		CacheReductionMinRatio: 0.10,
		CacheReductionMaxRatio: 0.20,
	}
	ctx := context.WithValue(req.Context(), ctxkey.Group, group)
	req = req.WithContext(ctx)
	c.Request = req

	inputJSON := []byte(`{
		"usage": {
			"prompt_tokens": 2048,
			"prompt_tokens_details": {
				"cached_tokens": 1024
			}
		}
	}`)

	outputJSON, changed := ApplyOpenAICacheReductionToJSONBytes(c, inputJSON)
	assert.False(t, changed)
	assert.Equal(t, inputJSON, outputJSON)
}

func TestValidateCacheReductionConfig(t *testing.T) {
	// Disabled passes for any platform
	require.NoError(t, ValidateCacheReductionConfig(PlatformAnthropic, false, 0, 0))

	// Enabled passes for OpenAI and Composite
	for _, platform := range []string{PlatformOpenAI, PlatformComposite} {
		require.NoError(t, ValidateCacheReductionConfig(platform, true, 0.05, 0.10))
		require.NoError(t, ValidateCacheReductionConfig(platform, true, 0, 0))
		require.NoError(t, ValidateCacheReductionConfig(platform, true, 0.5, 0.5))
		require.NoError(t, ValidateCacheReductionConfig(platform, true, 0, 1.0))
	}

	// Unsupported platform when enabled
	require.Error(t, ValidateCacheReductionConfig(PlatformAnthropic, true, 0.05, 0.10))
	require.Error(t, ValidateCacheReductionConfig(PlatformGemini, true, 0.05, 0.10))

	// Invalid ratios
	require.Error(t, ValidateCacheReductionConfig(PlatformOpenAI, true, -0.05, 0.10))
	require.Error(t, ValidateCacheReductionConfig(PlatformOpenAI, true, 0.05, 1.5))
	require.Error(t, ValidateCacheReductionConfig(PlatformOpenAI, true, 0.20, 0.10), "min > max must fail")
}

func TestNormalizeCacheReductionConfig(t *testing.T) {
	t.Run("unsupported platform resets", func(t *testing.T) {
		enabled, minR, maxR := NormalizeCacheReductionConfig(PlatformAnthropic, true, 0.05, 0.10)
		require.False(t, enabled)
		require.Zero(t, minR)
		require.Zero(t, maxR)
	})

	t.Run("supported platform normalizes", func(t *testing.T) {
		enabled, minR, maxR := NormalizeCacheReductionConfig(PlatformOpenAI, true, 0.05, 0.10)
		require.True(t, enabled)
		require.InDelta(t, 0.05, minR, 1e-12)
		require.InDelta(t, 0.10, maxR, 1e-12)
	})

	t.Run("min greater than max adjusts min to max", func(t *testing.T) {
		enabled, minR, maxR := NormalizeCacheReductionConfig(PlatformOpenAI, true, 0.20, 0.10)
		require.True(t, enabled)
		require.InDelta(t, 0.10, minR, 1e-12)
		require.InDelta(t, 0.10, maxR, 1e-12)
	})

	t.Run("negative values cleaned", func(t *testing.T) {
		enabled, minR, maxR := NormalizeCacheReductionConfig(PlatformOpenAI, true, -0.05, 0.10)
		require.True(t, enabled)
		require.Zero(t, minR)
		require.InDelta(t, 0.10, maxR, 1e-12)
	})
}

func TestCacheReduction_TokenConservationAndBillingAlignment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	group := &Group{
		ID:                     1,
		Platform:               PlatformOpenAI,
		CacheReductionEnabled:  true,
		CacheReductionMinRatio: 0.10,
		CacheReductionMaxRatio: 0.10,
	}
	ctx := context.WithValue(req.Context(), ctxkey.Group, group)
	req = req.WithContext(ctx)
	c.Request = req

	// Upstream returns 4096 prompt tokens, 2048 of which were cached
	upstreamJSON := []byte(`{
		"id": "chatcmpl-test",
		"object": "chat.completion",
		"usage": {
			"prompt_tokens": 4096,
			"completion_tokens": 120,
			"total_tokens": 4216,
			"prompt_tokens_details": {
				"cached_tokens": 2048
			}
		}
	}`)

	// Without cache reduction:
	origUsage, ok := extractOpenAIUsageFromJSONBytes(upstreamJSON)
	require.True(t, ok)
	origActualInput := origUsage.InputTokens - origUsage.CacheReadInputTokens
	assert.Equal(t, 4096, origUsage.InputTokens)
	assert.Equal(t, 2048, origUsage.CacheReadInputTokens)
	assert.Equal(t, 2048, origActualInput)

	// Apply reduction (10% off 2048 -> 1843.2 -> floored to 1792)
	reducedJSON, changed := ApplyOpenAICacheReductionToJSONBytes(c, upstreamJSON)
	require.True(t, changed)

	reducedUsage, ok := extractOpenAIUsageFromJSONBytes(reducedJSON)
	require.True(t, ok)

	// Token conservation checks:
	// 1. Total prompt_tokens remains invariant
	assert.Equal(t, origUsage.InputTokens, reducedUsage.InputTokens, "total prompt_tokens must be conserved")
	// 2. Completion tokens remain invariant
	assert.Equal(t, origUsage.OutputTokens, reducedUsage.OutputTokens, "completion_tokens must be invariant")
	// 3. Cached tokens reduced to 1792 (multiple of 128)
	assert.Equal(t, 1792, reducedUsage.CacheReadInputTokens)
	assert.Equal(t, 0, reducedUsage.CacheReadInputTokens%128)

	// Billing alignment checks:
	// Converted tokens = 2048 - 1792 = 256 tokens converted to standard input
	reducedActualInput := reducedUsage.InputTokens - reducedUsage.CacheReadInputTokens
	assert.Equal(t, 2304, reducedActualInput)
	assert.Equal(t, 256, reducedActualInput-origActualInput, "converted tokens must be billed as standard input")
	assert.Equal(t, reducedUsage.InputTokens, reducedActualInput+reducedUsage.CacheReadInputTokens, "sum of actual and cached must equal total prompt tokens")
}
