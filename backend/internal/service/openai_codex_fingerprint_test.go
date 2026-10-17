package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCodexFingerprintSeed = "11111111-1111-4111-8111-111111111111"

func newTestOAuthAccount(id int64, extra map[string]any) *Account {
	if codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(extra)) {
		if extra == nil {
			extra = make(map[string]any)
		}
		if _, exists := extra[codexFingerprintSeedExtraKey]; !exists {
			extra[codexFingerprintSeedExtraKey] = testCodexFingerprintSeed
		}
	}
	return &Account{
		ID:       id,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    extra,
	}
}

// --- deriveStableUUIDv4 ---

func TestDeriveStableUUIDv4_Deterministic(t *testing.T) {
	a := deriveStableUUIDv4("test-seed-1")
	b := deriveStableUUIDv4("test-seed-1")
	assert.Equal(t, a, b, "同一种子应返回相同结果")
}

func TestDeriveStableUUIDv4_DifferentSeeds(t *testing.T) {
	a := deriveStableUUIDv4("seed-a")
	b := deriveStableUUIDv4("seed-b")
	assert.NotEqual(t, a, b, "不同种子应返回不同结果")
}

func TestDeriveStableUUIDv4_ValidFormat(t *testing.T) {
	result := deriveStableUUIDv4("test-seed")
	parsed, err := uuid.Parse(result)
	require.NoError(t, err, "应返回合法 UUID 格式")
	assert.Equal(t, uuid.Version(4), parsed.Version(), "应为 UUIDv4")
	assert.Equal(t, uuid.RFC4122, parsed.Variant(), "应为 RFC4122 变体")
}

// --- GetCodexFingerprintMode ---

func TestGetCodexFingerprintMode(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected codexFingerprintMode
	}{
		{"nil 账号", nil, codexFingerprintOff},
		{"非 OAuth 账号", &Account{Platform: PlatformOpenAI, Type: "api_key"}, codexFingerprintOff},
		{"OpenAI setup token", &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{codexFingerprintModeExtraKey: "session"}}, codexFingerprintSession},
		{"Anthropic setup token", &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken, Extra: map[string]any{codexFingerprintModeExtraKey: "session"}}, codexFingerprintOff},
		// 收敛是显式 opt-in：缺省/空/非法一律 off（#5610）。存量账号普遍没有这个
		// extra 键，升级不得把它们静默切进收敛。
		{"无 extra 默认 off", newTestOAuthAccount(1, nil), codexFingerprintOff},
		{"空值默认 off", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: ""}), codexFingerprintOff},
		{"非法值默认 off", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "invalid"}), codexFingerprintOff},
		{"显式 off", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "off"}), codexFingerprintOff},
		{"device", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "device"}), codexFingerprintDevice},
		{"session", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "session"}), codexFingerprintSession},
		{"full", newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "full"}), codexFingerprintFull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.account.GetCodexFingerprintMode())
		})
	}
}

// --- resolveConvergedInstallationID ---

func TestResolveConvergedInstallationID_UsesDeviceID(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{"openai_device_id": "real-device-id"})
	assert.Equal(t, "real-device-id", resolveConvergedInstallationID(account, testCodexFingerprintSeed))
}

func TestResolveConvergedInstallationID_DerivesFromSeed(t *testing.T) {
	account := newTestOAuthAccount(42, nil)
	result := resolveConvergedInstallationID(account, testCodexFingerprintSeed)
	_, err := uuid.Parse(result)
	require.NoError(t, err, "派生值应为合法 UUID")
	assert.Equal(t, result, resolveConvergedInstallationID(account, testCodexFingerprintSeed), "确定性")
}

func TestResolveConvergedInstallationID_DifferentSeeds(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	a := resolveConvergedInstallationID(account, testCodexFingerprintSeed)
	b := resolveConvergedInstallationID(account, "22222222-2222-4222-8222-222222222222")
	assert.NotEqual(t, a, b)
}

// --- resolveConvergedThreadID ---

func TestResolveConvergedThreadID_PerClientSession(t *testing.T) {
	a := resolveConvergedThreadID(testCodexFingerprintSeed, "session-aaa")
	b := resolveConvergedThreadID(testCodexFingerprintSeed, "session-bbb")
	assert.NotEqual(t, a, b, "不同客户端 session 应得到不同 thread_id")
}

func TestResolveConvergedThreadID_Deterministic(t *testing.T) {
	a := resolveConvergedThreadID(testCodexFingerprintSeed, "session-aaa")
	b := resolveConvergedThreadID(testCodexFingerprintSeed, "session-aaa")
	assert.Equal(t, a, b, "同一客户端 session 应得到相同 thread_id")
}

func TestResolveConvergedThreadID_EmptySession(t *testing.T) {
	assert.Equal(t, "", resolveConvergedThreadID(testCodexFingerprintSeed, ""))
}

// --- off 模式：resolveCodexFingerprintIDsFromRequest 返回 nil ---

func TestResolveCodexFingerprintIDsFromRequest_ExplicitOff(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "off"})
	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	assert.Nil(t, ids, "显式 off 模式应返回 nil")
}

// 未显式配置的存量账号不得被收敛（#5610）：默认返回 nil，出站身份保持
// v0.1.175 之前的客户端原值。
func TestResolveCodexFingerprintIDsFromRequest_DefaultIsOff(t *testing.T) {
	account := newTestOAuthAccount(1, nil)
	assert.Nil(t, resolveCodexFingerprintIDsFromRequest(account, nil), "无 extra 应视为 off")
}

// 管理员显式 opt-in 的账号行为不变。
func TestResolveCodexFingerprintIDsFromRequest_ExplicitOptInHonored(t *testing.T) {
	for _, mode := range []string{"device", "session", "full"} {
		t.Run(mode, func(t *testing.T) {
			account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: mode})
			ids := resolveCodexFingerprintIDsFromRequest(account, nil)
			require.NotNil(t, ids, "显式配置必须生效")
			assert.Equal(t, codexFingerprintMode(mode), ids.mode)
			assert.NotEmpty(t, ids.installationID)
		})
	}
}

func TestResolveCodexFingerprintIDsFromRequest_EnabledModesRequireValidSeed(t *testing.T) {
	for _, tt := range []struct {
		name  string
		extra map[string]any
	}{
		{name: "missing", extra: map[string]any{codexFingerprintModeExtraKey: "device"}},
		{name: "missing with device override", extra: map[string]any{codexFingerprintModeExtraKey: "device", "openai_device_id": "real-device"}},
		{name: "blank", extra: map[string]any{codexFingerprintModeExtraKey: "session", codexFingerprintSeedExtraKey: ""}},
		{name: "uppercase", extra: map[string]any{codexFingerprintModeExtraKey: "full", codexFingerprintSeedExtraKey: "11111111-1111-4111-8111-AAAAAAAAAAAA"}},
		{name: "nil uuid", extra: map[string]any{codexFingerprintModeExtraKey: "device", codexFingerprintSeedExtraKey: "00000000-0000-0000-0000-000000000000"}},
		{name: "non string", extra: map[string]any{codexFingerprintModeExtraKey: "session", codexFingerprintSeedExtraKey: 123}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: tt.extra}
			require.Nil(t, resolveCodexFingerprintIDsFromRequest(account, nil))
		})
	}
}

// --- applyCodexFingerprintHeaders: off 模式 ---

func TestApplyCodexFingerprintHeaders_OffMode(t *testing.T) {
	h := http.Header{}
	h.Set("x-codex-installation-id", "original-install-id")
	h.Set("x-codex-window-id", "original-window-id")

	applyCodexFingerprintHeaders(h, nil)

	assert.Equal(t, "original-install-id", h.Get("x-codex-installation-id"), "nil ids 不改写")
	assert.Equal(t, "original-window-id", h.Get("x-codex-window-id"), "nil ids 不改写")
}

