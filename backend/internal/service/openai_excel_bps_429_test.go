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
}
