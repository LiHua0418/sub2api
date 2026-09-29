package service

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// excelBPSDownstreamUsage applies the account's cache-creation-as-input policy
// to client-visible usage after recording the original upstream measurement.
// OpenAI total input already includes cache creation: keep totals and cache
// reads intact, and clear every supported cache-write alias and TTL breakdown.
func excelBPSDownstreamUsage(payload []byte) ([]byte, error) {
	for _, path := range []string{"usage", "response.usage"} {
		usage := gjson.GetBytes(payload, path)
		if !usage.IsObject() {
			continue
		}
		normalized := []byte(usage.Raw)
		changed := false
		for _, field := range []string{
			"input_tokens_details.cache_write_tokens", "prompt_tokens_details.cache_write_tokens",
			"input_tokens_details.cache_creation_tokens", "prompt_tokens_details.cache_creation_tokens",
			"input_tokens_details.cached_creation_tokens", "prompt_tokens_details.cached_creation_tokens",
			"cache_write_tokens", "cache_creation_input_tokens", "cache_write_input_tokens", "cache_creation_tokens",
			"cached_creation_tokens",
			"cache_creation.ephemeral_5m_input_tokens", "cache_creation.ephemeral_1h_input_tokens",
			"claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens",
		} {
			if value := usage.Get(field); !value.Exists() || value.Raw == "0" {
				continue
			}
			var err error
			normalized, err = sjson.SetBytes(normalized, field, 0)
			if err != nil {
				return nil, err
			}
			changed = true
		}
		if changed {
			var err error
			payload, err = sjson.SetRawBytes(payload, path, normalized)
			if err != nil {
				return nil, err
			}
		}
	}
	return payload, nil
}

func clearResponsesUsageCacheCreation(u *apicompat.ResponsesUsage) {
	if u == nil {
		return
	}
	u.CacheCreationInputTokens = 0
	if u.InputTokensDetails != nil {
		u.InputTokensDetails.CacheCreationTokens = 0
		u.InputTokensDetails.CacheWriteTokens = 0
	}
}

func clearChatUsageCacheCreation(u *apicompat.ChatUsage) {
	if u == nil {
		return
	}
	if u.PromptTokensDetails != nil {
		u.PromptTokensDetails.CacheCreationTokens = 0
		u.PromptTokensDetails.CacheWriteTokens = 0
	}
}

