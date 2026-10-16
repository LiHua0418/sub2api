package service

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
)

// HardwareProfile 终端原生环境画像 (100% 贴合官方 codex_cli_rs 运行时协议)
type HardwareProfile struct {
	// 系统原生平台
	DeviceModel    string // 实体设备型号 (如 "MacBook Pro 14", "ThinkPad P1 Gen 6")
	OSName         string // 操作系统原生名称 ("macOS" 或 "Windows")
	OSVersion      string // 操作系统内核/发行版本号 (如 "14.5.0", "10.0.22631")
	Arch           string // CPU 物理架构 ("arm64" 或 "x86_64")
	SecPlatform    string // 平台标识 ("macOS" 或 "Windows")
	PlatformUA     string // 规范平台段 ("Macintosh; Intel Mac OS X 10_15_7" 或 "Windows NT 10.0; Win64; x64")

	// 基础硬件规格 (纯原生标量)
	CPUCores       int // 物理核心数 (8, 10, 12, 14, 16 等)
	MemoryGB       int // 物理内存容量 (GB, 如 16, 32, 64)

	// 终端物理视图
	TerminalCols   int    // 终端宽度 (列数)
	TerminalRows   int    // 终端高度 (行数)
	DefaultShell   string // 默认登录 Shell (/bin/zsh 或 powershell.exe)

	// 本地网络与地理位置
	City           string // 美国城市
	State          string // 美国州名代码 (CA, NY, TX 等)
	Timezone       string // 真实本地时区 (America/Los_Angeles 等)
	AcceptLanguage string // 语言回退链

	// 本地工作区与开发环境
	Username      string // 本地用户名，如 "vlad", "anne", "alex"
	HomeDir       string // 主目录路径，如 "/Users/vlad"
	WorkspacePath string // 当前真实项目工作区路径
	GitBranch     string // 拟真的当前 Git 分支

	// W3C 分布式追踪标头 (APM 遥测链路上下文)
	Traceparent string // W3C Trace Context
	Baggage     string // W3C Baggage Header
}

type terminalDeviceSpec struct {
	model     string
	osVersion string
	arch      string
	cores     int
	memoryGB  int
	termCols  int
	termRows  int
}

var macCatalog = []terminalDeviceSpec{
	{model: "MacBook Pro 14", osVersion: "14.5.0", arch: "arm64", cores: 10, memoryGB: 16, termCols: 140, termRows: 45},
	{model: "MacBook Air 13", osVersion: "14.4.1", arch: "arm64", cores: 8, memoryGB: 16, termCols: 120, termRows: 38},
	{model: "MacBook Pro 16", osVersion: "14.6.0", arch: "arm64", cores: 14, memoryGB: 36, termCols: 160, termRows: 50},
	{model: "Mac Studio", osVersion: "14.5.0", arch: "arm64", cores: 12, memoryGB: 32, termCols: 160, termRows: 48},
	{model: "MacBook Pro 13", osVersion: "13.6.7", arch: "x86_64", cores: 8, memoryGB: 16, termCols: 120, termRows: 36},
	{model: "iMac 24", osVersion: "14.5.0", arch: "arm64", cores: 8, memoryGB: 16, termCols: 150, termRows: 46},
}

var winCatalog = []terminalDeviceSpec{
	{model: "ThinkPad P1 Gen 6", osVersion: "10.0.22631", arch: "x86_64", cores: 14, memoryGB: 32, termCols: 130, termRows: 40},
	{model: "Dell XPS 15 9530", osVersion: "10.0.22631", arch: "x86_64", cores: 12, memoryGB: 16, termCols: 120, termRows: 38},
	{model: "Lenovo Legion Pro 5", osVersion: "10.0.22631", arch: "x86_64", cores: 16, memoryGB: 32, termCols: 140, termRows: 42},
	{model: "ASUS ROG Zephyrus G14", osVersion: "10.0.22631", arch: "x86_64", cores: 16, memoryGB: 32, termCols: 135, termRows: 40},
	{model: "HP Spectre x360 16", osVersion: "10.0.22631", arch: "x86_64", cores: 12, memoryGB: 16, termCols: 125, termRows: 38},
	{model: "Desktop Workstation", osVersion: "10.0.22631", arch: "x86_64", cores: 16, memoryGB: 64, termCols: 160, termRows: 48},
}

type usGeoSpec struct {
	city     string
	state    string
	timezone string
	lang     string
}

