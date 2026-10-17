package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// MaxCodexSlots 定义每个账号最大分配的固定工位数 (只点亮不销毁)
const MaxCodexSlots = 3

// CodexSlotStaticSpec 定义工作区槽位的静态画像规格
type CodexSlotStaticSpec struct {
	Index         int
	SessionID     string
	WorkspacePath string
	GitBranch     string
}

// DeriveAccountSlotSpecs 确定性派生账号对应的固定 3 工位规格
func DeriveAccountSlotSpecs(accountID int64, seed string) [MaxCodexSlots]CodexSlotStaticSpec {
	hw := ResolveHardwareProfile(accountID, seed)

	// Windows 系统路径反斜杠兼容判断
	isWindows := hw.OSName == "Windows" || strings.HasPrefix(hw.WorkspacePath, "C:\\")

	var slot1Path, slot2Path string
	if isWindows {
		// 对应 Windows: C:\Users\<Username>\source\repos\service-infra 与 scripts
		// 保持与 hw.HomeDir 或 hw.WorkspacePath 前缀一致
		userPrefix := fmt.Sprintf("C:\\Users\\%s\\source\\repos", strings.Title(hw.Username))
		slot1Path = fmt.Sprintf("%s\\service-infra", userPrefix)
		slot2Path = fmt.Sprintf("%s\\scripts", userPrefix)
	} else {
		// macOS / Linux: /Users/<Username>/Projects/service-infra 与 scripts
		slot1Path = fmt.Sprintf("/Users/%s/Projects/service-infra", hw.Username)
		slot2Path = fmt.Sprintf("/Users/%s/Projects/scripts", hw.Username)
	}

	return [MaxCodexSlots]CodexSlotStaticSpec{
		{
			Index:         0,
			SessionID:     resolveConvergedSessionID(seed),
			WorkspacePath: hw.WorkspacePath,
			GitBranch:     "main",
		},
		{
			Index:         1,
			SessionID:     deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-session-id:v2:%s:slot-2", seed)),
			WorkspacePath: slot1Path,
			GitBranch:     "feature/api-v2",
		},
		{
			Index:         2,
			SessionID:     deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-session-id:v2:%s:slot-3", seed)),
			WorkspacePath: slot2Path,
			GitBranch:     "dev",
		},
	}
}

// AccountSlotRuntimeState 账号工作区槽位池运行时状态机
type AccountSlotRuntimeState struct {
	mu             sync.RWMutex
	AccountID      int64
	Seed           string
	Specs          [MaxCodexSlots]CodexSlotStaticSpec
	unlockedCount  int
	activeInFlight [MaxCodexSlots]int32
	clientBindings map[string]int
	bindingAccess  map[string]int64
}

// SlotLease 槽位租约凭证
type SlotLease struct {
	AccountID int64
	SlotIndex int
	Spec      CodexSlotStaticSpec
	state     *AccountSlotRuntimeState
	released  int32
}

// Release 释放槽位租约（CAS 防重复释放）
func (l *SlotLease) Release() {
	if l == nil || l.state == nil {
		return
	}
	if atomic.CompareAndSwapInt32(&l.released, 0, 1) {
		l.state.releaseSlot(l.SlotIndex)
	}
}

// releaseSlot 原子递减 in-flight 并保证保底不小于 0
func (s *AccountSlotRuntimeState) releaseSlot(slotIndex int) {
	if slotIndex < 0 || slotIndex >= MaxCodexSlots {
		return
	}
	for {
		cur := atomic.LoadInt32(&s.activeInFlight[slotIndex])
		if cur <= 0 {
			break
		}
		if atomic.CompareAndSwapInt32(&s.activeInFlight[slotIndex], cur, cur-1) {
			break
		}
	}
}

