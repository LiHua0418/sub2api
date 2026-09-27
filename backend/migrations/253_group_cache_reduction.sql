-- Upstream cache hit reduction for OpenAI/Codex requests.
-- When enabled, requests with cached_tokens >= 1024 have their reported
-- cached_tokens reduced by a random ratio within [min_ratio, max_ratio],
-- floored to multiples of 128 tokens. Total prompt_tokens remains conserved,
-- converting the difference into standard input tokens for billing and downstream transparency.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS cache_reduction_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS cache_reduction_min_ratio DECIMAL(10,4) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_reduction_max_ratio DECIMAL(10,4) NOT NULL DEFAULT 0;

COMMENT ON COLUMN groups.cache_reduction_enabled IS
    'Whether to enable upstream cache hit reduction for OpenAI/Codex requests';
COMMENT ON COLUMN groups.cache_reduction_min_ratio IS
    'Minimum ratio to reduce cached tokens (0.0 to 1.0)';
COMMENT ON COLUMN groups.cache_reduction_max_ratio IS
    'Maximum ratio to reduce cached tokens (0.0 to 1.0)';