// --- applyCodexFingerprintHeaders: device 模式 ---

func TestApplyCodexFingerprintHeaders_DeviceMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		"openai_device_id":           "converged-device",
	})
	turnMetadata := `{"installation_id":"user-install","session_id":"user-session","sandbox":"seccomp"}`
	h := http.Header{}
	h.Set("x-codex-installation-id", "user-install")
	h.Set("x-codex-window-id", "user-window:0")
	h.Set("x-codex-turn-metadata", turnMetadata)

	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	applyCodexFingerprintHeaders(h, ids)

	assert.Equal(t, "converged-device", h.Get("x-codex-installation-id"), "installation_id 应收敛")
	assert.Equal(t, "user-window:0", h.Get("x-codex-window-id"), "device 模式不改写 window_id")

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &meta))
	assert.Equal(t, "converged-device", meta["installation_id"])
	assert.Equal(t, "user-session", meta["session_id"], "device 模式不改写 session_id")
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

// --- applyCodexFingerprintHeaders: session 模式 ---

func TestApplyCodexFingerprintHeaders_SessionMode(t *testing.T) {
	ResetGlobalSlotRegistryForTest()
	account := newTestOAuthAccount(101, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-aaa")

	turnMetadata := `{"installation_id":"user-install","session_id":"user-session","thread_id":"user-thread","turn_id":"user-turn","window_id":"user-thread:0","sandbox":"seccomp","thread_source":"user"}`
	h := http.Header{}
	h.Set("x-codex-installation-id", "user-install")
	h.Set("x-codex-window-id", "user-thread:0")
	h.Set("x-codex-turn-metadata", turnMetadata)
	h.Set("x-client-request-id", "user-thread")
	h.Set("conversation_id", "user-conversation")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	applyCodexFingerprintHeaders(h, ids)

	seed, ok := codexFingerprintSeed(account.Extra)
	require.True(t, ok)
	convergedInstall := resolveConvergedInstallationID(account, seed)
	convergedSession := resolveConvergedSessionID(seed)
	convergedThread := resolveConvergedThreadID(seed, "client-session-aaa")

	assert.Equal(t, convergedInstall, h.Get("x-codex-installation-id"))
	assert.Equal(t, convergedSession, h.Get("session-id"))
	assert.Equal(t, convergedSession, h.Get("session_id"), "下划线形式也应被改写")
	assert.Equal(t, convergedSession, h.Get("conversation_id"), "conversation_id 也应被改写且与 sessionID 一致")
	assert.Equal(t, convergedThread, h.Get("thread-id"))
	assert.Equal(t, convergedThread, h.Get("x-client-request-id"))
	assert.Equal(t, convergedThread+":0", h.Get("x-codex-window-id"))

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &meta))
	assert.Equal(t, convergedInstall, meta["installation_id"])
	assert.Equal(t, convergedSession, meta["session_id"])
	assert.Equal(t, convergedThread, meta["thread_id"])
	assert.NotEqual(t, "user-turn", meta["turn_id"], "turn_id 应被新生成的值替换")
	assert.Equal(t, "seccomp", meta["sandbox"], "sandbox 保留原样")
	assert.Equal(t, "user", meta["thread_source"], "thread_source 保留原样")
}

// --- applyCodexFingerprintHeaders: conversation_id 收敛改写 ---

func TestApplyCodexFingerprintHeaders_ConversationID_Rewritten(t *testing.T) {
	ResetGlobalSlotRegistryForTest()
	account := newTestOAuthAccount(102, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	seed, ok := codexFingerprintSeed(account.Extra)
	require.True(t, ok)
	convergedSession := resolveConvergedSessionID(seed)

	t.Run("客户端携带原始 conversation_id 时应被强制收敛为 sessionID", func(t *testing.T) {
		ResetGlobalSlotRegistryForTest()
		clientHeaders := http.Header{}
		clientHeaders.Set("session-id", "client-session-123")
		ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
		require.NotNil(t, ids)

		h := http.Header{}
		h.Set("conversation_id", "user-original-conversation-uuid")
		h.Set("session_id", "user-original-session")
		h.Set("session-id", "user-original-session")

		applyCodexFingerprintHeaders(h, ids)

		assert.Equal(t, convergedSession, h.Get("conversation_id"))
		assert.Equal(t, convergedSession, h.Get("session-id"))
		assert.Equal(t, convergedSession, h.Get("session_id"))
		assert.Equal(t, h.Get("session-id"), h.Get("conversation_id"), "conversation_id 必须与 session-id 完全一致")
	})

	t.Run("客户端未携带 conversation_id 时不凭空捏造该头（保持官方CLI纯净度）", func(t *testing.T) {
		ResetGlobalSlotRegistryForTest()
		clientHeaders := http.Header{}
		clientHeaders.Set("session-id", "client-session-456")
		ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
		require.NotNil(t, ids)

		h := http.Header{}
		applyCodexFingerprintHeaders(h, ids)

		assert.Empty(t, h.Get("conversation_id"), "纯净 CLI 流量不应包含 Web 端的 conversation_id")
		assert.Equal(t, convergedSession, h.Get("session-id"))
		assert.Equal(t, convergedSession, h.Get("session_id"))
	})
}

// --- rewriteCodexTurnMetadataFields: 自动初始化与注入 ---

func TestRewriteCodexTurnMetadataFields_EmptyHeaderInjected(t *testing.T) {
	t.Run("头完全不存在时自动初始化并注入 JSON", func(t *testing.T) {
		h := http.Header{}
		fields := map[string]any{
			"installation_id":         "inst-1",
			"session_id":              "sess-1",
			"thread_id":               "th-1",
			"turn_id":                 "turn-1",
			"window_id":               "win-1",
			"turn_started_at_unix_ms": int64(1700000000000),
		}

		rewriteCodexTurnMetadataFields(h, fields)

		raw := h.Get("x-codex-turn-metadata")
		require.NotEmpty(t, raw, "未带头时应自动初始化并注入 x-codex-turn-metadata")

		var meta map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &meta))
		assert.Equal(t, "inst-1", meta["installation_id"])
		assert.Equal(t, "sess-1", meta["session_id"])
		assert.Equal(t, "th-1", meta["thread_id"])
		assert.Equal(t, "turn-1", meta["turn_id"])
		assert.Equal(t, "win-1", meta["window_id"])
		assert.Equal(t, float64(1700000000000), meta["turn_started_at_unix_ms"])
	})

	t.Run("头为空字符串或纯空白时自动初始化并注入 JSON", func(t *testing.T) {
		for _, emptyVal := range []string{"", "   ", "\t\n"} {
			h := http.Header{}
			h.Set("x-codex-turn-metadata", emptyVal)
			fields := map[string]any{
				"installation_id": "inst-2",
				"session_id":      "sess-2",
			}

			rewriteCodexTurnMetadataFields(h, fields)

			raw := h.Get("x-codex-turn-metadata")
			require.NotEmpty(t, raw)

			var meta map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &meta))
			assert.Equal(t, "inst-2", meta["installation_id"])
			assert.Equal(t, "sess-2", meta["session_id"])
		}
	})

	t.Run("头为非法 JSON 格式时回退并以 fields 自动初始化注入", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-turn-metadata", "{invalid-json-body")
		fields := map[string]any{
			"installation_id": "inst-3",
			"session_id":      "sess-3",
		}

		rewriteCodexTurnMetadataFields(h, fields)

		raw := h.Get("x-codex-turn-metadata")
		require.NotEmpty(t, raw)

		var meta map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &meta))
		assert.Equal(t, "inst-3", meta["installation_id"])
		assert.Equal(t, "sess-3", meta["session_id"])
	})

	t.Run("头已存在有效 JSON 时保留既有非指定字段并覆写指定字段", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-turn-metadata", `{"installation_id":"old-inst","sandbox":"seatbelt","custom_field":123}`)
		fields := map[string]any{
			"installation_id": "new-inst",
			"session_id":      "new-sess",
		}

		rewriteCodexTurnMetadataFields(h, fields)

		raw := h.Get("x-codex-turn-metadata")
		var meta map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &meta))
		assert.Equal(t, "new-inst", meta["installation_id"], "指定字段应被覆写")
		assert.Equal(t, "new-sess", meta["session_id"], "指定字段应被注入")
		assert.Equal(t, "seatbelt", meta["sandbox"], "既有字段应保留")
		assert.Equal(t, float64(123), meta["custom_field"], "既有字段应保留")
	})
}

