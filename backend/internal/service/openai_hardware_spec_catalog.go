package service

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
)

// HardwareProfile 完整的真实个人电脑物理画像 (全拟真军工级·含部件具体型号)
type HardwareProfile struct {
	DeviceModel    string   // 机型名称，如 MacBook Pro 14 (M2 Pro)
	OSName         string   // macOS 或 Windows
	OSVersion      string   // 具体系统内核或构建号
	Arch           string   // arm64 或 x86_64
	PlatformUA     string   // UA平台段
	SecPlatform    string   // Client Hint: "macOS" 或 "Windows"

	// CPU 深度物理规格与型号
	CPUModel       string   // 具体的 CPU 完整型号字符串，如 13th Gen Intel(R) Core(TM) i7-13700H
	CPUCores       int      // 核心数
	CPUThreads     int      // 逻辑线程数
	CPUFrequency   string   // 主频标称，如 "3.4 GHz"

	// 内存深度物理规格与型号
	MemoryGB       int      // 物理内存容量 (GB)
	MemoryType     string   // 内存代际与规格，如 "LPDDR5-6400" 或 "DDR5-5600"
	MemoryModel    string   // 具体内存条/颗粒型号，如 "Samsung M425R2GA3PB0-CWMOL" 或 "Apple Unified Memory"
	JSHeapLimit    int64    // V8 引擎 JS Heap 内存上限 (字节)
	JSHeapTotal    int64    // 当前已分配堆大小
	JSHeapUsed     int64    // 实际存活对象大小

	// 固态硬盘深度物理规格与型号
	SSDModel       string   // 具体的真实 SSD 出厂型号，如 "SAMSUNG MZVL21T0HCLR-00B00 (PM9A1)" 或 "APPLE SSD AP1024Z"
	SSDSizeGB      int      // 固态硬盘物理容量 (512 / 1024 / 2048)
	SSDInterface   string   // 总线接口，如 "PCIe 4.0 x4 NVMe" 或 "Apple Fabric"
	FileSystem     string   // 文件系统 (APFS / NTFS)
	StorageQuota   int64    // 浏览器持久化配额 (字节)
	StorageUsage   int64    // 当前已用缓存 (字节)

	// 屏幕与显示驱动
	ScreenWidth    int      // 屏幕物理宽度
	ScreenHeight   int      // 屏幕物理高度
	AvailHeight    int      // 扣除任务栏/顶栏后的工作区高度
	DPR            float64  // 设备像素比 (Device Pixel Ratio)
	MaxTouchPoints int      // 触控点数 (Mac恒为0，Win部分触控为10)
	WebGLVendor    string   // WebGL 厂商字符串
	WebGLRenderer  string   // 完整的 WebGL 驱动渲染器字符串
	AudioDevice    string   // 扬声器外设标签
	MicDevice      string   // 麦克风外设标签

	// 终端与本地网络
	TerminalCols   int      // 终端列数
	TerminalRows   int      // 终端行数
	DefaultShell   string   // 默认登录 Shell (/bin/zsh 或 powershell.exe)
	DownlinkMbps   int      // 网络接口估计带宽 (Mbps)
	NetworkRTT     int      // 真实 RTT 时延 (ms)
	City           string   // 美国城市
	State          string   // 美国州名代码 (CA, NY, TX 等)
	Timezone       string   // 真实本地时区 (America/Los_Angeles 等)
	AcceptLanguage string   // 拟真的多语言回退链
	ChromeMajor    string   // 主版本号，如 "128"
	ChromeFull     string   // 完整浏览器构建号

	// 软件生态与真实使用痕迹
	Username       string   // 本地用户名，如 "vlad", "anne", "alex"
	HomeDir        string   // 主目录路径，如 "/Users/vlad"
	InstalledApps  []string // 本地安装的高频应用列表
	SystemFonts    []string // 真实安装字体列表 (含系统字体与开发者编程字体)
	WorkspacePath  string   // 当前真实项目工作区路径
	GitBranch      string   // 拟真的当前 Git 分支
	IDEHost        string   // 真实使用的开发工具环境，如 "VSCode 1.96.2"

	// 第二部分：终端 Shell 深度交互与开发环境变量链 (Terminal & Shell Ecology)
	TermType         string   // 终端类型: "xterm-256color"
	ColorTerm        string   // 颜色支持: "truecolor"
	ParentProcess    string   // 父进程: "/bin/zsh" 或 "pwsh.exe"
	ShellPlugins     string   // Shell插件链条: "zsh-autosuggestions,zsh-syntax-highlighting"
	EnvPaths         []string // 开发者关键环境变量路径 (GOPATH, CARGO_HOME, NVM_DIR)

	// 第三部分：微观 Git 仓储深层状态 (Git Repository Micro-Context)
	GitCommitSHA     string   // 40位真实合法 SHA-1 Commit Hash
	GitDirtyFilesCount int    // 未暂存修改文件数 (1 ~ 3)
	GitRemoteOrigin  string   // 真实 Git Remote Origin URL

	// 第五部分：浏览器权限与窗口活动上下文 (Window & Viewport Activity)
	RefreshRate      int      // 物理刷新率 (60 / 120 / 165 Hz)
	WindowState      string   // 窗口状态: "maximized"
	DocumentHidden   bool     // 页面可见性: false
	WindowFocused    bool     // 窗口焦点: true

	// 第一部分：W3C 遥测追踪与 L4/TLS 指纹签名 (APM & Transport Fingerprint)
	Traceparent      string   // W3C Trace Context, 如 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
	Baggage          string   // W3C Baggage Header, 携带真实桌面环境上下文
	JA4Fingerprint   string   // 平台匹配的拟真 JA4 签名字符串
	BatteryLevel     float64  // 物理电池电量 (0.65 ~ 1.0)
	BatteryCharging  bool     // 是否插电办公

	// 键盘、鼠标与交互外设生态 (Human Interface Devices - HID Profile)
	KeyboardModel    string   // 物理键盘出厂型号与布局 (如 "Apple Internal Keyboard (ANSI US)")
	KeyboardLayout   string   // 键盘布局代码 (美式 "00000409" / "com.apple.keylayout.US")
	MouseModel       string   // 物理鼠标/指针设备型号 (如 "Logitech MX Master 3S" 或 "Apple Force Touch Trackpad")
	PointerType      string   // 指针设备类型: "touchpad" / "mouse"
	HasTrackpad      bool     // 是否具备内置触控板
	HasTrackpoint    bool     // 是否具备 ThinkPad 专属小红点指示设备
}