// AcquireSlot 为请求分配工位槽位
// 策略：阶段 A 会话亲和 -> 阶段 B 空闲探测优先 Slot 0 -> 阶段 C 动态点亮 1->2->3 只增不减 -> 阶段 D 最小 inFlight 排队
func (s *AccountSlotRuntimeState) AcquireSlot(clientSessionID string) *SlotLease {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	chosenSlot := -1

	// 阶段 A: 会话亲和性优先
	if clientSessionID != "" {
		if boundSlot, exists := s.clientBindings[clientSessionID]; exists {
			if boundSlot >= 0 && boundSlot < s.unlockedCount {
				chosenSlot = boundSlot
				s.bindingAccess[clientSessionID] = now
			}
		}
	}

	// 阶段 B: 空闲探测（优先 Slot 0，其次已点亮的其他槽位）
	if chosenSlot == -1 {
		for i := 0; i < s.unlockedCount; i++ {
			if atomic.LoadInt32(&s.activeInFlight[i]) == 0 {
				chosenSlot = i
				break
			}
		}
	}

	// 阶段 C: 动态点亮 (若所有已解锁槽位都有活跃请求且 unlockedCount < MaxCodexSlots)
	if chosenSlot == -1 && s.unlockedCount < MaxCodexSlots {
		chosenSlot = s.unlockedCount
		s.unlockedCount++
	}

	// 阶段 D: 达到最大工位数，在已点亮工位中寻找最小 inFlight 排队
	if chosenSlot == -1 {
		minInFlight := int32(1<<31 - 1)
		bestSlot := 0
		for i := 0; i < s.unlockedCount; i++ {
			flight := atomic.LoadInt32(&s.activeInFlight[i])
			if flight < minInFlight {
				minInFlight = flight
				bestSlot = i
			}
		}
		chosenSlot = bestSlot
	}

	// 绑定并递增 inFlight
	if clientSessionID != "" {
		s.clientBindings[clientSessionID] = chosenSlot
		s.bindingAccess[clientSessionID] = now
		// 防御内存泄漏：若活跃绑定超过 512，清理 24 小时未活跃的条目
		if len(s.clientBindings) > 512 {
			for k, t := range s.bindingAccess {
				if now-t > 86400 {
					delete(s.clientBindings, k)
					delete(s.bindingAccess, k)
				}
			}
		}
	}
	atomic.AddInt32(&s.activeInFlight[chosenSlot], 1)

	return &SlotLease{
		AccountID: s.AccountID,
		SlotIndex: chosenSlot,
		Spec:      s.Specs[chosenSlot],
		state:     s,
	}
}

// FixedSlotRegistry 账号槽位池全局注册表
type FixedSlotRegistry struct {
	mu     sync.RWMutex
	states map[int64]*AccountSlotRuntimeState
}

var (
	globalSlotRegistry     *FixedSlotRegistry
	globalSlotRegistryOnce sync.Once
)

// GetGlobalSlotRegistry 获取全局单例槽位注册表
func GetGlobalSlotRegistry() *FixedSlotRegistry {
	globalSlotRegistryOnce.Do(func() {
		globalSlotRegistry = &FixedSlotRegistry{
			states: make(map[int64]*AccountSlotRuntimeState),
		}
	})
	return globalSlotRegistry
}

// ResetGlobalSlotRegistryForTest 重置全局注册表（仅用于单测隔离）
func ResetGlobalSlotRegistryForTest() {
	globalSlotRegistryOnce = sync.Once{}
	globalSlotRegistry = nil
}

// GetOrCreateAccountState 获取或创建账号对应的槽位运行时状态机
func (r *FixedSlotRegistry) GetOrCreateAccountState(accountID int64, seed string) *AccountSlotRuntimeState {
	if accountID <= 0 || strings.TrimSpace(seed) == "" {
		return nil
	}

	r.mu.RLock()
	st, exists := r.states[accountID]
	if exists && st.Seed == seed {
		r.mu.RUnlock()
		return st
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	if st, exists = r.states[accountID]; exists && st.Seed == seed {
		return st
	}

	specs := DeriveAccountSlotSpecs(accountID, seed)
	newState := &AccountSlotRuntimeState{
		AccountID:      accountID,
		Seed:           seed,
		Specs:          specs,
		unlockedCount:  1, // 初始为 1
		clientBindings: make(map[string]int),
		bindingAccess:  make(map[string]int64),
	}
	r.states[accountID] = newState
	return newState
}

// codexFingerprintIDsContextKey 是暂存在 gin context 的收敛 ID 集合键。
// 由 Forward（非透传）或 forwardOpenAIPassthrough（透传）解析后写入，请求
// 构造器读取用于出站头改写——请求体与出站头必须共享同一份 IDs，保证
// turn_id 等随机字段一致。
const codexFingerprintIDsContextKey = "codex_fingerprint_ids"

// stageCodexFingerprintIDs 将本 attempt 解析出的收敛 ID 暂存到 gin context。
// 必须无条件覆写（含 nil）：failover 从收敛账号切到 off 账号时，上一账号的
// IDs 不得残留并被误应用到新账号的出站头（typed-nil 由应用侧 nil 守卫吸收）。
func stageCodexFingerprintIDs(c *gin.Context, ids *codexFingerprintIDs) {
	if c != nil {
		c.Set(codexFingerprintIDsContextKey, ids)
	}
}

func stagedCodexFingerprintIDs(c *gin.Context, account *Account) *codexFingerprintIDs {
	if c == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return nil
	}
	value, ok := c.Get(codexFingerprintIDsContextKey)
	if !ok {
		return nil
	}
	ids, ok := value.(*codexFingerprintIDs)
	if !ok || ids == nil || ids.accountID != account.ID {
		return nil
	}
	return ids
}

// applyStagedCodexFingerprintHeaders 读取 context 暂存的收敛 ID 并改写出站头。
// 非透传与透传两个请求构造器共用本函数，防止应用语义漂移。仅解析该
// snapshot 的 OAuth 账号可读取，避免 stale context 跨账号 failover 泄漏。
func applyStagedCodexFingerprintHeaders(c *gin.Context, account *Account, h http.Header) {
	applyCodexFingerprintHeaders(h, stagedCodexFingerprintIDs(c, account))
}