// usGeoCatalog 针对西海岸出口服务器 (Los Angeles, CA) 进行物理地理时区收敛，
// 确保出站请求的 x-codex-client-timezone 与真实公网出口 IP 的地理位置严格匹配闭环
var usGeoCatalog = []usGeoSpec{
	{city: "Los Angeles", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "San Francisco", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9,es;q=0.8"},
	{city: "San Jose", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "San Diego", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "Irvine", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "Palo Alto", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "Seattle", state: "WA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "Sacramento", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
}

var firstNames = []string{
	"alex", "david", "vlad", "anne", "sarah", "michael", "emily", "jason",
	"daniel", "brian", "chris", "ryan", "kevin", "eric", "lisa", "jessica",
}

var developerProjectNames = []string{
	"ai-agent-service", "cloud-infra-core", "fintech-gateway", "data-pipeline-v2",
	"auth-service", "mobile-api-backend", "developer-portal", "mesh-router",
}

// ResolveHardwareProfile 依据确定性种子派生纯净原生的终端环境画像 (100% 贴合官方 codex_cli_rs 运行时协议)
func ResolveHardwareProfile(accountID int64, seed string) HardwareProfile {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:openai_terminal_profile_v1", accountID, seed)))
	n0 := binary.BigEndian.Uint64(h[0:8])
	n1 := binary.BigEndian.Uint64(h[8:16])
	n2 := binary.BigEndian.Uint64(h[16:24])
	n3 := binary.BigEndian.Uint64(h[24:32])

	isMac := (n0 % 100) < 55
	geo := usGeoCatalog[n1%uint64(len(usGeoCatalog))]
	username := firstNames[n3%uint64(len(firstNames))]
	projectName := developerProjectNames[(n0>>4)%uint64(len(developerProjectNames))]
	branchList := []string{"main", "develop", "feat/auth", "fix/gateway"}
	branch := branchList[n1%uint64(len(branchList))]

	var p HardwareProfile
	p.Username = username
	p.City = geo.city
	p.State = geo.state
	p.Timezone = geo.timezone
	p.AcceptLanguage = geo.lang
	p.GitBranch = branch

	if isMac {
		spec := macCatalog[n0%uint64(len(macCatalog))]
		p.DeviceModel = spec.model
		p.OSName = "macOS"
		p.OSVersion = spec.osVersion
		p.Arch = spec.arch
		p.SecPlatform = "macOS"
		p.PlatformUA = "Macintosh; Intel Mac OS X 10_15_7"

		p.CPUCores = spec.cores
		p.MemoryGB = spec.memoryGB
		p.TerminalCols = spec.termCols
		p.TerminalRows = spec.termRows
		p.DefaultShell = "/bin/zsh"
		p.HomeDir = fmt.Sprintf("/Users/%s", username)
		p.WorkspacePath = fmt.Sprintf("/Users/%s/Projects/%s", username, projectName)
	} else {
		spec := winCatalog[n0%uint64(len(winCatalog))]
		p.DeviceModel = spec.model
		p.OSName = "Windows"
		p.OSVersion = spec.osVersion
		p.Arch = "x86_64"
		p.SecPlatform = "Windows"
		p.PlatformUA = "Windows NT 10.0; Win64; x64"

		p.CPUCores = spec.cores
		p.MemoryGB = spec.memoryGB
		p.TerminalCols = spec.termCols
		p.TerminalRows = spec.termRows
		p.DefaultShell = "powershell.exe"
		p.HomeDir = fmt.Sprintf("C:\\Users\\%s", strings.Title(username))
		p.WorkspacePath = fmt.Sprintf("C:\\Users\\%s\\source\\repos\\%s", strings.Title(username), projectName)
	}

	// 派生 W3C 分布式追踪链路标头 (与官方客户端 APM 遥测一致)
	traceID := fmt.Sprintf("%016x%016x", n0^n2, n1^n3)
	spanID := fmt.Sprintf("%016x", n2^n3)
	p.Traceparent = fmt.Sprintf("00-%s-%s-01", traceID, spanID)
	p.Baggage = fmt.Sprintf("sentry-environment=production,sentry-release=codex-cli@0.162.1,sentry-user_id=%s", username)

	return p
}

// ApplyToRequestHeaders 统一将该画像注入 HTTP Headers (完全纯净原生终端契约，绝对无任何浏览器专属头)
func (p HardwareProfile) ApplyToRequestHeaders(h http.Header, version string) {
	if h == nil {
		return
	}
	if version == "" {
		version = "0.162.1"
	}

	h.Set("User-Agent", fmt.Sprintf("codex_cli_rs/%s (%s) xterm-256color", version, p.PlatformUA))
	h.Set("Accept-Language", p.AcceptLanguage)
	h.Set("x-codex-client-timezone", p.Timezone)

	// 注入 W3C 遥测追踪标头 (彻底补齐客户端调用上下文，版本严格与出站 User-Agent 规范对齐)
	if p.Traceparent != "" {
		h.Set("traceparent", p.Traceparent)
	}
	h.Set("baggage", fmt.Sprintf("sentry-environment=production,sentry-release=codex-cli@%s,sentry-user_id=%s", version, p.Username))
}
