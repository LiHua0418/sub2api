package service

import (
	"encoding/json"
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
		fmt.Printf("  • V8 堆内存配置: 上限 %.2f GB (已分配: %d MB, 存活: %d MB)\n", float64(p.JSHeapLimit)/(1024*1024*1024), p.JSHeapTotal/(1024*1024), p.JSHeapUsed/(1024*1024))
		fmt.Printf("  • 固态硬盘型号 : %s (%d GB, %s, %s)\n", p.SSDModel, p.SSDSizeGB, p.SSDInterface, p.FileSystem)
		fmt.Printf("  • 本地存储配额 : 浏览器持久化配额 %.2f GB (已占用缓存 %.2f GB)\n", float64(p.StorageQuota)/(1024*1024*1024), float64(p.StorageUsage)/(1024*1024*1024))
		fmt.Printf("  • 音频外设标签 : 扬声器: %s | 麦克风: %s\n", p.AudioDevice, p.MicDevice)
		fmt.Printf("  • 开发者工作区 : %s 正在编写 %s (Git: %s)\n", p.Username, p.WorkspacePath, p.GitBranch)
		apps, _ := json.Marshal(p.InstalledApps)
		fmt.Printf("  • 本地安装软件 : %s\n", string(apps))
	}
}