func applyStagedCodexFingerprintClientMetadata(c *gin.Context, account *Account, reqBody map[string]any) bool {
	return applyCodexFingerprintClientMetadata(reqBody, stagedCodexFingerprintIDs(c, account))
}

// codexFingerprintMode 控制 OAuth 账号出站请求的设备指纹收敛强度。
// 多人共享同一 OAuth 账号时，每个用户的 Codex 客户端会携带各自不同的
// installation_id / session_id / thread_id，上游据此判定设备数和会话数。
// 收敛模式将这些标识改写为账号级恒定值，减少上游可见的设备/会话指纹。
type codexFingerprintMode string

const (
	// codexFingerprintOff 不做任何收敛，原样透传客户端标识。
	// 这是默认值：收敛是显式 opt-in 的（见 GetCodexFingerprintMode）。
	codexFingerprintOff codexFingerprintMode = "off"
	// codexFingerprintDevice 仅收敛 installation_id 为账号级恒定值。
	// 上游看到 1 台设备 + 多会话（每用户各自的 session）。
	codexFingerprintDevice codexFingerprintMode = "device"
	// codexFingerprintSession 收敛 installation_id + session_id，
	// thread_id 按客户端原始 session-id 确定性派生（每个真实 Codex 会话一个独立线程）。
	// 上游看到 1 台设备 + 1 会话 + N 线程，最接近正常用户 spawn 子代理的模式。
	codexFingerprintSession codexFingerprintMode = "session"
	// codexFingerprintFull 收敛所有标识：installation_id + session_id + thread_id。
	// 上游看到 1 台设备 + 1 会话 + 1 线程，最激进。
	codexFingerprintFull codexFingerprintMode = "full"
)

const (
	codexFingerprintModeExtraKey = "codex_fingerprint_mode"
	codexFingerprintSeedExtraKey = "codex_fingerprint_seed"
)

func canonicalCodexFingerprintSeed(value any) (string, bool) {
	raw, ok := value.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(raw)
	parsed, err := uuid.Parse(trimmed)
	if err != nil || parsed == uuid.Nil || trimmed != parsed.String() {
		return "", false
	}
	return trimmed, true
}

func newCodexFingerprintSeed() string {
	return uuid.NewString()
}

func stripCodexFingerprintSeed(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	stripped := maps.Clone(extra)
	delete(stripped, codexFingerprintSeedExtraKey)
	return stripped
}

func codexFingerprintModeFromExtra(extra map[string]any) codexFingerprintMode {
	if extra == nil {
		return codexFingerprintOff
	}
	raw, _ := extra[codexFingerprintModeExtraKey].(string)
	switch codexFingerprintMode(strings.TrimSpace(raw)) {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull:
		return codexFingerprintMode(strings.TrimSpace(raw))
	default:
		return codexFingerprintOff
	}
}

func codexFingerprintModeRequiresSeed(mode codexFingerprintMode) bool {
	switch mode {
	case codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull:
		return true
	default:
		return false
	}
}

func codexFingerprintSeed(extra map[string]any) (string, bool) {
	if extra == nil {
		return "", false
	}
	return canonicalCodexFingerprintSeed(extra[codexFingerprintSeedExtraKey])
}

func prepareCodexFingerprintExtraForCreate(platform, accountType string, extra map[string]any) map[string]any {
	prepared := stripCodexFingerprintSeed(extra)
	if platform != PlatformOpenAI || (accountType != AccountTypeOAuth && accountType != AccountTypeSetupToken) || !codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(prepared)) {
		return prepared
	}
	if prepared == nil {
		prepared = make(map[string]any, 1)
	}
	prepared[codexFingerprintSeedExtraKey] = newCodexFingerprintSeed()
	return prepared
}

func prepareCodexFingerprintExtraForUpdate(account *Account, extra map[string]any) map[string]any {
	prepared := stripCodexFingerprintSeed(extra)
	if account == nil || !account.IsOpenAIOAuthLike() {
		return prepared
	}
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		if prepared == nil {
			prepared = make(map[string]any, 1)
		}
		prepared[codexFingerprintSeedExtraKey] = seed
		return prepared
	}
	if codexFingerprintModeRequiresSeed(codexFingerprintModeFromExtra(prepared)) {
		if prepared == nil {
			prepared = make(map[string]any, 1)
		}
		prepared[codexFingerprintSeedExtraKey] = newCodexFingerprintSeed()
	}
	return prepared
}

func sanitizedCodexFingerprintExtraUpdates(updates map[string]any) map[string]any {
	if updates == nil {
		return nil
	}
	sanitized := maps.Clone(updates)
	delete(sanitized, codexFingerprintSeedExtraKey)
	return sanitized
}

