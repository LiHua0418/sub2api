package service

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
)

// HardwareProfile 完整的真实个人电脑物理画像 (军工级全自洽)
type HardwareProfile struct {
	DeviceModel    string  // 机型名称，如 MacBook Pro 14 (M2 Pro)
	OSName         string  // macOS 或 Windows
	OSVersion      string  // 具体系统内核或构建号
	Arch           string  // arm64 或 x86_64
	PlatformUA     string  // UA平台段
	SecPlatform    string  // Client Hint: "macOS" 或 "Windows"
	CPUCores       int     // 核心数
	MemoryGB       int     // 物理内存 (GB)
	JSHeapLimit    int64   // V8 引擎 JS Heap 内存上限 (字节)
	JSHeapTotal    int64   // 当前已分配堆大小
	JSHeapUsed     int64   // 实际存活对象大小
	SSDSizeGB      int     // 固态硬盘物理容量 (512 / 1024 / 2048)
	FileSystem     string  // 文件系统 (APFS / NTFS)
	StorageQuota   int64   // 浏览器持久化配额 (字节)
	StorageUsage   int64   // 当前已用缓存 (字节)
	ScreenWidth    int     // 屏幕物理宽度
	ScreenHeight   int     // 屏幕物理高度
	AvailHeight    int     // 扣除任务栏/顶栏后的工作区高度
	DPR            float64 // 设备像素比 (Device Pixel Ratio)
	MaxTouchPoints int     // 触控点数 (Mac恒为0，Win部分触控为10)
	WebGLVendor    string  // WebGL 厂商字符串
	WebGLRenderer  string  // 完整的 WebGL 驱动渲染器字符串
	AudioDevice    string  // 扬声器外设标签
	MicDevice      string  // 麦克风外设标签
	TerminalCols   int     // 终端列数
	TerminalRows   int     // 终端行数
	DefaultShell   string  // 默认登录 Shell (/bin/zsh 或 powershell.exe)
	DownlinkMbps   int     // 网络接口估计带宽 (Mbps)
	NetworkRTT     int     // 真实 RTT 时延 (ms)
	City           string  // 美国城市
	State          string  // 美国州名代码 (CA, NY, TX 等)
	Timezone       string  // 真实本地时区 (America/Los_Angeles 等)
	AcceptLanguage string  // 拟真的多语言回退链
	ChromeMajor    string  // 主版本号，如 "128"
	ChromeFull     string  // 完整浏览器构建号
}

type macDeviceSpec struct {
	model        string
	osVersion    string
	arch         string
	cores        int
	memoryGB     int
	ssdSizeGB    int
	screenWidth  int
	screenHeight int
	availHeight  int
	dpr          float64
	renderer     string
	termCols     int
	termRows     int
}

type winDeviceSpec struct {
	model          string
	osVersion      string
	cores          int
	memoryGB       int
	ssdSizeGB      int
	screenWidth    int
	screenHeight   int
	availHeight    int
	dpr            float64
	maxTouchPoints int
	vendor         string
	renderer       string
	termCols       int
	termRows       int
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
		cores: 10, memoryGB: 16, ssdSizeGB: 512,
		screenWidth: 3024, screenHeight: 1964, availHeight: 1939, dpr: 2.0,
		renderer: "Apple M2 Pro", termCols: 140, termRows: 45,
	},
	{
		model: "MacBook Air 13 (M2)", osVersion: "14.4.1", arch: "arm64",
		cores: 8, memoryGB: 16, ssdSizeGB: 512,
		screenWidth: 2560, screenHeight: 1664, availHeight: 1639, dpr: 2.0,
		renderer: "Apple M2", termCols: 120, termRows: 38,
	},
	{
		model: "MacBook Pro 16 (M3 Max)", osVersion: "14.6.0", arch: "arm64",
		cores: 14, memoryGB: 36, ssdSizeGB: 1024,
		screenWidth: 3456, screenHeight: 2234, availHeight: 2209, dpr: 2.0,
		renderer: "Apple M3 Max", termCols: 160, termRows: 50,
	},
	{
		model: "Mac Studio (M2 Max)", osVersion: "14.5.0", arch: "arm64",
		cores: 12, memoryGB: 32, ssdSizeGB: 1024,
		screenWidth: 3840, screenHeight: 2160, availHeight: 2135, dpr: 2.0,
		renderer: "Apple M2 Max", termCols: 160, termRows: 48,
	},
	{
		model: "MacBook Pro 13 (Intel Core i7)", osVersion: "13.6.7", arch: "x86_64",
		cores: 8, memoryGB: 16, ssdSizeGB: 512,
		screenWidth: 2560, screenHeight: 1600, availHeight: 1575, dpr: 2.0,
		renderer: "Intel(R) Iris(R) Plus Graphics 655", termCols: 120, termRows: 36,
	},
	{
		model: "iMac 24 (M3)", osVersion: "14.5.0", arch: "arm64",
		cores: 8, memoryGB: 16, ssdSizeGB: 512,
		screenWidth: 4480, screenHeight: 2520, availHeight: 2495, dpr: 2.0,
		renderer: "Apple M3", termCols: 150, termRows: 46,
	},
}