// --- applyCodexFingerprintHeaders 在原请求未携带 turn-metadata 时的端到端改写 ---

func TestApplyCodexFingerprintHeaders_InitsTurnMetadataWhenMissing(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-no-metadata")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	// 原请求完全不带 x-codex-turn-metadata 也不带 conversation_id
	h := http.Header{}
	h.Set("session-id", "client-no-metadata")

	applyCodexFingerprintHeaders(h, ids)

	// 1. 未携带 conversation_id 时不凭空添加该头
	assert.Empty(t, h.Get("conversation_id"), "原请求未带 conversation_id 时不应凭空注入")
	assert.Equal(t, ids.sessionID, h.Get("session-id"))
	assert.Equal(t, ids.sessionID, h.Get("session_id"))

	// 2. x-codex-turn-metadata 头应被自动初始化并包含完整的 turn metadata 字段
	rawMeta := h.Get("x-codex-turn-metadata")
	require.NotEmpty(t, rawMeta, "原请求缺少 x-codex-turn-metadata 时应由 applyCodexFingerprintHeaders 自动初始化并注入")

	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(rawMeta), &meta))
	assert.Equal(t, ids.installationID, meta["installation_id"])
	assert.Equal(t, ids.sessionID, meta["session_id"])
	assert.Equal(t, ids.threadID, meta["thread_id"])
	assert.Equal(t, ids.turnID, meta["turn_id"])
	assert.Equal(t, ids.windowID, meta["window_id"])
	assert.Equal(t, float64(ids.turnStartedAtUnixMs), meta["turn_started_at_unix_ms"])
}

// --- device 模式：原请求无 turn metadata 头时不凭空捏造 ---

func TestApplyCodexFingerprintHeaders_DeviceMode_DoesNotInjectPartialTurnMetadata(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
	})
	seed, ok := codexFingerprintSeed(account.Extra)
	require.True(t, ok)
	convergedInstall := resolveConvergedInstallationID(account, seed)

	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-device-sess")
	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	t.Run("原请求无 x-codex-turn-metadata 时绝不凭空注入残缺头", func(t *testing.T) {
		h := http.Header{}
		applyCodexFingerprintHeaders(h, ids)

		assert.Equal(t, convergedInstall, h.Get("x-codex-installation-id"))
		assert.Empty(t, h.Get("x-codex-turn-metadata"), "device 模式下原请求无 turn metadata 头时绝不凭空注入畸形头")
	})

	t.Run("原请求已有合法 turn metadata 时仅改写 installation_id 并保留其他字段", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-turn-metadata", `{"installation_id":"raw","session_id":"raw-sess","turn_id":"raw-turn"}`)
		applyCodexFingerprintHeaders(h, ids)

		assert.Equal(t, convergedInstall, h.Get("x-codex-installation-id"))
		rawMeta := h.Get("x-codex-turn-metadata")
		require.NotEmpty(t, rawMeta)

		var meta map[string]any
		require.NoError(t, json.Unmarshal([]byte(rawMeta), &meta))
		assert.Equal(t, convergedInstall, meta["installation_id"])
		assert.Equal(t, "raw-sess", meta["session_id"], "device 模式保留原 session_id")
		assert.Equal(t, "raw-turn", meta["turn_id"], "device 模式保留原 turn_id")
	})
}

// --- session 模式：不同客户端得到不同 thread ---

func TestApplyCodexFingerprintHeaders_SessionMode_DifferentClients(t *testing.T) {
	ResetGlobalSlotRegistryForTest()
	account := newTestOAuthAccount(103, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})

	makeTurnMeta := func() string {
		return `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`
	}

	clientA := http.Header{}
	clientA.Set("session-id", "client-A")
	idsA := resolveCodexFingerprintIDsFromRequest(account, clientA)
	if idsA != nil && idsA.lease != nil {
		idsA.lease.Release()
	}
	hA := http.Header{}
	hA.Set("x-codex-turn-metadata", makeTurnMeta())
	hA.Set("conversation_id", "client-A-conv")
	applyCodexFingerprintHeaders(hA, idsA)

	clientB := http.Header{}
	clientB.Set("session-id", "client-B")
	idsB := resolveCodexFingerprintIDsFromRequest(account, clientB)
	if idsB != nil && idsB.lease != nil {
		idsB.lease.Release()
	}
	hB := http.Header{}
	hB.Set("x-codex-turn-metadata", makeTurnMeta())
	hB.Set("conversation_id", "client-B-conv")
	applyCodexFingerprintHeaders(hB, idsB)

	assert.Equal(t, hA.Get("session-id"), hB.Get("session-id"), "session_id 应相同")
	assert.Equal(t, hA.Get("session-id"), hA.Get("conversation_id"), "A 的 conversation_id 应与 session-id 一致")
	assert.Equal(t, hB.Get("session-id"), hB.Get("conversation_id"), "B 的 conversation_id 应与 session-id 一致")
	assert.NotEqual(t, hA.Get("thread-id"), hB.Get("thread-id"), "不同客户端 thread_id 应不同")
	assert.NotEqual(t, hA.Get("x-codex-window-id"), hB.Get("x-codex-window-id"), "不同客户端 window_id 应不同")
	assert.Equal(t, hA.Get("x-codex-installation-id"), hB.Get("x-codex-installation-id"))
}

// --- full 模式 ---

func TestApplyCodexFingerprintHeaders_FullMode(t *testing.T) {
	ResetGlobalSlotRegistryForTest()
	account := newTestOAuthAccount(104, map[string]any{
		codexFingerprintModeExtraKey: "full",
	})
	seed, ok := codexFingerprintSeed(account.Extra)
	require.True(t, ok)
	convergedSession := resolveConvergedSessionID(seed)

	clientA := http.Header{}
	clientA.Set("session-id", "client-A")
	idsA := resolveCodexFingerprintIDsFromRequest(account, clientA)
	if idsA != nil && idsA.lease != nil {
		idsA.lease.Release()
	}
	hA := http.Header{}
	hA.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	hA.Set("conversation_id", "client-A-conv")
	applyCodexFingerprintHeaders(hA, idsA)

	clientB := http.Header{}
	clientB.Set("session-id", "client-B")
	idsB := resolveCodexFingerprintIDsFromRequest(account, clientB)
	if idsB != nil && idsB.lease != nil {
		idsB.lease.Release()
	}
	hB := http.Header{}
	hB.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	hB.Set("conversation_id", "client-B-conv")
	applyCodexFingerprintHeaders(hB, idsB)

	assert.Equal(t, hA.Get("thread-id"), hB.Get("thread-id"), "full 模式 thread_id 应相同")
	assert.Equal(t, convergedSession, hA.Get("thread-id"), "full 模式 thread_id 应等于 session_id")
	assert.Equal(t, convergedSession, hA.Get("conversation_id"), "full 模式 conversation_id 应等于 session_id")
	assert.Equal(t, convergedSession, hB.Get("conversation_id"), "full 模式 conversation_id 应等于 session_id")
	assert.Equal(t, hA.Get("x-codex-window-id"), hB.Get("x-codex-window-id"), "full 模式 window_id 应相同")
}

