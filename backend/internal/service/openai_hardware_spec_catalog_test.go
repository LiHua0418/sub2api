package service

import (
	"fmt"
	"testing"
)

func TestHardwareProfileDenseSimulation(t *testing.T) {
	sampleIDs := []int64{8, 9, 10, 42}
	fmt.Println("=== 验证全物理硬件深层部件规格派生 (CPU型号/内存代际/固态硬盘型号) ===")
	for _, aid := range sampleIDs {
		p := ResolveHardwareProfile(aid, fmt.Sprintf("seed_dense_%d", aid))
		fmt.Printf("\n=======================================================\n")
		fmt.Printf("🖥️ 账号 #%d 派生实体设备: %s\n", aid, p.DeviceModel)
		fmt.Printf("  • CPU具体型号 : %s (%d核/%d线程, %s)\n", p.CPUModel, p.CPUCores, p.CPUThreads, p.CPUFrequency)
		fmt.Printf("  • 物理内存规格 : %d GB %s (颗粒/条型号: %s)\n", p.MemoryGB, p.MemoryType, p.MemoryModel)
		fmt.Printf("  • 屏幕物理显示 : %dx%d (可用高: %d, DPR: %.2f, 刷新率: %d Hz)\n", p.ScreenWidth, p.ScreenHeight, p.AvailHeight, p.DPR, p.RefreshRate)
		fmt.Printf("  • 固态硬盘型号 : %s (%d GB, %s, %s)\n", p.SSDModel, p.SSDSizeGB, p.SSDInterface, p.FileSystem)
		fmt.Printf("  • 本地存储配额 : 浏览器持久化配额 %.2f GB (已占用缓存 %.2f GB)\n", float64(p.StorageQuota)/(1024*1024*1024), float64(p.StorageUsage)/(1024*1024*1024))
		fmt.Printf("  • 终端Shell环境: 父进程: %s (窗口: %dx%d, 类型: %s, 颜色: %s)\n", p.ParentProcess, p.TerminalCols, p.TerminalRows, p.TermType, p.ColorTerm)
		fmt.Printf("  • 开发者工作区 : %s 正在编写 %s (Git分支: %s, 脏文件: %d个)\n", p.Username, p.WorkspacePath, p.GitBranch, p.GitDirtyFilesCount)
		fmt.Printf("  • Git提交追踪  : Commit: %s (Remote: %s)\n", p.GitCommitSHA, p.GitRemoteOrigin)
		fmt.Printf("  • W3C遥测追踪  : Traceparent: %s\n", p.Traceparent)
		fmt.Printf("  • W3C行李标头  : Baggage: %s\n", p.Baggage)
	}
}