type macDeviceSpec struct {
	model          string
	osVersion      string
	arch           string
	cpuModel       string
	cores          int
	threads        int
	cpuFreq        string
	memoryGB       int
	memoryType     string
	memoryModel    string
	ssdModel       string
	ssdSizeGB      int
	ssdInterface   string
	screenWidth    int
	screenHeight   int
	availHeight    int
	dpr            float64
	renderer       string
	termCols       int
	termRows       int
	keyboardModel  string
	keyboardLayout string
	mouseModel     string
	pointerType    string
	hasTrackpad    bool
	hasTrackpoint  bool
}

type winDeviceSpec struct {
	model          string
	osVersion      string
	cpuModel       string
	cores          int
	threads        int
	cpuFreq        string
	memoryGB       int
	memoryType     string
	memoryModel    string
	ssdModel       string
	ssdSizeGB      int
	ssdInterface   string
	screenWidth    int
	screenHeight   int
	availHeight    int
	dpr            float64
	maxTouchPoints int
	vendor         string
	renderer       string
	termCols       int
	termRows       int
	keyboardModel  string
	keyboardLayout string
	mouseModel     string
	pointerType    string
	hasTrackpad    bool
	hasTrackpoint  bool
}

type usGeoSpec struct {
	city     string
	state    string
	timezone string
	lang     string
}