// ShouldEnsureCodexFingerprintSeedForExtraUpdates reports whether a JSONB key-level
// extra update is enabling Codex fingerprint convergence and therefore must atomically
// preserve or create the system-managed per-account seed in the repository update.
func ShouldEnsureCodexFingerprintSeedForExtraUpdates(updates map[string]any) bool {
	if updates == nil {
		return false
	}
	raw, exists := updates[codexFingerprintModeExtraKey]
	if !exists {
		return false
	}
	modeStr, _ := raw.(string)
	return codexFingerprintModeRequiresSeed(codexFingerprintMode(strings.TrimSpace(modeStr)))
}

// GetCodexFingerprintMode 从账号 extra JSON 读取指纹收敛模式。
//
// **收敛是显式 opt-in**：未设置、空值或非法值一律按 off 处理，只有管理员
// 明确配置 device / session / full 才收敛。
//
// 历史：v0.1.175（#5553）把缺省值当作 session，导致升级后存量 OAuth 账号
// （普遍没有这个 extra 键）的每个非透传请求都被静默改写 installation /
// session / thread / turn / window 五类标识；#5555、#5556、#5582 报告的额度
// 缩水都卡在该版本边界，并有"回退 v0.1.173 即恢复"与"新账号开收敛后降额"
// 的 A/B 实测。上游的配额判定策略不可观测，因此这里取兼容安全的一侧：
// 不显式 opt-in 就保持 v0.1.175 之前的客户端身份（#5610）。
func (a *Account) GetCodexFingerprintMode() codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuthLike() {
		return codexFingerprintOff
	}
	return codexFingerprintModeFromExtra(a.Extra)
}

// deriveStableUUIDv4 从种子确定性派生一个 UUIDv4 格式的字符串。
// 同一种子永远返回同一值。
func deriveStableUUIDv4(seed string) string {
	h := sha256.Sum256([]byte(seed))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16])
}

// resolveConvergedInstallationID 返回账号级恒定的 installation_id。
// 优先使用管理员配置的真实 device_id，无则从系统管理的账号随机种子确定性派生。
func resolveConvergedInstallationID(account *Account, seed string) string {
	if account == nil {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deviceID
	}
	if seed == "" {
		return ""
	}
	return deriveStableUUIDv4("sub2api:codex-install-id:v2:" + seed)
}

// resolveConvergedSessionID 返回账号级恒定的 session_id。
func resolveConvergedSessionID(seed string) string {
	if seed == "" {
		return ""
	}
	return deriveStableUUIDv4("sub2api:codex-session-id:v2:" + seed)
}

// resolveConvergedThreadID 按客户端原始 session-id 确定性派生 thread_id。
// 每个真实 Codex 会话（不同客户端启动实例）获得一个独立线程，
// 模拟正常用户 spawn 子代理或开多窗口的模式。
func resolveConvergedThreadID(seed, clientSessionID string) string {
	if seed == "" || clientSessionID == "" {
		return ""
	}
	return deriveStableUUIDv4("sub2api:codex-thread-id:v2:" + seed + ":" + clientSessionID)
}

// codexFingerprintIDs 收敛后的完整 ID 集合。
// 由 resolveCodexFingerprintIDs 一次性生成，同一个实例在头改写和体改写之间共享，
// 确保所有载体中的 turn_id 等随机字段一致。体改写时还会补记原始
// client_metadata.session_id，用于识别 root prompt_cache_key 的默认值。
type codexFingerprintIDs struct {
	accountID                     int64
	mode                          codexFingerprintMode
	slotIndex                     int
	lease                         *SlotLease
	installationID                string
	sessionID                     string
	threadID                      string
	turnID                        string
	windowID                      string
	turnStartedAtUnixMs           int64
	clientTraceparent             string
	originalBodySessionID         string
	originalBodySessionIDCaptured bool
	userAgent                     string
	acceptLanguage                string
	clientTimezone                string
	osName                        string
	osVersion                     string
	arch                          string
	cpuCores                      int
	memoryGB                      int
	terminalCols                  int
	terminalRows                  int
	username                      string
	homeDir                       string
	workspacePath                 string
	gitBranch                     string
	traceparent                   string
	baggage                       string
}