// --- H1 修复验证：头和体的 turn_id 一致性 ---

func TestFingerprintIDs_HeaderAndBody_TurnID_Consistent(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-xyz")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	// 头改写
	h := http.Header{}
	h.Set("x-codex-turn-metadata", `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`)
	applyCodexFingerprintHeaders(h, ids)

	// 体改写（使用同一份 ids）
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "x",
			"session_id":              "x",
			"turn_id":                 "x",
			"x-codex-turn-metadata":   `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`,
		},
	}
	applyCodexFingerprintClientMetadata(reqBody, ids)

	// 从头 turn-metadata JSON 提取 turn_id
	var headerMeta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &headerMeta))
	headerTurnID, ok := headerMeta["turn_id"].(string)
	require.True(t, ok, "头 turn-metadata 应包含 string 类型的 turn_id")

	// 从体 client_metadata 提取 turn_id
	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok, "请求体应包含 client_metadata")
	bodyTurnID, ok := cm["turn_id"].(string)
	require.True(t, ok, "体 client_metadata 应包含 string 类型的 turn_id")

	// 从体内嵌 turn-metadata JSON 提取 turn_id
	embeddedRaw, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok, "体 client_metadata 应包含 x-codex-turn-metadata 字符串")
	var bodyMeta map[string]any
	require.NoError(t, json.Unmarshal([]byte(embeddedRaw), &bodyMeta))
	bodyEmbeddedTurnID, ok := bodyMeta["turn_id"].(string)
	require.True(t, ok, "体内嵌 turn-metadata 应包含 string 类型的 turn_id")

	assert.Equal(t, headerTurnID, bodyTurnID, "头和体的 turn_id 必须一致")
	assert.Equal(t, headerTurnID, bodyEmbeddedTurnID, "头和体内嵌 turn-metadata 的 turn_id 必须一致")
	assert.Equal(t, ids.turnID, headerTurnID, "所有 turn_id 都应来自同一份 ids")
	assert.Equal(t, headerMeta["turn_started_at_unix_ms"], bodyMeta["turn_started_at_unix_ms"], "头和体的 timestamp 必须一致")
	assert.Equal(t, float64(ids.turnStartedAtUnixMs), headerMeta["turn_started_at_unix_ms"])
}

func TestFingerprintIDs_MalformedEmbeddedMetadataRebuiltConsistently(t *testing.T) {
	account := newTestOAuthAccount(2, map[string]any{codexFingerprintModeExtraKey: "session"})
	clientHeaders := make(http.Header)
	clientHeaders.Set("session-id", "client-session-malformed")
	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	h := make(http.Header)
	h.Set("x-codex-turn-metadata", "{malformed")
	applyCodexFingerprintHeaders(h, ids)

	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"session_id":            "client-session-malformed",
			"x-codex-turn-metadata": "[malformed",
		},
	}
	require.True(t, applyCodexFingerprintClientMetadata(reqBody, ids))

	var headerMeta map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.Get("x-codex-turn-metadata")), &headerMeta))
	clientMetadata, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	bodyRaw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var bodyMeta map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyRaw), &bodyMeta))

	for _, key := range []string{"installation_id", "session_id", "thread_id", "turn_id", "window_id", "turn_started_at_unix_ms"} {
		assert.Equal(t, headerMeta[key], bodyMeta[key], "rebuilt metadata field %s must match", key)
	}
}

// --- applyCodexFingerprintClientMetadata ---

func TestApplyCodexFingerprintClientMetadata_OffMode(t *testing.T) {
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original",
		},
	}
	modified := applyCodexFingerprintClientMetadata(reqBody, nil)
	assert.False(t, modified, "nil ids 不改写")
}

func TestApplyCodexFingerprintClientMetadata_DeviceMode(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{
		codexFingerprintModeExtraKey: "device",
		"openai_device_id":           "converged-device",
	})
	ids := resolveCodexFingerprintIDsFromRequest(account, nil)
	require.NotNil(t, ids)

	embeddedMeta := `{"installation_id":"x","session_id":"user-session","sandbox":"seccomp"}`
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original-install",
			"session_id":              "user-session",
			"x-codex-turn-metadata":   embeddedMeta,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "converged-device", cm["x-codex-installation-id"])
	assert.Equal(t, "user-session", cm["session_id"], "device 模式不改 session_id")

	turnMetaStr, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(turnMetaStr), &meta))
	assert.Equal(t, "converged-device", meta["installation_id"])
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

func TestApplyCodexFingerprintClientMetadata_SessionMode(t *testing.T) {
	ResetGlobalSlotRegistryForTest()
	account := newTestOAuthAccount(105, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "client-session-aaa")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	embeddedMeta := `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0","sandbox":"seccomp"}`
	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"x-codex-installation-id": "original-install",
			"session_id":              "original-session",
			"x-codex-turn-metadata":   embeddedMeta,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	seed, ok := codexFingerprintSeed(account.Extra)
	require.True(t, ok)
	convergedInstall := resolveConvergedInstallationID(account, seed)
	convergedSession := resolveConvergedSessionID(seed)
	convergedThread := resolveConvergedThreadID(seed, "client-session-aaa")

	assert.Equal(t, convergedInstall, cm["x-codex-installation-id"])
	assert.Equal(t, convergedSession, cm["session_id"])
	assert.Equal(t, convergedThread, cm["thread_id"])
	assert.Equal(t, convergedThread+":0", cm["x-codex-window-id"])

	turnMetaStr, ok := cm["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	var meta map[string]any
	require.NoError(t, json.Unmarshal([]byte(turnMetaStr), &meta))
	assert.Equal(t, convergedInstall, meta["installation_id"])
	assert.Equal(t, convergedSession, meta["session_id"])
	assert.Equal(t, "seccomp", meta["sandbox"], "非指纹字段保留原样")
}

func TestApplyCodexFingerprintClientMetadata_FullMode(t *testing.T) {
	ResetGlobalSlotRegistryForTest()
	account := newTestOAuthAccount(106, map[string]any{
		codexFingerprintModeExtraKey: "full",
	})
	clientHeaders := http.Header{}
	clientHeaders.Set("session-id", "any-client")

	ids := resolveCodexFingerprintIDsFromRequest(account, clientHeaders)
	require.NotNil(t, ids)

	reqBody := map[string]any{
		"client_metadata": map[string]any{
			"session_id":            "x",
			"thread_id":             "x",
			"x-codex-turn-metadata": `{"installation_id":"x","session_id":"x","thread_id":"x","turn_id":"x","window_id":"x:0"}`,
		},
	}

	modified := applyCodexFingerprintClientMetadata(reqBody, ids)
	require.True(t, modified)

	cm, ok := reqBody["client_metadata"].(map[string]any)
	require.True(t, ok)
	seed, ok := codexFingerprintSeed(account.Extra)
	require.True(t, ok)
	convergedSession := resolveConvergedSessionID(seed)

	assert.Equal(t, convergedSession, cm["session_id"])
	assert.Equal(t, convergedSession, cm["thread_id"], "full 模式 thread_id 应等于 session_id")
}

// --- extractClientSessionID ---

func TestExtractClientSessionID(t *testing.T) {
	tests := []struct {
		name     string
		headers  http.Header
		expected string
	}{
		{"连字符形式优先", func() http.Header {
			h := http.Header{}
			h.Set("session-id", "hyphen-form")
			h.Set("session_id", "underscore-form")
			return h
		}(), "hyphen-form"},
		{"回退到下划线形式", func() http.Header {
			h := http.Header{}
			h.Set("session_id", "underscore-form")
			return h
		}(), "underscore-form"},
		{"都没有", http.Header{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractClientSessionID(tt.headers))
		})
	}
}