var macCatalog = []macDeviceSpec{
	{
		model: "MacBook Pro 14 (M2 Pro)", osVersion: "14.5.0", arch: "arm64",
		cpuModel: "Apple M2 Pro (10-core, 6P+4E)", cores: 10, threads: 10, cpuFreq: "3.49 GHz",
		memoryGB: 16, memoryType: "LPDDR5-6400 Unified Memory", memoryModel: "Apple Embedded LPDDR5 (SK Hynix)",
		ssdModel: "APPLE SSD AP0512Z", ssdSizeGB: 512, ssdInterface: "Apple Fabric NVMe",
		screenWidth: 3024, screenHeight: 1964, availHeight: 1939, dpr: 2.0,
		renderer: "Apple M2 Pro", termCols: 140, termRows: 45,
		keyboardModel: "Apple Internal Keyboard (ANSI US Layout)", keyboardLayout: "com.apple.keylayout.US",
		mouseModel: "Apple Force Touch Trackpad", pointerType: "touchpad",
		hasTrackpad: true, hasTrackpoint: false,
	},
	{
		model: "MacBook Air 13 (M2)", osVersion: "14.4.1", arch: "arm64",
		cpuModel: "Apple M2 (8-core, 4P+4E)", cores: 8, threads: 8, cpuFreq: "3.49 GHz",
		memoryGB: 16, memoryType: "LPDDR5-6400 Unified Memory", memoryModel: "Apple Embedded LPDDR5 (Samsung)",
		ssdModel: "APPLE SSD AP0512Z", ssdSizeGB: 512, ssdInterface: "Apple Fabric NVMe",
		screenWidth: 2560, screenHeight: 1664, availHeight: 1639, dpr: 2.0,
		renderer: "Apple M2", termCols: 120, termRows: 38,
		keyboardModel: "Apple Internal Keyboard (ANSI US Layout)", keyboardLayout: "com.apple.keylayout.US",
		mouseModel: "Apple Force Touch Trackpad", pointerType: "touchpad",
		hasTrackpad: true, hasTrackpoint: false,
	},
	{
		model: "MacBook Pro 16 (M3 Max)", osVersion: "14.6.0", arch: "arm64",
		cpuModel: "Apple M3 Max (14-core, 10P+4E)", cores: 14, threads: 14, cpuFreq: "4.05 GHz",
		memoryGB: 36, memoryType: "LPDDR5-6400 Unified Memory", memoryModel: "Apple Embedded LPDDR5 (Micron)",
		ssdModel: "APPLE SSD AP1024Z", ssdSizeGB: 1024, ssdInterface: "Apple Fabric NVMe",
		screenWidth: 3456, screenHeight: 2234, availHeight: 2209, dpr: 2.0,
		renderer: "Apple M3 Max", termCols: 160, termRows: 50,
		keyboardModel: "Apple Internal Keyboard (ANSI US Layout)", keyboardLayout: "com.apple.keylayout.US",
		mouseModel: "Logitech MX Master 3S for Mac (Bluetooth)", pointerType: "mouse",
		hasTrackpad: true, hasTrackpoint: false,
	},
	{
		model: "Mac Studio (M2 Max)", osVersion: "14.5.0", arch: "arm64",
		cpuModel: "Apple M2 Max (12-core, 8P+4E)", cores: 12, threads: 12, cpuFreq: "3.68 GHz",
		memoryGB: 32, memoryType: "LPDDR5-6400 Unified Memory", memoryModel: "Apple Embedded LPDDR5 (Samsung)",
		ssdModel: "APPLE SSD AP1024Z", ssdSizeGB: 1024, ssdInterface: "Apple Fabric NVMe",
		screenWidth: 3840, screenHeight: 2160, availHeight: 2135, dpr: 2.0,
		renderer: "Apple M2 Max", termCols: 160, termRows: 48,
		keyboardModel: "Apple Magic Keyboard with Touch ID and Numeric Keypad", keyboardLayout: "com.apple.keylayout.US",
		mouseModel: "Apple Magic Mouse (Multi-Touch)", pointerType: "mouse",
		hasTrackpad: false, hasTrackpoint: false,
	},
	{
		model: "MacBook Pro 13 (Intel Core i7)", osVersion: "13.6.7", arch: "x86_64",
		cpuModel: "Intel(R) Core(TM) i7-1068NG7 CPU @ 2.30GHz", cores: 8, threads: 8, cpuFreq: "2.30 GHz",
		memoryGB: 16, memoryType: "LPDDR4X-3733", memoryModel: "Samsung K4U6E3S4AM",
		ssdModel: "APPLE SSD AP0512M", ssdSizeGB: 512, ssdInterface: "PCIe 3.0 x4 NVMe",
		screenWidth: 2560, screenHeight: 1600, availHeight: 1575, dpr: 2.0,
		renderer: "Intel(R) Iris(R) Plus Graphics 655", termCols: 120, termRows: 36,
		keyboardModel: "Apple Internal Keyboard (ANSI US Layout)", keyboardLayout: "com.apple.keylayout.US",
		mouseModel: "Apple Force Touch Trackpad", pointerType: "touchpad",
		hasTrackpad: true, hasTrackpoint: false,
	},
	{
		model: "iMac 24 (M3)", osVersion: "14.5.0", arch: "arm64",
		cpuModel: "Apple M3 (8-core, 4P+4E)", cores: 8, threads: 8, cpuFreq: "4.05 GHz",
		memoryGB: 16, memoryType: "LPDDR5-6400 Unified Memory", memoryModel: "Apple Embedded LPDDR5 (SK Hynix)",
		ssdModel: "APPLE SSD AP0512Z", ssdSizeGB: 512, ssdInterface: "Apple Fabric NVMe",
		screenWidth: 4480, screenHeight: 2520, availHeight: 2495, dpr: 2.0,
		renderer: "Apple M3", termCols: 150, termRows: 46,
		keyboardModel: "Apple Magic Keyboard with Touch ID", keyboardLayout: "com.apple.keylayout.US",
		mouseModel: "Apple Magic Trackpad (Multi-Touch)", pointerType: "touchpad",
		hasTrackpad: true, hasTrackpoint: false,
	},
}