var winCatalog = []winDeviceSpec{
	{
		model: "ThinkPad P1 Gen 6", osVersion: "10.0.22631",
		cores: 14, memoryGB: 32, ssdSizeGB: 1024,
		screenWidth: 2560, screenHeight: 1600, availHeight: 1560, dpr: 1.25, maxTouchPoints: 0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4070 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 130, termRows: 40,
	},
	{
		model: "Dell XPS 15 9530 (Touch)", osVersion: "10.0.22631",
		cores: 12, memoryGB: 16, ssdSizeGB: 1024,
		screenWidth: 1920, screenHeight: 1200, availHeight: 1160, dpr: 1.0, maxTouchPoints: 10,
		vendor: "Google Inc. (Intel)", renderer: "ANGLE (Intel, Intel(R) Iris(R) Xe Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 120, termRows: 38,
	},
	{
		model: "Lenovo Legion Pro 5 16IRX8", osVersion: "10.0.22631",
		cores: 16, memoryGB: 32, ssdSizeGB: 1024,
		screenWidth: 2560, screenHeight: 1600, availHeight: 1560, dpr: 1.25, maxTouchPoints: 0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4060 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 140, termRows: 42,
	},
	{
		model: "ASUS ROG Zephyrus G14", osVersion: "10.0.22631",
		cores: 16, memoryGB: 32, ssdSizeGB: 1024,
		screenWidth: 2880, screenHeight: 1800, availHeight: 1760, dpr: 1.5, maxTouchPoints: 0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4070 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 135, termRows: 40,
	},
	{
		model: "HP Spectre x360 16", osVersion: "10.0.22631",
		cores: 12, memoryGB: 16, ssdSizeGB: 512,
		screenWidth: 3840, screenHeight: 2400, availHeight: 2360, dpr: 2.0, maxTouchPoints: 10,
		vendor: "Google Inc. (Intel)", renderer: "ANGLE (Intel, Intel(R) Arc(TM) A370M Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 125, termRows: 38,
	},
	{
		model: "Custom Desktop (Ryzen 7 7800X3D / RTX 4080)", osVersion: "10.0.22631",
		cores: 16, memoryGB: 64, ssdSizeGB: 2048,
		screenWidth: 2560, screenHeight: 1440, availHeight: 1400, dpr: 1.0, maxTouchPoints: 0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4080 Direct3D11 vs_5_0 ps_5_0, D3D11)",
		termCols: 160, termRows: 48,
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

var chromeVersions = [][2]string{
	{"128", "128.0.6613.138"},
	{"128", "128.0.6613.120"},
	{"129", "129.0.6668.59"},
	{"129", "129.0.6668.71"},
}

// ResolveHardwareProfile 基于账号唯一特征进行确定性散列派生出高度拟真的真实电脑
func ResolveHardwareProfile(accountID int64, seed string) HardwareProfile {
	h := sha256.Sum256([]byte(fmt.Sprintf("account:%d:seed:%s:hardware_v3", accountID, seed)))
	n0 := binary.BigEndian.Uint64(h[0:8])
	n1 := binary.BigEndian.Uint64(h[8:16])
	n2 := binary.BigEndian.Uint64(h[16:24])
	n3 := binary.BigEndian.Uint64(h[24:32])

	isMac := (n0 % 100) < 55
	geo := usGeoCatalog[n1%uint64(len(usGeoCatalog))]
	cv := chromeVersions[n2%uint64(len(chromeVersions))]

	var p HardwareProfile
	p.City = geo.city
	p.State = geo.state
	p.Timezone = geo.timezone
	p.AcceptLanguage = geo.lang
	p.ChromeMajor = cv[0]
	p.ChromeFull = cv[1]
	p.DownlinkMbps = 80 + int(n3%120) // 80 ~ 200 Mbps
	p.NetworkRTT = 25 + int(n2%30)    // 25 ~ 55 ms

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
		p.SSDSizeGB = spec.ssdSizeGB
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
		p.SSDSizeGB = spec.ssdSizeGB
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
	}

	// 内存堆上限与固态存储配额计算 (严格符合 Chromium 算法)
	p.JSHeapLimit = int64(p.MemoryGB) * 1024 * 1024 * 1024 / 4 // 1/4 物理内存为堆限制
	p.JSHeapTotal = 60*1024*1024 + int64(n1%40)*1024*1024
	p.JSHeapUsed = p.JSHeapTotal - int64(n2%20)*1024*1024
	p.StorageQuota = int64(p.SSDSizeGB) * 1024 * 1024 * 1024 / 3 // 约 1/3 磁盘为浏览器存储配额
	p.StorageUsage = 2*1024*1024*1024 + int64(n0%4)*1024*1024*1024 // 2G ~ 6G 历史缓存

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
}