// --- 透传路径：raw 字节版 client_metadata 改写 ---

// rawVsMapClientMetadata 用同一份 ids 分别跑 map 版与 raw 字节版，
// 返回两侧最终的 client_metadata 解码结果。
func rawVsMapClientMetadata(t *testing.T, body []byte, ids *codexFingerprintIDs) (map[string]any, map[string]any) {
	t.Helper()

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	applyCodexFingerprintClientMetadata(decoded, ids)
	mapCM, _ := decoded["client_metadata"].(map[string]any)
	// 归一化 map 版中的数字类型为 float64，保证与 JSON 反序列化结果具有相同的类型语义
	for k, v := range mapCM {
		if i, ok := v.(int); ok {
			mapCM[k] = float64(i)
		}
	}

	rawBody, changed, err := applyCodexFingerprintClientMetadataRaw(body, ids)
	require.NoError(t, err)
	require.True(t, changed)
	var rawDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawBody, &rawDecoded))
	rawCM, _ := rawDecoded["client_metadata"].(map[string]any)
	return mapCM, rawCM
}

func cloneCodexFingerprintIDsForTest(ids *codexFingerprintIDs) *codexFingerprintIDs {
	if ids == nil {
		return nil
	}
	cloned := *ids
	cloned.originalBodySessionID = ""
	cloned.originalBodySessionIDCaptured = false
	return &cloned
}

func applyMapAndRawFingerprintBodiesForTest(t *testing.T, body []byte, ids *codexFingerprintIDs) (map[string]any, map[string]any) {
	t.Helper()

	mapIDs := cloneCodexFingerprintIDsForTest(ids)
	rawIDs := cloneCodexFingerprintIDsForTest(ids)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	applyCodexFingerprintClientMetadata(decoded, mapIDs)

	rawBody, _, err := applyCodexFingerprintClientMetadataRaw(body, rawIDs)
	require.NoError(t, err)
	var rawDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawBody, &rawDecoded))
	return decoded, rawDecoded
}

func TestApplyCodexFingerprintPromptCacheKey_MapRawEquivalence(t *testing.T) {
	for _, mode := range []codexFingerprintMode{codexFingerprintSession, codexFingerprintFull} {
		t.Run(string(mode)+"/default", func(t *testing.T) {
			account := newTestOAuthAccount(4300, map[string]any{codexFingerprintModeExtraKey: string(mode)})
			ids := resolveCodexFingerprintIDs(account, "header-session", mode)
			require.NotNil(t, ids)

			body := []byte(`{"model":"gpt-5.6-sol","prompt_cache_key":"body-session","client_metadata":{"session_id":" body-session ","trace":"keep"},"input":[]}`)
			mapBody, rawBody := applyMapAndRawFingerprintBodiesForTest(t, body, ids)

			require.Equal(t, mapBody["prompt_cache_key"], rawBody["prompt_cache_key"])
			require.Equal(t, ids.sessionID, mapBody["prompt_cache_key"])
			mapCM, _ := mapBody["client_metadata"].(map[string]any)
			rawCM, _ := rawBody["client_metadata"].(map[string]any)
			require.Equal(t, ids.sessionID, mapCM["session_id"])
			require.Equal(t, mapCM["session_id"], rawCM["session_id"])
			require.Equal(t, "keep", rawCM["trace"])
		})
	}

	t.Run("explicit override", func(t *testing.T) {
		account := newTestOAuthAccount(4301, map[string]any{codexFingerprintModeExtraKey: "session"})
		ids := resolveCodexFingerprintIDs(account, "header-session", codexFingerprintSession)
		require.NotNil(t, ids)

		body := []byte(`{"model":"gpt-5.6-sol","prompt_cache_key":"explicit-cache","client_metadata":{"session_id":"body-session"},"input":[]}`)
		mapBody, rawBody := applyMapAndRawFingerprintBodiesForTest(t, body, ids)

		require.Equal(t, "explicit-cache", mapBody["prompt_cache_key"])
		require.Equal(t, "explicit-cache", rawBody["prompt_cache_key"])
		mapCM, _ := mapBody["client_metadata"].(map[string]any)
		rawCM, _ := rawBody["client_metadata"].(map[string]any)
		require.Equal(t, ids.sessionID, mapCM["session_id"])
		require.Equal(t, ids.sessionID, rawCM["session_id"])
	})
}