// resolveCodexFingerprintIDsWithClientContext 按收敛模式计算出站 ID 集合，同时感知客户端上下文（如上游 W3C Trace Context）。
func resolveCodexFingerprintIDsWithClientContext(account *Account, clientSessionID, clientTraceparent string, mode codexFingerprintMode) *codexFingerprintIDs {
	if account == nil || mode == codexFingerprintOff {
		return nil
	}
	seed, ok := codexFingerprintSeed(account.Extra)
	if !ok {
		return nil
	}

	ids := &codexFingerprintIDs{
		accountID:           account.ID,
		mode:                mode,
		turnStartedAtUnixMs: time.Now().UnixMilli(),
		clientTraceparent:   clientTraceparent,
	}

	// 采用工业级硬件与环境基因库派生真实美国个人电脑画像 (纯原生终端契约)
	hwProfile := ResolveHardwareProfile(account.ID, seed)
	ids.userAgent = fmt.Sprintf("codex_cli_rs/0.162.1 (%s) xterm-256color", hwProfile.PlatformUA)
	ids.clientTimezone = hwProfile.Timezone
	ids.acceptLanguage = hwProfile.AcceptLanguage
	ids.osName = hwProfile.OSName
	ids.osVersion = hwProfile.OSVersion
	ids.arch = hwProfile.Arch
	ids.cpuCores = hwProfile.CPUCores
	ids.memoryGB = hwProfile.MemoryGB
	ids.terminalCols = hwProfile.TerminalCols
	ids.terminalRows = hwProfile.TerminalRows
	ids.username = hwProfile.Username
	ids.homeDir = hwProfile.HomeDir
	ids.workspacePath = hwProfile.WorkspacePath
	ids.gitBranch = hwProfile.GitBranch
	ids.traceparent = hwProfile.Traceparent
	ids.baggage = hwProfile.Baggage

	ids.installationID = resolveConvergedInstallationID(account, seed)
	if ids.installationID == "" {
		return nil
	}

	switch mode {
	case codexFingerprintDevice:
		return ids

	case codexFingerprintSession:
		registry := GetGlobalSlotRegistry()
		state := registry.GetOrCreateAccountState(account.ID, seed)
		lease := state.AcquireSlot(clientSessionID)

		ids.slotIndex = lease.SlotIndex
		ids.lease = lease
		ids.sessionID = lease.Spec.SessionID
		ids.workspacePath = lease.Spec.WorkspacePath
		ids.gitBranch = lease.Spec.GitBranch

		ids.threadID = resolveConvergedThreadID(seed, clientSessionID)
		if ids.threadID == "" {
			ids.threadID = ids.sessionID
		}
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		// 动态演变链路: 优先继承上游链路或基于 8 小时自然工时 Epoch 演进
		ids.traceparent = DeriveTraceparentWithUpstream(clientTraceparent, account.ID, seed, ids.sessionID, ids.turnStartedAtUnixMs)
		return ids

	case codexFingerprintFull:
		registry := GetGlobalSlotRegistry()
		state := registry.GetOrCreateAccountState(account.ID, seed)
		lease := state.AcquireSlot(clientSessionID)

		ids.slotIndex = lease.SlotIndex
		ids.lease = lease
		ids.sessionID = lease.Spec.SessionID
		ids.workspacePath = lease.Spec.WorkspacePath
		ids.gitBranch = lease.Spec.GitBranch

		ids.threadID = ids.sessionID
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		ids.traceparent = DeriveTraceparentWithUpstream(clientTraceparent, account.ID, seed, ids.sessionID, ids.turnStartedAtUnixMs)
		return ids
	}

	return nil
}

// resolveCodexFingerprintIDs 按收敛模式计算出站 ID 集合 (保持向后兼容)。
// clientSessionID 是客户端原始的 session-id 头值（连字符形式），用于 session 模式下
// 的 thread_id 派生——每个真实 Codex 会话得到一个独立线程。
// 返回 nil 表示 off 模式，不需要改写。
// 注意：包含随机生成的 turn_id，调用方必须只调用一次并共享结果给头改写和体改写。
func resolveCodexFingerprintIDs(account *Account, clientSessionID string, mode codexFingerprintMode) *codexFingerprintIDs {
	return resolveCodexFingerprintIDsWithClientContext(account, clientSessionID, "", mode)
}

// extractClientSessionID 从请求头中提取客户端原始的会话标识。
// 优先取 session-id（连字符形式，Codex CLI 标准），回退到 session_id（下划线形式）。
// 返回的值尚未被 isolateOpenAISessionID 改写，是客户端的真实标识。
func extractClientSessionID(h http.Header) string {
	if v := strings.TrimSpace(h.Get("session-id")); v != "" {
		return v
	}
	return strings.TrimSpace(h.Get("session_id"))
}

// extractClientTraceparent 从请求头中提取客户端原始的 traceparent 标头 (小写与规范化匹配)。
func extractClientTraceparent(h http.Header) string {
	if h == nil {
		return ""
	}
	if v := strings.TrimSpace(h.Get("traceparent")); v != "" {
		return v
	}
	if v := strings.TrimSpace(h.Get("Traceparent")); v != "" {
		return v
	}
	return ""
}

// resolveCodexFingerprintIDsFromRequest 从客户端原始请求头中提取 session-id 与 traceparent，
// 结合账号配置一次性解析收敛 ID 集合。调用方应将返回的 ids 同时传给
// applyCodexFingerprintHeaders 和 applyCodexFingerprintClientMetadata。
func resolveCodexFingerprintIDsFromRequest(account *Account, clientHeaders http.Header) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	if mode == codexFingerprintOff {
		return nil
	}
	clientSessionID := ""
	clientTraceparent := ""
	if clientHeaders != nil {
		clientSessionID = extractClientSessionID(clientHeaders)
		clientTraceparent = extractClientTraceparent(clientHeaders)
	}
	return resolveCodexFingerprintIDsWithClientContext(account, clientSessionID, clientTraceparent, mode)
}

