package service

import (
	"fmt"
	"testing"
)

func TestHardwareProfileDenseSimulation(t *testing.T) {
	sampleIDs := []int64{8, 9, 10, 42}
	fmt.Println("=== 验证原生终端纯净画像派生 (符合官方 codex_cli_rs 运行时协议) ===")
	for _, aid := range sampleIDs {
		p := ResolveHardwareProfile(aid, fmt.Sprintf("seed_dense_%d", aid))
		fmt.Printf("\n=======================================================\n")
		fmt.Printf("🖥️ 账号 #%d 派生实体设备: %s (%s / %s)\n", aid, p.DeviceModel, p.OSName, p.Arch)
		fmt.Printf("  • 物理规格标量 : %d 核物理 CPU / %d GB 物理内存\n", p.CPUCores, p.MemoryGB)
		fmt.Printf("  • 终端Shell视图: 窗口尺寸: %dx%d (列x行), 登录Shell: %s\n", p.TerminalCols, p.TerminalRows, p.DefaultShell)
		fmt.Printf("  • 开发者工作区 : %s 正在编写 %s (Git分支: %s)\n", p.Username, p.WorkspacePath, p.GitBranch)
		fmt.Printf("  • 本地地理时区 : %s, %s (时区: %s, 语言: %s)\n", p.City, p.State, p.Timezone, p.AcceptLanguage)
		fmt.Printf("  • W3C遥测追踪  : Traceparent: %s\n", p.Traceparent)
		fmt.Printf("  • W3C行李标头  : Baggage: %s\n", p.Baggage)
	}
}