func TestApplyCodexFingerprintPromptCacheKey_Negatives(t *testing.T) {
	sessionAccount := newTestOAuthAccount(4310, map[string]any{codexFingerprintModeExtraKey: "session"})
	sessionIDs := resolveCodexFingerprintIDs(sessionAccount, "header-session", codexFingerprintSession)
	require.NotNil(t, sessionIDs)
	deviceAccount := newTestOAuthAccount(4311, map[string]any{codexFingerprintModeExtraKey: "device"})
	deviceIDs := resolveCodexFingerprintIDs(deviceAccount, "header-session", codexFingerprintDevice)
	require.NotNil(t, deviceIDs)

	tests := []struct {
		name          string
		body          []byte
		ids           *codexFingerprintIDs
		wantExists    bool
		wantCacheKey  any
		wantRawString string
	}{
		{
			name:       "missing key is not injected",
			body:       []byte(`{"client_metadata":{"session_id":"body-session"}}`),
			ids:        sessionIDs,
			wantExists: false,
		},
		{
			name:         "empty key preserved",
			body:         []byte(`{"prompt_cache_key":"","client_metadata":{"session_id":"body-session"}}`),
			ids:          sessionIDs,
			wantExists:   true,
			wantCacheKey: "",
		},
		{
			name:         "whitespace-different key is an explicit override",
			body:         []byte(`{"prompt_cache_key":" body-session ","client_metadata":{"session_id":"body-session"}}`),
			ids:          sessionIDs,
			wantExists:   true,
			wantCacheKey: " body-session ",
		},
		{
			name:         "non-string key preserved",
			body:         []byte(`{"prompt_cache_key":123,"client_metadata":{"session_id":"body-session"}}`),
			ids:          sessionIDs,
			wantExists:   true,
			wantCacheKey: float64(123),
		},
		{
			name:         "missing source metadata preserves key",
			body:         []byte(`{"prompt_cache_key":"body-session"}`),
			ids:          sessionIDs,
			wantExists:   true,
			wantCacheKey: "body-session",
		},
		{
			name:         "non-string source session preserves key",
			body:         []byte(`{"prompt_cache_key":"123","client_metadata":{"session_id":123}}`),
			ids:          sessionIDs,
			wantExists:   true,
			wantCacheKey: "123",
		},
		{
			name:         "non-object source metadata preserves key",
			body:         []byte(`{"prompt_cache_key":"body-session","client_metadata":"bad"}`),
			ids:          sessionIDs,
			wantExists:   true,
			wantCacheKey: "body-session",
		},
		{
			name:         "device mode preserves key",
			body:         []byte(`{"prompt_cache_key":"body-session","client_metadata":{"session_id":"body-session"}}`),
			ids:          deviceIDs,
			wantExists:   true,
			wantCacheKey: "body-session",
		},
		{
			name:          "off mode preserves body",
			body:          []byte(`{"prompt_cache_key":"body-session","client_metadata":{"session_id":"body-session"}}`),
			ids:           nil,
			wantExists:    true,
			wantCacheKey:  "body-session",
			wantRawString: `{"prompt_cache_key":"body-session","client_metadata":{"session_id":"body-session"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mapBody map[string]any
			require.NoError(t, json.Unmarshal(tt.body, &mapBody))
			changedMap := applyCodexFingerprintClientMetadata(mapBody, cloneCodexFingerprintIDsForTest(tt.ids))

			rawBody, changedRaw, err := applyCodexFingerprintClientMetadataRaw(tt.body, cloneCodexFingerprintIDsForTest(tt.ids))
			require.NoError(t, err)
			if tt.ids == nil {
				require.False(t, changedMap)
				require.False(t, changedRaw)
				require.JSONEq(t, tt.wantRawString, string(rawBody))
				return
			}
			require.True(t, changedMap)
			require.True(t, changedRaw)

			rawDecoded := map[string]any{}
			require.NoError(t, json.Unmarshal(rawBody, &rawDecoded))
			_, mapExists := mapBody["prompt_cache_key"]
			_, rawExists := rawDecoded["prompt_cache_key"]
			require.Equal(t, tt.wantExists, mapExists)
			require.Equal(t, tt.wantExists, rawExists)
			if tt.wantExists {
				require.Equal(t, tt.wantCacheKey, mapBody["prompt_cache_key"])
				require.Equal(t, tt.wantCacheKey, rawDecoded["prompt_cache_key"])
			}
		})
	}
}

func TestApplyCodexFingerprintClientMetadataRaw_MatchesMapVariant(t *testing.T) {
	embedded := `{\"installation_id\":\"real-install\",\"session_id\":\"real-session\",\"sandbox\":\"seatbelt\"}`
	bodies := map[string]string{
		"no_client_metadata": `{"model":"gpt-5.6-sol","input":[],"stream":true}`,
		"object_with_extras": `{"model":"gpt-5.6-sol","client_metadata":{"session_id":"client-session","traceparent":"00-abc-def-01","x-codex-turn-metadata":"` + embedded + `"},"stream":true}`,
		"non_object_value":   `{"model":"gpt-5.6-sol","client_metadata":"bogus","stream":true}`,
	}
	for _, mode := range []codexFingerprintMode{codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull} {
		account := newTestOAuthAccount(4242, map[string]any{codexFingerprintModeExtraKey: string(mode)})
		ids := resolveCodexFingerprintIDs(account, "client-sess-raw", mode)
		require.NotNil(t, ids)
		for name, body := range bodies {
			t.Run(string(mode)+"/"+name, func(t *testing.T) {
				mapCM, rawCM := rawVsMapClientMetadata(t, []byte(body), ids)
				assert.Equal(t, mapCM, rawCM, "raw 字节版与 map 版的 client_metadata 结果必须逐点一致")
			})
		}
	}
}

func TestApplyCodexFingerprintClientMetadataRaw_PreservesUnrelatedFields(t *testing.T) {
	account := newTestOAuthAccount(4243, map[string]any{codexFingerprintModeExtraKey: "session"})
	ids := resolveCodexFingerprintIDs(account, "client-sess-preserve", codexFingerprintSession)
	require.NotNil(t, ids)

	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"hi"}],"stream":true,"prompt_cache_key":"pck-1"}`)
	out, changed, err := applyCodexFingerprintClientMetadataRaw(body, ids)
	require.NoError(t, err)
	require.True(t, changed)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out, &decoded))
	assert.Equal(t, "gpt-5.6-sol", decoded["model"])
	assert.Equal(t, "pck-1", decoded["prompt_cache_key"])
	assert.Equal(t, true, decoded["stream"])
	cm, _ := decoded["client_metadata"].(map[string]any)
	require.NotNil(t, cm)
	assert.Equal(t, ids.sessionID, cm["session_id"])
	assert.Equal(t, ids.turnID, cm["turn_id"])
}

func TestApplyCodexFingerprintClientMetadataRaw_Noop(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol"}`)
	out, changed, err := applyCodexFingerprintClientMetadataRaw(body, nil)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, body, out)

	out, changed, err = applyCodexFingerprintClientMetadataRaw(nil, &codexFingerprintIDs{mode: codexFingerprintSession, installationID: "x"})
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Nil(t, out)
}

// --- context 暂存与出站头应用（透传/非透传共用 seam）---

func newFingerprintStageTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

func TestStageCodexFingerprintIDs_NilOverwritesPreviousAccount(t *testing.T) {
	c := newFingerprintStageTestContext(t)
	accountA := newTestOAuthAccount(1001, map[string]any{codexFingerprintModeExtraKey: "session"})
	idsA := resolveCodexFingerprintIDs(accountA, "sess-x", codexFingerprintSession)
	require.NotNil(t, idsA)
	stageCodexFingerprintIDs(c, idsA)

	// failover 切到 off 模式账号：无条件覆写为 nil，上一账号 IDs 不得残留
	stageCodexFingerprintIDs(c, nil)

	h := http.Header{}
	h.Set("session_id", "isolated-session")
	accountB := newTestOAuthAccount(1002, map[string]any{"codex_fingerprint_mode": "off"})
	applyStagedCodexFingerprintHeaders(c, accountB, h)
	assert.Equal(t, "isolated-session", h.Get("session_id"), "off 账号不得应用上一账号的收敛 ID")
	assert.Empty(t, h.Get("x-codex-installation-id"))
}