// applyCodexFingerprintHeaders 按预计算的收敛 ID 改写出站 HTTP 头中的设备指纹。
// 在 buildUpstreamRequest 的白名单透传之后、enforceCodexIdentityHeaders 之前调用。
func applyCodexFingerprintHeaders(h http.Header, ids *codexFingerprintIDs) {
	if h == nil || ids == nil {
		return
	}

	// 所有非 off 模式都收敛 installation_id
	h.Set("x-codex-installation-id", ids.installationID)

	if ids.mode == codexFingerprintDevice {
		// 仅当原请求携带了 x-codex-turn-metadata 时，才改写其中的 installation_id；
		// 原请求若未携带，绝不凭空注入一个缺少 session/turn 的残缺头，保持与 Body 的改写逻辑完全镜像一致。
		if strings.TrimSpace(h.Get("x-codex-turn-metadata")) != "" {
			rewriteCodexTurnMetadataFields(h, map[string]any{
				"installation_id": ids.installationID,
			})
		}
		return
	}

	// session / full 模式：改写所有相关头
	h.Set("x-codex-window-id", ids.windowID)
	h.Set("x-client-request-id", ids.threadID)
	// 连字符形式和下划线形式都改写，保证一致
	h.Set("session-id", ids.sessionID)
	h.Set("session_id", ids.sessionID)
	// 仅当客户端原本携带了 conversation_id 时才强制收敛为 sessionID，
	// 避免对纯净官方 CLI 流量凭空捏造 Web 端的非标 conversation_id 头
	if h.Get("conversation_id") != "" {
		h.Set("conversation_id", ids.sessionID)
	}
	h.Set("thread-id", ids.threadID)

	// 注入真实终端环境指纹 (纯原生终端规范：UA、系统语言、本地时区、W3C APM 遥测，绝对不带 sec-ch-ua 浏览器头)
	if ids.userAgent != "" {
		h.Set("User-Agent", ids.userAgent)
	}
	if ids.acceptLanguage != "" {
		h.Set("Accept-Language", ids.acceptLanguage)
	}
	if ids.clientTimezone != "" {
		h.Set("x-codex-client-timezone", ids.clientTimezone)
	}
	if ids.traceparent != "" {
		h.Set("traceparent", ids.traceparent)
	}
	// 动态对齐 baggage 中的 sentry-release 与当前生效的 User-Agent 版本
	version := codexClientVersionFromUA(ids.userAgent)
	if version == "" {
		version = "0.162.1"
	}
	h.Set("baggage", fmt.Sprintf("sentry-environment=production,sentry-release=codex-cli@%s,sentry-user_id=%s", version, ids.username))

	rewriteCodexTurnMetadataFields(h, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": ids.turnStartedAtUnixMs,
	})
}

// rewriteCodexTurnMetadataFields 解析 x-codex-turn-metadata 头中的 JSON，
// 替换指定字段后回写。若原请求未携带该头，则以指定字段自动初始化并注入，
// 确保任何客户端（即使是轻量或非官方客户端）都能拥有官方 codex_cli_rs 必须具备的轮次元数据头。
func rewriteCodexTurnMetadataFields(h http.Header, fields map[string]any) {
	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	var metadata map[string]any
	if raw == "" {
		metadata = make(map[string]any, len(fields))
	} else if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		metadata = make(map[string]any, len(fields))
	}
	for k, v := range fields {
		metadata[k] = v
	}
	rebuilt, err := marshalCodexTurnMetadata(metadata)
	if err != nil {
		return
	}
	h.Set("x-codex-turn-metadata", string(rebuilt))
}

// applyCodexFingerprintClientMetadata 按预计算的收敛 ID 改写请求体中的 client_metadata。
// 使用与头改写相同的 ids 实例，确保 turn_id 等随机字段一致。
func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil {
		return false
	}

	captureCodexFingerprintOriginalBodySessionID(ids, reqBody["client_metadata"])
	existing, _ := reqBody["client_metadata"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}

	modified := false
	if applyCodexFingerprintToClientMetadataMap(existing, ids) {
		reqBody["client_metadata"] = existing
		modified = true
	}
	if applyCodexFingerprintPromptCacheKey(reqBody, ids) {
		modified = true
	}
	return modified
}