var winCatalog = []winDeviceSpec{
	{
		model: "ThinkPad P1 Gen 6", osVersion: "10.0.22631",
		cpuModel: "13th Gen Intel(R) Core(TM) i7-13700H", cores: 14, threads: 20, cpuFreq: "2.40 GHz",
		memoryGB: 32, memoryType: "DDR5-5600 SODIMM", memoryModel: "SK hynix HMCG78AGBSA095N 16GBx2",
		ssdModel: "SAMSUNG MZVL21T0HCLR-00B00 (PM9A1 1TB)", ssdSizeGB: 1024, ssdInterface: "PCIe 4.0 x4 NVMe",
		screenWidth: 2560, screenHeight: 1600, availHeight: 1560, dpr: 1.25, maxTouchPoints: 0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4070 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 130, termRows: 40,
		keyboardModel: "ThinkPad UltraNav Backlit Keyboard (US Layout)", keyboardLayout: "00000409",
		mouseModel: "Synaptics Precision Touchpad & TrackPoint", pointerType: "touchpad",
		hasTrackpad: true, hasTrackpoint: true,
	},
	{
		model: "Dell XPS 15 9530 (Touch)", osVersion: "10.0.22631",
		cpuModel: "13th Gen Intel(R) Core(TM) i7-13620H", cores: 12, threads: 16, cpuFreq: "2.40 GHz",
		memoryGB: 16, memoryType: "DDR5-4800 SODIMM", memoryModel: "Crucial CT8G48C40S5 8GBx2",
		ssdModel: "WD_BLACK SN850X 1000GB NVMe SSD", ssdSizeGB: 1024, ssdInterface: "PCIe 4.0 x4 NVMe",
		screenWidth: 1920, screenHeight: 1200, availHeight: 1160, dpr: 1.0, maxTouchPoints: 10,
		vendor: "Google Inc. (Intel)", renderer: "ANGLE (Intel, Intel(R) Iris(R) Xe Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 120, termRows: 38,
		keyboardModel: "Dell Standard Backlit Keyboard (US Layout)", keyboardLayout: "00000409",
		mouseModel: "Logitech MX Master 3S Wireless Mouse", pointerType: "mouse",
		hasTrackpad: true, hasTrackpoint: false,
	},
	{
		model: "Lenovo Legion Pro 5 16IRX8", osVersion: "10.0.22631",
		cpuModel: "13th Gen Intel(R) Core(TM) i7-13700HX", cores: 16, threads: 24, cpuFreq: "2.10 GHz",
		memoryGB: 32, memoryType: "DDR5-4800 SODIMM", memoryModel: "Samsung M425R2GA3PB0-CWMOL 16GBx2",
		ssdModel: "Micron 3400 MTFDKBA1T0TFH 1024GB", ssdSizeGB: 1024, ssdInterface: "PCIe 4.0 x4 NVMe",
		screenWidth: 2560, screenHeight: 1600, availHeight: 1560, dpr: 1.25, maxTouchPoints: 0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4060 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 140, termRows: 42,
		keyboardModel: "Legion TrueStrike 4-Zone RGB Gaming Keyboard", keyboardLayout: "00000409",
		mouseModel: "Logitech G PRO X SUPERLIGHT Wireless Gaming Mouse", pointerType: "mouse",
		hasTrackpad: true, hasTrackpoint: false,
	},
	{
		model: "ASUS ROG Zephyrus G14", osVersion: "10.0.22631",
		cpuModel: "AMD Ryzen 9 7940HS with Radeon 780M Graphics", cores: 16, threads: 16, cpuFreq: "4.00 GHz",
		memoryGB: 32, memoryType: "LPDDR5X-6400", memoryModel: "Micron LPDDR5X Onboard 32GB",
		ssdModel: "WD PC SN740 NVMe 1024GB", ssdSizeGB: 1024, ssdInterface: "PCIe 4.0 x4 NVMe",
		screenWidth: 2880, screenHeight: 1800, availHeight: 1760, dpr: 1.5, maxTouchPoints: 0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4070 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 135, termRows: 40,
		keyboardModel: "ROG Overstroke Chiclet RGB Keyboard", keyboardLayout: "00000409",
		mouseModel: "Razer DeathAdder V3 Pro Wireless", pointerType: "mouse",
		hasTrackpad: true, hasTrackpoint: false,
	},
	{
		model: "HP Spectre x360 16", osVersion: "10.0.22631",
		cpuModel: "13th Gen Intel(R) Core(TM) i7-1360P", cores: 12, threads: 16, cpuFreq: "2.20 GHz",
		memoryGB: 16, memoryType: "DDR4-3200 SODIMM", memoryModel: "SK hynix HMA81GS6DJR8N-XN 8GBx2",
		ssdModel: "KIOXIA KBG50ZNV512G 512GB", ssdSizeGB: 512, ssdInterface: "PCIe 4.0 x4 NVMe",
		screenWidth: 3840, screenHeight: 2400, availHeight: 2360, dpr: 2.0, maxTouchPoints: 10,
		vendor: "Google Inc. (Intel)", renderer: "ANGLE (Intel, Intel(R) Arc(TM) A370M Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 125, termRows: 38,
		keyboardModel: "HP Premium Quiet Backlit Keyboard", keyboardLayout: "00000409",
		mouseModel: "Synaptics Precision Glass Touchpad", pointerType: "touchpad",
		hasTrackpad: true, hasTrackpoint: false,
	},
	{
		model: "Custom Desktop (Ryzen 7 7800X3D / RTX 4080)", osVersion: "10.0.22631",
		cpuModel: "AMD Ryzen 7 7800X3D 8-Core Processor", cores: 16, threads: 16, cpuFreq: "4.20 GHz",
		memoryGB: 64, memoryType: "DDR5-6000 UDIMM", memoryModel: "Corsair VENGEANCE RGB DDR5 32GBx2",
		ssdModel: "Solidigm P44 Pro 2048GB NVMe SSD", ssdSizeGB: 2048, ssdInterface: "PCIe 4.0 x4 NVMe",
		screenWidth: 2560, screenHeight: 1440, availHeight: 1400, dpr: 1.0, maxTouchPoints: 0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4080 Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 160, termRows: 48,
		keyboardModel: "Keychron Q1 Pro Wireless Custom Mechanical Keyboard", keyboardLayout: "00000409",
		mouseModel: "Logitech G502 X PLUS LIGHTSPEED Wireless Gaming Mouse", pointerType: "mouse",
		hasTrackpad: false, hasTrackpoint: false,
	},
}