func TestApplyStagedCodexFingerprintRejectsDifferentOAuthAccount(t *testing.T) {
	c := newFingerprintStageTestContext(t)
	accountA := newTestOAuthAccount(1003, map[string]any{codexFingerprintModeExtraKey: "session"})
	idsA := resolveCodexFingerprintIDs(accountA, "sess-a", codexFingerprintSession)
	require.NotNil(t, idsA)
	stageCodexFingerprintIDs(c, idsA)

	accountB := newTestOAuthAccount(1004, map[string]any{codexFingerprintModeExtraKey: "session"})
	h := make(http.Header)
	h.Set("session-id", "account-b-session")
	applyStagedCodexFingerprintHeaders(c, accountB, h)
	assert.Equal(t, "account-b-session", h.Get("session-id"))
	assert.Empty(t, h.Get("x-codex-installation-id"))

	body := map[string]any{"client_metadata": map[string]any{"session_id": "account-b-session"}}
	assert.False(t, applyStagedCodexFingerprintClientMetadata(c, accountB, body))
	clientMetadata, ok := body["client_metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "account-b-session", clientMetadata["session_id"])
}

func TestApplyStagedCodexFingerprintHeaders_SkipsNonOAuthAccount(t *testing.T) {
	c := newFingerprintStageTestContext(t)
	oauthIDs := resolveCodexFingerprintIDs(newTestOAuthAccount(1003, map[string]any{codexFingerprintModeExtraKey: "session"}), "sess-y", codexFingerprintSession)
	require.NotNil(t, oauthIDs)
	stageCodexFingerprintIDs(c, oauthIDs)

	h := http.Header{}
	apiKeyAccount := &Account{ID: 1004, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	applyStagedCodexFingerprintHeaders(c, apiKeyAccount, h)
	assert.Empty(t, h.Get("x-codex-installation-id"), "stale 收敛 ID 不得应用到非 OAuth 账号")
}

func TestBuildUpstreamRequestOpenAIPassthrough_AppliesStagedFingerprint(t *testing.T) {
	svc := &OpenAIGatewayService{}
	// 收敛是显式 opt-in（#5610）：显式开启后验证透传路径的出站头收敛。
	account := newTestOAuthAccount(2001, map[string]any{
		"openai_oauth_passthrough": true,
		"codex_fingerprint_mode":   "session",
	})

	c := newFingerprintStageTestContext(t)
	c.Request.Header.Set("session_id", "real-client-session")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color")
	c.Request.Header.Set("originator", "codex_cli_rs")
	c.Request.Header.Set("x-codex-turn-metadata", `{"installation_id":"real-install","session_id":"real-session","sandbox":"seatbelt"}`)

	// 复刻 forwardOpenAIPassthrough 的解析+暂存 seam（默认 session 模式）
	ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
	require.NotNil(t, ids)
	stageCodexFingerprintIDs(c, ids)

	body := []byte(`{"model":"gpt-5.6-sol","input":[],"stream":true}`)
	req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test-token")
	require.NoError(t, err)

	assert.Equal(t, ids.sessionID, req.Header.Get("session_id"), "session 模式下出站 session_id 应为账号级收敛值")
	assert.Equal(t, ids.sessionID, req.Header.Get("conversation_id"), "session 模式下出站 conversation_id 应与 session_id 保持绝对一致")
	assert.Equal(t, ids.installationID, req.Header.Get("x-codex-installation-id"))
	assert.Equal(t, ids.windowID, req.Header.Get("x-codex-window-id"))
	assert.Equal(t, ids.threadID, req.Header.Get("x-client-request-id"))
	turnMetadata := req.Header.Get("x-codex-turn-metadata")
	require.NotEmpty(t, turnMetadata)
	assert.Contains(t, turnMetadata, ids.sessionID, "turn-metadata JSON 中的 session_id 应被收敛")
	assert.Contains(t, turnMetadata, `"sandbox":"seatbelt"`, "turn-metadata 未指定字段应原样保留")
}

func TestBuildUpstreamRequestOpenAIPassthrough_OffModeKeepsIsolatedSession(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := newTestOAuthAccount(2002, map[string]any{
		"openai_oauth_passthrough": true,
		"codex_fingerprint_mode":   "off",
	})

	c := newFingerprintStageTestContext(t)
	c.Request.Header.Set("session_id", "real-client-session")
	c.Request.Header.Set("originator", "codex_cli_rs")

	ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
	require.Nil(t, ids)
	stageCodexFingerprintIDs(c, ids)

	body := []byte(`{"model":"gpt-5.6-sol","input":[],"stream":true}`)
	req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test-token")
	require.NoError(t, err)

	assert.NotEmpty(t, req.Header.Get("session_id"))
	assert.NotEqual(t, resolveConvergedSessionID(testCodexFingerprintSeed), req.Header.Get("session_id"), "off 模式不得收敛 session_id")
	assert.Empty(t, req.Header.Get("x-codex-window-id"))
}

func TestApplyCodexFingerprintClientMetadataRaw_NonObjectBodyUntouched(t *testing.T) {
	account := newTestOAuthAccount(4244, map[string]any{codexFingerprintModeExtraKey: "session"})
	ids := resolveCodexFingerprintIDs(account, "client-sess-nonobj", codexFingerprintSession)
	require.NotNil(t, ids)

	for _, body := range []string{`[1,2,3]`, `"plain string"`, `not json at all`} {
		out, changed, err := applyCodexFingerprintClientMetadataRaw([]byte(body), ids)
		require.NoError(t, err)
		assert.False(t, changed, "非 JSON 对象 body 不应被改写: %s", body)
		assert.Equal(t, []byte(body), out)
	}
}

// --- W3C Trace 链路步进引擎测试 ---

func TestParseW3CTraceparent(t *testing.T) {
	// 合法测试
	valid := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	p, ok := ParseW3CTraceparent(valid)
	require.True(t, ok)
	require.NotNil(t, p)
	assert.Equal(t, "00", p.Version)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", p.TraceID)
	assert.Equal(t, "00f067aa0ba902b7", p.SpanID)
	assert.Equal(t, "01", p.Flags)

	// 非法测试
	invalidCases := []struct {
		name string
		raw  string
	}{
		{"长度不足", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7"},
		{"分段数错误", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra"},
		{"版本为ff", "ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		{"traceID全零", "00-00000000000000000000000000000000-00f067aa0ba902b7-01"},
		{"spanID全零", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01"},
		{"包含非hex字符", "00-4bf92f3577b34da6a3ce929d0e0e473g-00f067aa0ba902b7-01"},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, valid := ParseW3CTraceparent(tc.raw)
			assert.False(t, valid, "应判定为非法 traceparent: %s", tc.raw)
			assert.Nil(t, parsed)
		})
	}
}

func TestDeriveTraceparentWithUpstream_InheritValidUpstream(t *testing.T) {
	upstream := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	accountID := int64(1001)
	seed := "9f8c6d3e-5b2a-4f1c-8e7d-3a1b5c9e2f4a"
	sessionID := "sess-upstream-test"
	turnTime1 := int64(1700000000000)
	turnTime2 := int64(1700000005000)

	derived1 := DeriveTraceparentWithUpstream(upstream, accountID, seed, sessionID, turnTime1)
	p1, ok1 := ParseW3CTraceparent(derived1)
	require.True(t, ok1, "派生结果必须为有效 W3C traceparent")
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", p1.TraceID, "必须继承上游 trace_id")
	assert.NotEqual(t, "00f067aa0ba902b7", p1.SpanID, "子 span_id 必须与上游父 span_id 不同")
	assert.False(t, isAllZeroHex(p1.SpanID), "子 span_id 杜绝全零")
	assert.Equal(t, "01", p1.Flags)

	// 同一轮次重放确定性
	derived1Repeat := DeriveTraceparentWithUpstream(upstream, accountID, seed, sessionID, turnTime1)
	assert.Equal(t, derived1, derived1Repeat, "相同上游与相同时间戳派生结果必须幂等一致")

	// 不同轮次时间戳步进演变
	derived2 := DeriveTraceparentWithUpstream(upstream, accountID, seed, sessionID, turnTime2)
	p2, ok2 := ParseW3CTraceparent(derived2)
	require.True(t, ok2)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", p2.TraceID, "后续轮次仍然继承上游 trace_id")
	assert.NotEqual(t, p1.SpanID, p2.SpanID, "不同轮次时间戳必须衍生出不同的子 span_id")
}

func TestDeriveTraceparentWithUpstream_NaturalEpochTransition(t *testing.T) {
	accountID := int64(1002)
	seed := "9f8c6d3e-5b2a-4f1c-8e7d-3a1b5c9e2f4a"
	sessionID := "sess-epoch-test"

	baseTime := int64(1700000000000)
	// 在同一个 8 小时 Epoch 窗口内
	withinEpochTime := baseTime + 1000*60*60 // +1 小时
	// 确保处于相同 epoch
	require.Equal(t, baseTime/W3CTraceEpochWindowMs, withinEpochTime/W3CTraceEpochWindowMs)

	tp1 := DeriveTraceparentWithUpstream("", accountID, seed, sessionID, baseTime)
	p1, ok1 := ParseW3CTraceparent(tp1)
	require.True(t, ok1)
	assert.False(t, isAllZeroHex(p1.TraceID))
	assert.False(t, isAllZeroHex(p1.SpanID))

	tpWithin := DeriveTraceparentWithUpstream("", accountID, seed, sessionID, withinEpochTime)
	pWithin, okWithin := ParseW3CTraceparent(tpWithin)
	require.True(t, okWithin)
	assert.Equal(t, p1.TraceID, pWithin.TraceID, "同一 8 小时自然工时 Epoch 内 trace_id 必须保持稳定自洽")
	assert.NotEqual(t, p1.SpanID, pWithin.SpanID, "不同时间戳 span_id 必须自然步进")

	// 跨越 8 小时 Epoch
	acrossEpochTime := baseTime + W3CTraceEpochWindowMs + 1000
	require.NotEqual(t, baseTime/W3CTraceEpochWindowMs, acrossEpochTime/W3CTraceEpochWindowMs)

	tpAcross := DeriveTraceparentWithUpstream("", accountID, seed, sessionID, acrossEpochTime)
	pAcross, okAcross := ParseW3CTraceparent(tpAcross)
	require.True(t, okAcross)
	assert.NotEqual(t, p1.TraceID, pAcross.TraceID, "跨越 8 小时自然工时后 trace_id 必须发生自然平滑跃迁")
	assert.False(t, isAllZeroHex(pAcross.TraceID))
	assert.False(t, isAllZeroHex(pAcross.SpanID))
}

func TestDeriveTraceparentWithUpstream_MalformedAndZeroFallback(t *testing.T) {
	accountID := int64(1003)
	seed := "9f8c6d3e-5b2a-4f1c-8e7d-3a1b5c9e2f4a"
	sessionID := "sess-malformed-test"
	now := int64(1700000000000)

	malformedInputs := []string{
		"",
		"invalid-format",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01", // 全零 trace_id
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", // 全零 span_id
		"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // version ff
	}

	for _, malformed := range malformedInputs {
		tp := DeriveTraceparentWithUpstream(malformed, accountID, seed, sessionID, now)
		p, ok := ParseW3CTraceparent(tp)
		assert.True(t, ok, "畸形或非法上游输入必须安全回退生成合法的 traceparent: %s", malformed)
		require.NotNil(t, p)
		assert.False(t, isAllZeroHex(p.TraceID), "回退生成的 trace_id 绝不能全零")
		assert.False(t, isAllZeroHex(p.SpanID), "回退生成的 span_id 绝不能全零")
		assert.Equal(t, "00", p.Version)
		assert.Equal(t, "01", p.Flags)
	}

	t.Run("turnStartedAtUnixMs 为 0 或负数时自动防御归一化", func(t *testing.T) {
		tpZero := DeriveTraceparentWithUpstream("", accountID, seed, sessionID, 0)
		pZero, okZero := ParseW3CTraceparent(tpZero)
		require.True(t, okZero)
		assert.False(t, isAllZeroHex(pZero.TraceID))
		assert.False(t, isAllZeroHex(pZero.SpanID))
		assert.False(t, strings.Contains(pZero.TraceID, "-"))

		tpNeg := DeriveTraceparentWithUpstream("", accountID, seed, sessionID, -50000000)
		pNeg, okNeg := ParseW3CTraceparent(tpNeg)
		require.True(t, okNeg)
		assert.False(t, isAllZeroHex(pNeg.TraceID))
		assert.False(t, isAllZeroHex(pNeg.SpanID))
		assert.False(t, strings.Contains(pNeg.TraceID, "-"))
	})
}

func TestResolveCodexFingerprintIDsFromRequest_TraceparentExtraction(t *testing.T) {
	account := newTestOAuthAccount(5001, map[string]any{
		codexFingerprintModeExtraKey: "session",
	})
	upstream := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	reqHeaders := http.Header{}
	reqHeaders.Set("session-id", "test-client-sess")
	reqHeaders.Set("traceparent", upstream)

	ids := resolveCodexFingerprintIDsFromRequest(account, reqHeaders)
	require.NotNil(t, ids)
	assert.Equal(t, upstream, ids.clientTraceparent, "必须成功提取 clientTraceparent")

	p, ok := ParseW3CTraceparent(ids.traceparent)
	require.True(t, ok)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", p.TraceID, "必须继承请求头中的合法上游 trace_id")
	assert.NotEqual(t, "00f067aa0ba902b7", p.SpanID, "必须演化生成新子 span_id")

	// 验证请求体 client_metadata 中的 traceparent 必须与出站 Header 里的 traceparent 100% 镜像一致
	bodyMap := map[string]any{
		"model": "gpt-5.6-sol",
		"client_metadata": map[string]any{
			"session_id":  "test-client-sess",
			"traceparent": "00-stale-stale-01",
		},
	}
	applyCodexFingerprintClientMetadata(bodyMap, ids)
	cm, _ := bodyMap["client_metadata"].(map[string]any)
	require.NotNil(t, cm)
	assert.Equal(t, ids.traceparent, cm["traceparent"], "请求体中的 traceparent 必须与出站 Header 里的 traceparent 100% 镜像一致，杜绝头体撕裂")
}

func TestFixedSlotRegistry_LifecycleAndSpecs(t *testing.T) {
	ResetGlobalSlotRegistryForTest()
	accountID := int64(8888)
	seed := "22222222-2222-4222-8222-222222222222"

	// 1. 验证 Specs 静态派生
	specs := DeriveAccountSlotSpecs(accountID, seed)
	assert.Equal(t, 0, specs[0].Index)
	assert.Equal(t, resolveConvergedSessionID(seed), specs[0].SessionID)
	assert.Equal(t, "main", specs[0].GitBranch)

	assert.Equal(t, 1, specs[1].Index)
	assert.Equal(t, deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-session-id:v2:%s:slot-2", seed)), specs[1].SessionID)
	assert.True(t, strings.HasSuffix(specs[1].WorkspacePath, "service-infra"))
	assert.Equal(t, "feature/api-v2", specs[1].GitBranch)

	assert.Equal(t, 2, specs[2].Index)
	assert.Equal(t, deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-session-id:v2:%s:slot-3", seed)), specs[2].SessionID)
	assert.True(t, strings.HasSuffix(specs[2].WorkspacePath, "scripts"))
	assert.Equal(t, "dev", specs[2].GitBranch)

	// 2. 状态机初始为 1
	registry := GetGlobalSlotRegistry()
	state := registry.GetOrCreateAccountState(accountID, seed)
	require.NotNil(t, state)
	assert.Equal(t, 1, state.unlockedCount)

	// 3. 连续单请求复用 Slot 0
	l1 := state.AcquireSlot("client-1")
	assert.Equal(t, 0, l1.SlotIndex)
	assert.Equal(t, int32(1), state.activeInFlight[0])
	assert.Equal(t, 1, state.unlockedCount)
	l1.Release()
	assert.Equal(t, int32(0), state.activeInFlight[0])

	// 会话亲和复用 Slot 0
	l1Again := state.AcquireSlot("client-1")
	assert.Equal(t, 0, l1Again.SlotIndex)
	l1Again.Release()

	// 4. 并发活跃时自然点亮至 2 和 3
	hold1 := state.AcquireSlot("client-1")
	assert.Equal(t, 0, hold1.SlotIndex)
	assert.Equal(t, 1, state.unlockedCount)

	// client-2 请求时 Slot 0 正处于 in-flight，动态点亮 Slot 1
	hold2 := state.AcquireSlot("client-2")
	assert.Equal(t, 1, hold2.SlotIndex)
	assert.Equal(t, 2, state.unlockedCount)

	// client-3 请求时 Slot 0、1 都处于 in-flight，动态点亮 Slot 2
	hold3 := state.AcquireSlot("client-3")
	assert.Equal(t, 2, hold3.SlotIndex)
	assert.Equal(t, 3, state.unlockedCount)

	// 5. 达到 MaxCodexSlots (3) 封顶，不再点亮新槽位，排队到最小 in-flight 槽位
	hold4 := state.AcquireSlot("client-4")
	assert.True(t, hold4.SlotIndex >= 0 && hold4.SlotIndex < MaxCodexSlots)
	assert.Equal(t, 3, state.unlockedCount) // 封顶不超

	// 6. 安全释放与防重复释放验证
	assert.Equal(t, int32(1), hold1.released^1) // 尚未释放
	hold1.Release()
	assert.Equal(t, int32(1), hold1.released)
	hold1.Release() // 重复释放不产生负数或多次减计数

	hold2.Release()
	hold3.Release()
	hold4.Release()

	for i := 0; i < MaxCodexSlots; i++ {
		assert.Equal(t, int32(0), state.activeInFlight[i], "全部释放后 in-flight 计数归零")
	}

	// 7. 只点亮不销毁：即便请求全部完成，unlockedCount 依然保持 3
	assert.Equal(t, 3, state.unlockedCount)
}