// applyCodexFingerprintToClientMetadataMap 是 client_metadata 改写的共享核心，
// map 版（非透传，body 已解码）与 raw 字节版（透传热路径）都经由它，保证两条
// 路径的收敛语义永不漂移。
func applyCodexFingerprintToClientMetadataMap(existing map[string]any, ids *codexFingerprintIDs) bool {
	if existing == nil || ids == nil {
		return false
	}

	modified := false

	if ids.installationID != "" {
		existing["x-codex-installation-id"] = ids.installationID
		modified = true
	}

	if ids.mode == codexFingerprintDevice {
		rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
			"installation_id": ids.installationID,
		})
		return modified
	}

	// session / full 模式：收敛身份标识并补齐物理硬件与环境元数据
	existing["session_id"] = ids.sessionID
	existing["thread_id"] = ids.threadID
	existing["turn_id"] = ids.turnID
	existing["x-codex-window-id"] = ids.windowID
	if ids.traceparent != "" {
		// 严密同步: 若 client_metadata 带有 traceparent，必须与出站 Header 里的 traceparent 100% 镜像一致，杜绝头体撕裂
		if _, exists := existing["traceparent"]; exists {
			existing["traceparent"] = ids.traceparent
		}
	}

	// 注入真实终端原生环境元数据 (100% 贴合官方 codex_cli_rs 运行时协议，杜绝任何非标/沙箱穿透伪造)
	if ids.osName != "" {
		existing["os"] = ids.osName
		existing["os_version"] = ids.osVersion
		existing["arch"] = ids.arch
		existing["cpu_cores"] = ids.cpuCores
		existing["memory_gb"] = ids.memoryGB
		existing["terminal_cols"] = ids.terminalCols
		existing["terminal_rows"] = ids.terminalRows
		existing["workspace_path"] = ids.workspacePath
		existing["git_branch"] = ids.gitBranch
	}

	rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": ids.turnStartedAtUnixMs,
		"workspace_path":          ids.workspacePath,
		"git_branch":              ids.gitBranch,
	})
	return true
}

func captureCodexFingerprintOriginalBodySessionID(ids *codexFingerprintIDs, clientMetadata any) {
	if ids == nil || ids.originalBodySessionIDCaptured {
		return
	}
	ids.originalBodySessionIDCaptured = true
	if clientMetadata == nil {
		return
	}
	switch metadata := clientMetadata.(type) {
	case map[string]any:
		if sessionID, ok := metadata["session_id"].(string); ok {
			ids.originalBodySessionID = strings.TrimSpace(sessionID)
		}
	case map[string]string:
		ids.originalBodySessionID = strings.TrimSpace(metadata["session_id"])
	}
}

func captureCodexFingerprintOriginalBodySessionIDRaw(ids *codexFingerprintIDs, value gjson.Result) {
	if ids == nil || ids.originalBodySessionIDCaptured {
		return
	}
	ids.originalBodySessionIDCaptured = true
	if value.Exists() && value.Type == gjson.String {
		ids.originalBodySessionID = strings.TrimSpace(value.String())
	}
}

func shouldRewriteCodexFingerprintPromptCacheKey(ids *codexFingerprintIDs, promptCacheKey string) bool {
	if ids == nil || !ids.originalBodySessionIDCaptured || ids.originalBodySessionID == "" || ids.sessionID == "" {
		return false
	}
	if ids.mode != codexFingerprintSession && ids.mode != codexFingerprintFull {
		return false
	}
	return promptCacheKey == ids.originalBodySessionID
}

func applyCodexFingerprintPromptCacheKey(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil {
		return false
	}
	promptCacheKey, ok := reqBody["prompt_cache_key"].(string)
	if !ok || strings.TrimSpace(promptCacheKey) == "" || !shouldRewriteCodexFingerprintPromptCacheKey(ids, promptCacheKey) {
		return false
	}
	if promptCacheKey == ids.sessionID {
		return false
	}
	reqBody["prompt_cache_key"] = ids.sessionID
	return true
}

// applyCodexFingerprintClientMetadataRaw 在原始 JSON 字节上改写 client_metadata，
// 供透传路径使用——透传是热路径，禁止对可能高达数十 MB 的 body 做全量
// Unmarshal（见 forwardOpenAIPassthrough 的轻量提取注释）。实现为：gjson 提取
// client_metadata 小对象单独解码，经共享核心改写后 sjson 一次性拼回，body
// 其余字节原样保留；root prompt_cache_key 仅在可证明是 body session 默认值时
// 做标量改写。语义与 applyCodexFingerprintClientMetadata 逐点一致（含
// "非对象值整体替换为收敛集合"的行为）。
func applyCodexFingerprintClientMetadataRaw(body []byte, ids *codexFingerprintIDs) ([]byte, bool, error) {
	if len(body) == 0 || ids == nil {
		return body, false, nil
	}
	// 非 JSON 对象的 body（数组/标量/畸形）没有 client_metadata 语义，
	// sjson 在这类根上写字段会改写整体结构，直接放行保持原样。
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		captureCodexFingerprintOriginalBodySessionIDRaw(ids, gjson.Result{})
		return body, false, nil
	}

	existing := map[string]any{}
	if cm := gjson.GetBytes(body, "client_metadata"); cm.IsObject() {
		captureCodexFingerprintOriginalBodySessionIDRaw(ids, gjson.GetBytes(body, "client_metadata.session_id"))
		if err := json.Unmarshal([]byte(cm.Raw), &existing); err != nil {
			return body, false, fmt.Errorf("decode client_metadata for fingerprint: %w", err)
		}
	} else {
		captureCodexFingerprintOriginalBodySessionIDRaw(ids, gjson.Result{})
	}

	next := body
	modified := false
	if applyCodexFingerprintToClientMetadataMap(existing, ids) {
		raw, err := json.Marshal(existing)
		if err != nil {
			return body, false, fmt.Errorf("encode converged client_metadata: %w", err)
		}
		var setErr error
		next, setErr = sjson.SetRawBytes(body, "client_metadata", raw)
		if setErr != nil {
			return body, false, fmt.Errorf("splice converged client_metadata: %w", setErr)
		}
		modified = true
	}
	promptCacheKey := gjson.GetBytes(body, "prompt_cache_key")
	if promptCacheKey.Exists() && promptCacheKey.Type == gjson.String && strings.TrimSpace(promptCacheKey.String()) != "" && shouldRewriteCodexFingerprintPromptCacheKey(ids, promptCacheKey.String()) {
		rewritten, err := sjson.SetBytes(next, "prompt_cache_key", ids.sessionID)
		if err != nil {
			return body, false, fmt.Errorf("splice converged prompt_cache_key: %w", err)
		}
		next = rewritten
		modified = true
	}
	return next, modified, nil
}