var usGeoCatalog = []usGeoSpec{
	{city: "Los Angeles", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "San Francisco", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9,es;q=0.8"},
	{city: "San Jose", state: "CA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "Seattle", state: "WA", timezone: "America/Los_Angeles", lang: "en-US,en;q=0.9"},
	{city: "New York", state: "NY", timezone: "America/New_York", lang: "en-US,en;q=0.9"},
	{city: "Boston", state: "MA", timezone: "America/New_York", lang: "en-US,en;q=0.9"},
	{city: "Chicago", state: "IL", timezone: "America/Chicago", lang: "en-US,en;q=0.9"},
	{city: "Austin", state: "TX", timezone: "America/Chicago", lang: "en-US,en;q=0.9"},
	{city: "Dallas", state: "TX", timezone: "America/Chicago", lang: "en-US,en;q=0.9,es;q=0.8"},
	{city: "Denver", state: "CO", timezone: "America/Denver", lang: "en-US,en;q=0.9"},
	{city: "Salt Lake City", state: "UT", timezone: "America/Denver", lang: "en-US,en;q=0.9"},
	{city: "Miami", state: "FL", timezone: "America/New_York", lang: "en-US,en;q=0.8,es;q=0.9"},
}

var firstNames = []string{
	"alex", "david", "vlad", "anne", "sarah", "michael", "emily", "jason",
	"daniel", "brian", "chris", "ryan", "kevin", "eric", "lisa", "jessica",
}

var developerProjectNames = []string{
	"ai-agent-service", "cloud-infra-core", "fintech-gateway", "data-pipeline-v2",
	"auth-service", "mobile-api-backend", "developer-portal", "mesh-router",
}

var macInstalledAppPool = [][]string{
	{"Visual Studio Code", "Docker Desktop", "iTerm2", "Slack", "Discord", "Notion", "Spotify", "1Password", "Postman"},
	{"Visual Studio Code", "Warp", "Slack", "Figma", "Notion", "Arc Browser", "Raycast", "Bitwarden"},
	{"Visual Studio Code", "JetBrains Rider", "Docker Desktop", "Zoom", "Slack", "Spotify", "TablePlus"},
}

var winInstalledAppPool = [][]string{
	{"Visual Studio Code", "Visual Studio 2022", "Windows Terminal", "Slack", "Discord", "Docker Desktop", "GitKraken", "PowerToys"},
	{"Visual Studio Code", "Windows Terminal", "Slack", "Notion", "Zoom", "Postman", "Spotify", "1Password"},
	{"Visual Studio Code", "IntelliJ IDEA", "Windows Terminal", "Teams", "Discord", "DBeaver", "Bitwarden"},
}

var macFontPool = []string{"-apple-system", "BlinkMacSystemFont", "SF Pro Display", "Helvetica Neue", "Menlo", "Monaco", "JetBrains Mono", "Fira Code"}
var winFontPool = []string{"Segoe UI", "Segoe UI Variable Text", "Consolas", "Cascadia Code", "Fira Code", "Arial", "Calibri"}

var chromeVersions = [][2]string{
	{"128", "128.0.6613.138"},
	{"128", "128.0.6613.120"},
	{"129", "129.0.6668.59"},
	{"129", "129.0.6668.71"},
}

// ResolveHardwareProfile 基于账号唯一特征进行确定性散列派生出高度拟真的真实电脑 (物理硬件全型号配准)
func ResolveHardwareProfile(accountID int64, seed string) HardwareProfile {
	h := sha256.Sum256([]byte(fmt.Sprintf("account:%d:seed:%s:hardware_v4_dense", accountID, seed)))
	n0 := binary.BigEndian.Uint64(h[0:8])
	n1 := binary.BigEndian.Uint64(h[8:16])
	n2 := binary.BigEndian.Uint64(h[16:24])
	n3 := binary.BigEndian.Uint64(h[24:32])

	isMac := (n0 % 100) < 55
	geo := usGeoCatalog[n1%uint64(len(usGeoCatalog))]
	cv := chromeVersions[n2%uint64(len(chromeVersions))]
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
	p.ChromeMajor = cv[0]
	p.ChromeFull = cv[1]
	p.DownlinkMbps = 80 + int(n3%120) // 80 ~ 200 Mbps
	p.NetworkRTT = 25 + int(n2%30)    // 25 ~ 55 ms
	p.GitBranch = branch
	p.IDEHost = "VSCode 1.96.2"

	if isMac {
		spec := macCatalog[n0%uint64(len(macCatalog))]
		p.DeviceModel = spec.model
		p.OSName = "macOS"
		p.OSVersion = spec.osVersion
		p.Arch = spec.arch
		p.SecPlatform = "macOS"
		p.PlatformUA = "Macintosh; Intel Mac OS X 10_15_7"

		// 硬件型号全装配
		p.CPUModel = spec.cpuModel
		p.CPUCores = spec.cores
		p.CPUThreads = spec.threads
		p.CPUFrequency = spec.cpuFreq

		p.MemoryGB = spec.memoryGB
		p.MemoryType = spec.memoryType
		p.MemoryModel = spec.memoryModel

		p.SSDModel = spec.ssdModel
		p.SSDSizeGB = spec.ssdSizeGB
		p.SSDInterface = spec.ssdInterface
		p.FileSystem = "APFS"

		p.ScreenWidth = spec.screenWidth
		p.ScreenHeight = spec.screenHeight
		p.AvailHeight = spec.availHeight
		p.DPR = spec.dpr
		p.MaxTouchPoints = 0 // Mac 恒无触控
		p.WebGLVendor = "Apple Inc."
		p.WebGLRenderer = spec.renderer
		p.AudioDevice = "MacBook Pro Speakers (Built-in)"
		p.MicDevice = "MacBook Pro Microphone (Built-in)"
		p.TerminalCols = spec.termCols
		p.TerminalRows = spec.termRows
		p.DefaultShell = "/bin/zsh"
		p.HomeDir = fmt.Sprintf("/Users/%s", username)
		p.WorkspacePath = fmt.Sprintf("/Users/%s/Projects/%s", username, projectName)
		p.InstalledApps = macInstalledAppPool[n2%uint64(len(macInstalledAppPool))]
		p.SystemFonts = macFontPool
		p.KeyboardModel = spec.keyboardModel
		p.KeyboardLayout = spec.keyboardLayout
		p.MouseModel = spec.mouseModel
		p.PointerType = spec.pointerType
		p.HasTrackpad = spec.hasTrackpad
		p.HasTrackpoint = spec.hasTrackpoint
	} else {
		spec := winCatalog[n0%uint64(len(winCatalog))]
		p.DeviceModel = spec.model
		p.OSName = "Windows"
		p.OSVersion = spec.osVersion
		p.Arch = "x86_64"
		p.SecPlatform = "Windows"
		p.PlatformUA = "Windows NT 10.0; Win64; x64"

		// 硬件型号全装配
		p.CPUModel = spec.cpuModel
		p.CPUCores = spec.cores
		p.CPUThreads = spec.threads
		p.CPUFrequency = spec.cpuFreq

		p.MemoryGB = spec.memoryGB
		p.MemoryType = spec.memoryType
		p.MemoryModel = spec.memoryModel

		p.SSDModel = spec.ssdModel
		p.SSDSizeGB = spec.ssdSizeGB
		p.SSDInterface = spec.ssdInterface
		p.FileSystem = "NTFS"

		p.ScreenWidth = spec.screenWidth
		p.ScreenHeight = spec.screenHeight
		p.AvailHeight = spec.availHeight
		p.DPR = spec.dpr
		p.MaxTouchPoints = spec.maxTouchPoints
		p.WebGLVendor = spec.vendor
		p.WebGLRenderer = spec.renderer
		p.AudioDevice = "Realtek(R) Audio (Speakers)"
		p.MicDevice = "Realtek(R) Audio (Microphone)"
		p.TerminalCols = spec.termCols
		p.TerminalRows = spec.termRows
		p.DefaultShell = "powershell.exe"
		p.HomeDir = fmt.Sprintf("C:\\Users\\%s", strings.Title(username))
		p.WorkspacePath = fmt.Sprintf("C:\\Users\\%s\\source\\repos\\%s", strings.Title(username), projectName)
		p.InstalledApps = winInstalledAppPool[n2%uint64(len(winInstalledAppPool))]
		p.SystemFonts = winFontPool
		p.KeyboardModel = spec.keyboardModel
		p.KeyboardLayout = spec.keyboardLayout
		p.MouseModel = spec.mouseModel
		p.PointerType = spec.pointerType
		p.HasTrackpad = spec.hasTrackpad
		p.HasTrackpoint = spec.hasTrackpoint
	}

	p.JSHeapLimit = int64(p.MemoryGB) * 1024 * 1024 * 1024 / 4
	p.JSHeapTotal = 60*1024*1024 + int64(n1%40)*1024*1024
	p.JSHeapUsed = p.JSHeapTotal - int64(n2%20)*1024*1024
	p.StorageQuota = int64(p.SSDSizeGB) * 1024 * 1024 * 1024 / 3
	p.StorageUsage = 2*1024*1024*1024 + int64(n0%4)*1024*1024*1024

	// 派生 W3C 分布式追踪链路标头 (与官方客户端 APM 遥测一致)
	traceID := fmt.Sprintf("%016x%016x", n0^n2, n1^n3)
	spanID := fmt.Sprintf("%016x", n2^n3)
	p.Traceparent = fmt.Sprintf("00-%s-%s-01", traceID, spanID)
	p.Baggage = fmt.Sprintf("sentry-environment=production,sentry-release=codex-desktop@%s,sentry-user_id=%s", cv[1], username)

	// 动态时变物理状态 (电池与轻度网络时延扰动)
	nowHour := (n1 % 24)
	if nowHour >= 9 && nowHour <= 18 {
		p.BatteryCharging = true
		p.BatteryLevel = 0.85 + float64(n3%15)/100.0 // 0.85 ~ 1.00
	} else {
		p.BatteryCharging = false
		p.BatteryLevel = 0.65 + float64(n3%25)/100.0 // 0.65 ~ 0.90
	}

	// 第二部分：终端 Shell 交互与开发环境变量链 (Terminal & Shell Ecology)
	p.TermType = "xterm-256color"
	p.ColorTerm = "truecolor"
	if isMac {
		p.ParentProcess = "/bin/zsh"
		p.ShellPlugins = "zsh-autosuggestions,zsh-syntax-highlighting"
		p.EnvPaths = []string{
			fmt.Sprintf("HOME=/Users/%s", username),
			"/opt/homebrew/bin",
			fmt.Sprintf("GOPATH=/Users/%s/go", username),
			fmt.Sprintf("CARGO_HOME=/Users/%s/.cargo", username),
			fmt.Sprintf("NVM_DIR=/Users/%s/.nvm", username),
		}
	} else {
		p.ParentProcess = "pwsh.exe"
		p.ShellPlugins = "PSReadLine"
		p.EnvPaths = []string{
			fmt.Sprintf("USERPROFILE=C:\\Users\\%s", strings.Title(username)),
			fmt.Sprintf("APPDATA=C:\\Users\\%s\\AppData\\Roaming", strings.Title(username)),
			fmt.Sprintf("LOCALAPPDATA=C:\\Users\\%s\\AppData\\Local", strings.Title(username)),
			fmt.Sprintf("GOPATH=C:\\Users\\%s\\go", strings.Title(username)),
		}
	}

	// 第三部分：微观 Git 仓储深层状态 (Git Repository Micro-Context)
	commitH := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:git_head", projectName, seed)))
	p.GitCommitSHA = fmt.Sprintf("%x", commitH)[:40] // 40位标准合法 SHA-1 模拟
	p.GitDirtyFilesCount = 1 + int(n2%3)             // 1 ~ 3 个未暂存文件
	p.GitRemoteOrigin = fmt.Sprintf("git@github.com:enterprise-org/%s.git", projectName)

	// 第五部分：浏览器窗口与刷新率上下文 (Window Activity)
	p.WindowState = "maximized"
	p.DocumentHidden = false
	p.WindowFocused = true
	if isMac {
		if strings.Contains(p.DeviceModel, "Pro 14") || strings.Contains(p.DeviceModel, "Pro 16") {
			p.RefreshRate = 120 // ProMotion 自适应高刷
		} else {
			p.RefreshRate = 60
		}
		p.JA4Fingerprint = "t13d1516h2_8daaf6152771_01669480cb9d"
	} else {
		if strings.Contains(p.DeviceModel, "Legion") || strings.Contains(p.DeviceModel, "ROG") {
			p.RefreshRate = 165 // 游戏高刷屏
		} else {
			p.RefreshRate = 60
		}
		p.JA4Fingerprint = "t13d1516h2_8daaf6152771_b1860882e796"
	}

	return p
}

// ApplyToRequestHeaders 统一将该画像注入 HTTP Headers (完全复刻真实电脑与浏览器)
func (p HardwareProfile) ApplyToRequestHeaders(h http.Header, version string) {
	if h == nil {
		return
	}
	if version == "" {
		version = "0.162.1"
	}

	h.Set("User-Agent", fmt.Sprintf("codex_cli_rs/%s (%s) xterm-256color", version, p.PlatformUA))
	h.Set("Accept-Language", p.AcceptLanguage)
	h.Set("sec-ch-ua-platform", `"`+p.SecPlatform+`"`)
	h.Set("sec-ch-ua-mobile", "?0")
	h.Set("sec-ch-ua", fmt.Sprintf(`"Chromium";v="%s", "Not;A=Brand";v="24", "Google Chrome";v="%s"`, p.ChromeMajor, p.ChromeMajor))
	h.Set("x-codex-client-timezone", p.Timezone)

	// 注入 W3C 遥测追踪标头 (彻底补齐客户端调用上下文)
	if p.Traceparent != "" {
		h.Set("traceparent", p.Traceparent)
	}
	if p.Baggage != "" {
		h.Set("baggage", p.Baggage)
	}
}
