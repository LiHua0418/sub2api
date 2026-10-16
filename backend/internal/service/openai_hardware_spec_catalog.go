package service

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
)

// HardwareProfile 完整的真实个人电脑物理画像
type HardwareProfile struct {
	DeviceModel    string // 机型名称，如 MacBook Pro 14 (M2 Pro)
	OSName         string // macOS 或 Windows
	OSVersion      string // 具体系统内核或构建号
	Arch           string // arm64 或 x86_64
	PlatformUA     string // UA平台段
	SecPlatform    string // Client Hint: "macOS" 或 "Windows"
	CPUCores       int    // 核心数
	MemoryGB       int    // 物理内存 (GB)
	ScreenWidth    int    // 屏幕物理宽度
	ScreenHeight   int    // 屏幕物理高度
	AvailHeight    int    // 扣除任务栏/顶栏后的工作区高度
	DPR            float64// 设备像素比 (Device Pixel Ratio)
	WebGLVendor    string // WebGL 厂商字符串
	WebGLRenderer  string // 完整的 WebGL 驱动渲染器字符串
	City           string // 美国城市
	State          string // 美国州名代码 (CA, NY, TX 等)
	Timezone       string // 真实本地时区 (America/Los_Angeles 等)
	AcceptLanguage string // 拟真的多语言回退链
	ChromeMajor    string // 主版本号，如 "128"
	ChromeFull     string // 完整浏览器构建号
}

type macDeviceSpec struct {
	model        string
	osVersion    string
	arch         string
	cores        int
	memoryGB     int
	screenWidth  int
	screenHeight int
	availHeight  int
	dpr          float64
	renderer     string
}

type winDeviceSpec struct {
	model        string
	osVersion    string
	cores        int
	memoryGB     int
	screenWidth  int
	screenHeight int
	availHeight  int
	dpr          float64
	vendor       string
	renderer     string
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
		cores: 10, memoryGB: 16, screenWidth: 3024, screenHeight: 1964, availHeight: 1939, dpr: 2.0,
		renderer: "Apple M2 Pro",
	},
	{
		model: "MacBook Air 13 (M2)", osVersion: "14.4.1", arch: "arm64",
		cores: 8, memoryGB: 16, screenWidth: 2560, screenHeight: 1664, availHeight: 1639, dpr: 2.0,
		renderer: "Apple M2",
	},
	{
		model: "MacBook Pro 16 (M3 Max)", osVersion: "14.6.0", arch: "arm64",
		cores: 14, memoryGB: 36, screenWidth: 3456, screenHeight: 2234, availHeight: 2209, dpr: 2.0,
		renderer: "Apple M3 Max",
	},
	{
		model: "Mac Studio (M2 Max)", osVersion: "14.5.0", arch: "arm64",
		cores: 12, memoryGB: 32, screenWidth: 3840, screenHeight: 2160, availHeight: 2135, dpr: 2.0,
		renderer: "Apple M2 Max",
	},
	{
		model: "MacBook Pro 13 (Intel Core i7)", osVersion: "13.6.7", arch: "x86_64",
		cores: 8, memoryGB: 16, screenWidth: 2560, screenHeight: 1600, availHeight: 1575, dpr: 2.0,
		renderer: "Intel(R) Iris(R) Plus Graphics 655",
	},
	{
		model: "iMac 24 (M3)", osVersion: "14.5.0", arch: "arm64",
		cores: 8, memoryGB: 16, screenWidth: 4480, screenHeight: 2520, availHeight: 2495, dpr: 2.0,
		renderer: "Apple M3",
	},
}

var winCatalog = []winDeviceSpec{
	{
		model: "ThinkPad P1 Gen 6", osVersion: "10.0.22631",
		cores: 14, memoryGB: 32, screenWidth: 2560, screenHeight: 1600, availHeight: 1560, dpr: 1.25,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4070 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
	},
	{
		model: "Dell XPS 15 9530", osVersion: "10.0.22631",
		cores: 12, memoryGB: 16, screenWidth: 1920, screenHeight: 1200, availHeight: 1160, dpr: 1.0,
		vendor: "Google Inc. (Intel)", renderer: "ANGLE (Intel, Intel(R) Iris(R) Xe Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)",
	},
	{
		model: "Lenovo Legion Pro 5 16IRX8", osVersion: "10.0.22631",
		cores: 16, memoryGB: 32, screenWidth: 2560, screenHeight: 1600, availHeight: 1560, dpr: 1.25,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4060 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
	},
	{
		model: "ASUS ROG Zephyrus G14", osVersion: "10.0.22631",
		cores: 16, memoryGB: 32, screenWidth: 2880, screenHeight: 1800, availHeight: 1760, dpr: 1.5,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4070 Laptop GPU Direct3D11 vs_5_0 ps_5_0, D3D11)",
	},
	{
		model: "HP Spectre x360 16", osVersion: "10.0.22631",
		cores: 12, memoryGB: 16, screenWidth: 3840, screenHeight: 2400, availHeight: 2360, dpr: 2.0,
		vendor: "Google Inc. (Intel)", renderer: "ANGLE (Intel, Intel(R) Arc(TM) A370M Graphics Direct3D11 vs_5_0 ps_5_0, D3D11)",
	},
	{
		model: "Custom Desktop (Ryzen 7 7800X3D / RTX 4080)", osVersion: "10.0.22631",
		cores: 16, memoryGB: 64, screenWidth: 2560, screenHeight: 1440, availHeight: 1400, dpr: 1.0,
		vendor: "Google Inc. (NVIDIA)", renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4080 Direct3D11 vs_5_0 ps_5_0, D3D11)",
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
	h := sha256.Sum256([]byte(fmt.Sprintf("account:%d:seed:%s:hardware_v2", accountID, seed)))
	n0 := binary.BigEndian.Uint64(h[0:8])
	n1 := binary.BigEndian.Uint64(h[8:16])
	n2 := binary.BigEndian.Uint64(h[16:24])

	// 55% 概率分配为 macOS，45% 分配为 Windows (符合主流 AI/代码开发者分布)
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
		p.ScreenWidth = spec.screenWidth
		p.ScreenHeight = spec.screenHeight
		p.AvailHeight = spec.availHeight
		p.DPR = spec.dpr
		p.WebGLVendor = "Apple Inc."
		p.WebGLRenderer = spec.renderer
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
		p.ScreenWidth = spec.screenWidth
		p.ScreenHeight = spec.screenHeight
		p.AvailHeight = spec.availHeight
		p.DPR = spec.dpr
		p.WebGLVendor = spec.vendor
		p.WebGLRenderer = spec.renderer
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
}
