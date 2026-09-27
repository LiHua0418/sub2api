package service

import (
	"bytes"
	"math/rand"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// ContextKeyCacheReductionRatio 是在 gin.Context 中缓存已解析的削减比例的键，
	// 确保同一请求中所有 SSE chunk 和最终计费使用的是同一个确定的随机削减比例。
	ContextKeyCacheReductionRatio = "sub2api_cache_reduction_ratio"

	// MinCachedTokensForReduction 缓存削减的最低门槛：仅当 cached_tokens >= 1024 时生效
	MinCachedTokensForReduction = 1024

	// CacheReductionChunkSize 缓存 token 对齐块大小：OpenAI 缓存以 128 tokens 为单位，
	// 削减后向下取整到 128 的整数倍。
	CacheReductionChunkSize = 128
)

// CalculateReducedCachedTokens 计算削减后的缓存 token 数：
//   - cachedTokens < 1024 时不削减；
//   - ratio <= 0 时不削减；
//   - 目标削减后的 cached_tokens = floor(cachedTokens * (1 - ratio) / 128) * 128；
//   - 保证 0 <= newCached <= cachedTokens；
//   - 保证 newCached % 128 == 0；
//   - 返回 (newCached, changed)。
func CalculateReducedCachedTokens(cachedTokens int, ratio float64) (int, bool) {
	if cachedTokens < MinCachedTokensForReduction || ratio <= 0 {
		return cachedTokens, false
	}
	if ratio > 1.0 {
		ratio = 1.0
	}
	target := float64(cachedTokens) * (1.0 - ratio)
	newCached := (int(target) / CacheReductionChunkSize) * CacheReductionChunkSize
	if newCached < 0 {
		newCached = 0
	}
	if newCached > cachedTokens {
		newCached = (cachedTokens / CacheReductionChunkSize) * CacheReductionChunkSize
	}
	if newCached == cachedTokens {
		return cachedTokens, false
	}
	return newCached, true
}

// ResolveCacheReductionRatio 解析并固定当前请求的缓存削减比例。
// 若当前请求已解析过，则直接返回已有的比例。
func ResolveCacheReductionRatio(c *gin.Context, group *Group) float64 {
	if group == nil || !group.CacheReductionEnabled {
		return 0
	}
	if c != nil {
		if v, exists := c.Get(ContextKeyCacheReductionRatio); exists {
			if r, ok := v.(float64); ok {
				return r
			}
		}
	}
	minRatio := group.CacheReductionMinRatio
	maxRatio := group.CacheReductionMaxRatio
	if minRatio < 0 {
		minRatio = 0
	}
	if maxRatio < 0 {
		maxRatio = 0
	}
	if minRatio > 1.0 {
		minRatio = 1.0
	}
	if maxRatio > 1.0 {
		maxRatio = 1.0
	}
	if minRatio > maxRatio {
		minRatio = maxRatio
	}
	if maxRatio <= 0 {
		if c != nil {
			c.Set(ContextKeyCacheReductionRatio, 0.0)
		}
		return 0
	}

	var ratio float64
	if minRatio >= maxRatio {
		ratio = minRatio
	} else {
		ratio = minRatio + rand.Float64()*(maxRatio-minRatio)
	}
	if c != nil {
		c.Set(ContextKeyCacheReductionRatio, ratio)
	}
	return ratio
}

// resolveGroupForCacheReduction 从 gin.Context 或 Request Context 中提取分组配置
func resolveGroupForCacheReduction(c *gin.Context) *Group {
	if c == nil {
		return nil
	}
	if req := c.Request; req != nil {
		if ctx := req.Context(); ctx != nil {
			if g, ok := ctx.Value(ctxkey.Group).(*Group); ok && g != nil {
				return g
			}
		}
	}
	if v, exists := c.Get("api_key"); exists {
		if apiKey, ok := v.(*APIKey); ok && apiKey != nil && apiKey.Group != nil {
			return apiKey.Group
		}
	}
	return nil
}

// ApplyOpenAICacheReductionToJSONBytes 如果命中缓存削减策略，对响应体/chunk 中的 usage 进行就地重写：
//   - 削减 cached_tokens 为向下对齐 128 的整数倍；
//   - prompt_tokens / total_tokens 保持守恒不改动；
//   - 返回重写后的 []byte 以及是否发生改变。
func ApplyOpenAICacheReductionToJSONBytes(c *gin.Context, body []byte) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	group := resolveGroupForCacheReduction(c)
	if group == nil || !group.CacheReductionEnabled {
		return body, false
	}
	ratio := ResolveCacheReductionRatio(c, group)
	if ratio <= 0 {
		return body, false
	}
	return ApplyOpenAICacheReductionWithRatio(body, ratio)
}

// ApplyOpenAICacheReductionWithRatio 根据指定的削减比例重写 body 中的 cached_tokens。
// 仅当包含 cached_tokens 且 >= 1024 时执行重写。
func ApplyOpenAICacheReductionWithRatio(body []byte, ratio float64) ([]byte, bool) {
	if len(body) == 0 || ratio <= 0 {
		return body, false
	}
	// 快速路径：如果不包含任何可能的缓存命中字段名，直接跳过 JSON 扫描
	if !bytes.Contains(body, []byte(`"cached_tokens"`)) &&
		!bytes.Contains(body, []byte(`"cache_read_tokens"`)) &&
		!bytes.Contains(body, []byte(`"cache_read_input_tokens"`)) {
		return body, false
	}
	if !gjson.ValidBytes(body) {
		return body, false
	}

	usageRoots := []string{
		"usage",
		"response.usage",
		"data.usage",
		"data.response.usage",
	}

	subPaths := []string{
		"prompt_tokens_details.cached_tokens",
		"input_tokens_details.cached_tokens",
		"cache_read_input_tokens",
		"cache_read_tokens",
		"cached_tokens",
	}

	changed := false
	resultBody := body

	for _, root := range usageRoots {
		rootVal := gjson.GetBytes(resultBody, root)
		if !rootVal.Exists() || !rootVal.IsObject() {
			continue
		}

		// 检查该 root 下是否有 cached_tokens >= 1024
		var origVal int64 = 0
		for _, sub := range subPaths {
			path := root + "." + sub
			item := gjson.GetBytes(resultBody, path)
			if item.Exists() && item.Int() >= MinCachedTokensForReduction {
				origVal = item.Int()
				break
			}
		}

		if origVal < MinCachedTokensForReduction {
			continue
		}

		newCached, ok := CalculateReducedCachedTokens(int(origVal), ratio)
		if !ok || newCached == int(origVal) {
			continue
		}

		// 将该 root 下所有存在的 cached 相关字段同步更新为 newCached
		for _, sub := range subPaths {
			path := root + "." + sub
			item := gjson.GetBytes(resultBody, path)
			if item.Exists() && item.Int() == origVal {
				if updated, err := sjson.SetBytes(resultBody, path, newCached); err == nil {
					resultBody = updated
					changed = true
				}
			}
		}
	}

	return resultBody, changed
}
