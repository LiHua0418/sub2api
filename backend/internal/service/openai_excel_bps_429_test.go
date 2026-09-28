package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExtractExcelBPS429WaitDuration(t *testing.T) {
	// Case 1: Milliseconds in error message
	msgMS := []byte(`Rate limit reached for gpt-6-sol in organization org-msnGs3IGIMHVb4KQfk72kh8y on tokens per min (TPM): Limit 40000000, Used 39588014, Requested 654120. Please try again in 363ms. Visit https://platform.openai.com/account/rate-limits to learn more.`)
	require.Equal(t, 363*time.Millisecond, extractExcelBPS429WaitDuration(msgMS, nil))

	// Case 2: 5ms
	msg5MS := []byte(`Rate limit exceeded. Please try again in 5ms.`)
	require.Equal(t, 5*time.Millisecond, extractExcelBPS429WaitDuration(msg5MS, nil))

	// Case 3: Seconds in error message
	msgS := []byte(`Rate limit exceeded. Please try again in 1.5s.`)
	require.Equal(t, 1500*time.Millisecond, extractExcelBPS429WaitDuration(msgS, nil))

	// Case 4: Retry-After header takes precedence if valid
	header := http.Header{}
	header.Set("Retry-After", "2")
	require.Equal(t, 2*time.Second, extractExcelBPS429WaitDuration(msgMS, header))

	// Case 5: Empty body and header
	require.Equal(t, time.Duration(0), extractExcelBPS429WaitDuration(nil, nil))

	// Case 6: Generic 429 without wait duration
	msgGeneric := []byte(`{"error": {"message": "Rate limit exceeded", "type": "requests"}}`)
	require.Equal(t, time.Duration(0), extractExcelBPS429WaitDuration(msgGeneric, nil))

	// Case 7: retry-after-ms header takes precedence over Retry-After
	headerMS := http.Header{}
	headerMS.Set("Retry-After", "1")
	headerMS.Set("retry-after-ms", "69")
	require.Equal(t, 69*time.Millisecond, extractExcelBPS429WaitDuration(msgMS, headerMS))

	// Case 8: x-ratelimit-reset-tokens header
	headerTokens := http.Header{}
	headerTokens.Set("x-ratelimit-reset-tokens", "85ms")
	require.Equal(t, 85*time.Millisecond, extractExcelBPS429WaitDuration(nil, headerTokens))

	// Case 9: JSON body with embedded retry-after-ms
	jsonBody := []byte(`{"error":{"code":"rate_limit_exceeded","headers":{"retry-after-ms":"157"}}}`)
	require.Equal(t, 157*time.Millisecond, extractExcelBPS429WaitDuration(jsonBody, nil))
}

func TestNormalizeExcelBPSToolChoice(t *testing.T) {
	// 1. auto / none untouched
	bodyAuto := []byte(`{"tool_choice":"auto"}`)
	out, changed := normalizeExcelBPSToolChoice(bodyAuto)
	require.False(t, changed)
	require.Equal(t, bodyAuto, out)

	bodyNone := []byte(`{"tool_choice":"none"}`)
	out, changed = normalizeExcelBPSToolChoice(bodyNone)
	require.False(t, changed)
	require.Equal(t, bodyNone, out)

	// 2. required normalized to auto
	bodyReq := []byte(`{"tool_choice":"required"}`)
	out, changed = normalizeExcelBPSToolChoice(bodyReq)
	require.True(t, changed)
	require.Equal(t, `{"tool_choice":"auto"}`, string(out))

	// 3. object tool_choice normalized to auto
	bodyObj := []byte(`{"tool_choice":{"type":"function","function":{"name":"shell"}}}`)
	out, changed = normalizeExcelBPSToolChoice(bodyObj)
	require.True(t, changed)
	require.Equal(t, `{"tool_choice":"auto"}`, string(out))
}