// rewriteClientMetadataEmbeddedTurnMetadata 改写 client_metadata 中内嵌的
// x-codex-turn-metadata JSON 字符串里的指定字段。非法/非对象值会重建，
// 避免 flat client_metadata 与 embedded metadata 暴露两套身份。
func rewriteClientMetadataEmbeddedTurnMetadata(clientMetadata map[string]any, fields map[string]any) {
	raw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	if !ok || raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		metadata = make(map[string]any, len(fields))
	}
	for k, v := range fields {
		metadata[k] = v
	}
	if rebuilt, err := marshalCodexTurnMetadata(metadata); err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}

// AccountFingerprintDetails 完整获取指定账号的终端指纹画像（供管理端可视化面板展示）
type AccountFingerprintDetails struct {
	DeviceModel    string
	OSName         string
	OSVersion      string
	Arch           string
	CPUCores       int
	MemoryGB       int
	Cols           int
	Rows           int
	DefaultShell   string
	PlatformUA     string
	UserAgent      string
	Username       string
	HomeDir        string
	WorkspacePath  string
	GitBranch      string
	City           string
	State          string
	Timezone       string
	AcceptLanguage string
	InstallationID string
	SessionID      string
	WindowID       string
	SeedMasked     string
	Traceparent    string
	Baggage        string
}

// ResolveAccountFingerprintDetails 解析指定账号的完整终端指纹细节
func ResolveAccountFingerprintDetails(account *Account) *AccountFingerprintDetails {
	if account == nil {
		return nil
	}
	seed, ok := codexFingerprintSeed(account.Extra)
	if !ok || seed == "" {
		return nil
	}

	hw := ResolveHardwareProfile(account.ID, seed)
	installID := resolveConvergedInstallationID(account, seed)
	sessionID := resolveConvergedSessionID(seed)
	threadID := resolveConvergedThreadID(seed, "")
	if threadID == "" {
		threadID = sessionID
	}
	windowID := threadID + ":0"

	seedMasked := seed
	if len(seed) > 8 {
		seedMasked = seed[:4] + "****" + seed[len(seed)-4:]
	}

	return &AccountFingerprintDetails{
		DeviceModel:    hw.DeviceModel,
		OSName:         hw.OSName,
		OSVersion:      hw.OSVersion,
		Arch:           hw.Arch,
		CPUCores:       hw.CPUCores,
		MemoryGB:       hw.MemoryGB,
		Cols:           hw.TerminalCols,
		Rows:           hw.TerminalRows,
		DefaultShell:   hw.DefaultShell,
		PlatformUA:     hw.PlatformUA,
		UserAgent:      fmt.Sprintf("codex_cli_rs/0.162.1 (%s) xterm-256color", hw.PlatformUA),
		Username:       hw.Username,
		HomeDir:        hw.HomeDir,
		WorkspacePath:  hw.WorkspacePath,
		GitBranch:      hw.GitBranch,
		City:           hw.City,
		State:          hw.State,
		Timezone:       hw.Timezone,
		AcceptLanguage: hw.AcceptLanguage,
		InstallationID: installID,
		SessionID:      sessionID,
		WindowID:       windowID,
		SeedMasked:     seedMasked,
		Traceparent:    hw.Traceparent,
		Baggage:        fmt.Sprintf("sentry-environment=production,sentry-release=codex-cli@0.162.1,sentry-user_id=%s", hw.Username),
	}
}

// ResolveAccountFingerprintSummary 解析极轻量的设备摘要 (仅供列表快速渲染)
func ResolveAccountFingerprintSummary(account *Account) (deviceModel, osName string, ok bool) {
	if account == nil {
		return "", "", false
	}
	seed, valid := codexFingerprintSeed(account.Extra)
	if !valid || seed == "" {
		return "", "", false
	}
	hw := ResolveHardwareProfile(account.ID, seed)
	return hw.DeviceModel, hw.OSName, true
}

