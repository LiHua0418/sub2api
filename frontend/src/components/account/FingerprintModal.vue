<template>
  <div v-if="show" class="fixed inset-0 z-50 overflow-y-auto" aria-labelledby="modal-title" role="dialog" aria-modal="true">
    <div class="flex min-h-screen items-center justify-center p-4 text-center sm:p-0">
      <div class="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity dark:bg-black dark:bg-opacity-75" aria-hidden="true" @click="emit('close')"></div>

      <div class="relative w-full max-w-3xl transform overflow-hidden rounded-xl border border-gray-200 bg-white p-6 text-left shadow-2xl transition-all dark:border-dark-700 dark:bg-dark-800 sm:my-8">
        <!-- Header -->
        <div class="flex items-center justify-between border-b border-gray-200 pb-4 dark:border-dark-700">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400">
              <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
            </div>
            <div>
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white" id="modal-title">
                {{ account?.name }} - 终端设备指纹画像
              </h3>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                账号 #{{ account?.id }} • 100% 确定性原生终端契约 (codex_cli_rs)
              </p>
            </div>
          </div>
          <button @click="emit('close')" class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-500 dark:hover:bg-dark-700 dark:hover:text-gray-300">
            <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <!-- Body -->
        <div class="mt-4">
          <!-- Loading -->
          <div v-if="loading" class="flex flex-col items-center justify-center py-12">
            <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
            <span class="mt-3 text-xs text-gray-500 dark:text-gray-400">加载设备指纹中...</span>
          </div>

          <!-- Error -->
          <div v-else-if="error" class="rounded-lg bg-red-50 p-4 text-center text-sm text-red-600 dark:bg-red-900/20 dark:text-red-400">
            {{ error }}
          </div>

          <!-- Content Grid -->
          <div v-else-if="data" class="space-y-4">
            <!-- 1. 系统与物理规格 + 终端环境 -->
            <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <!-- 系统硬件 -->
              <div class="rounded-lg border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-900/50">
                <div class="flex items-center gap-2 border-b border-gray-100 pb-2 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  <svg class="h-4 w-4 text-primary-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
                  </svg>
                  系统与物理规格
                </div>
                <div class="mt-3 space-y-2 text-xs">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">实体设备型号</span>
                    <span class="font-medium text-gray-900 dark:text-white">{{ data.hardware.device_model }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">操作系统</span>
                    <span class="font-medium text-gray-900 dark:text-white">{{ data.hardware.os_name }} ({{ data.hardware.os_version }})</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">CPU 架构 / 核心</span>
                    <span class="font-medium text-gray-900 dark:text-white">{{ data.hardware.arch }} ({{ data.hardware.cpu_cores }} 核心)</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">物理内存容量</span>
                    <span class="font-medium text-gray-900 dark:text-white">{{ data.hardware.memory_gb }} GB</span>
                  </div>
                </div>
              </div>

              <!-- 终端环境 -->
              <div class="rounded-lg border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-900/50">
                <div class="flex items-center gap-2 border-b border-gray-100 pb-2 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  <svg class="h-4 w-4 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
                  </svg>
                  终端 Shell 视图
                </div>
                <div class="mt-3 space-y-2 text-xs">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">窗口尺寸 (Cols x Rows)</span>
                    <span class="font-mono font-medium text-gray-900 dark:text-white">{{ data.terminal.cols }} x {{ data.terminal.rows }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">默认 Shell</span>
                    <span class="font-mono font-medium text-gray-900 dark:text-white">{{ data.terminal.default_shell }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">终端颜色模式</span>
                    <span class="font-medium text-gray-900 dark:text-white">xterm-256color (truecolor)</span>
                  </div>
                  <div class="flex items-center justify-between">
                    <span class="text-gray-500 dark:text-gray-400">User-Agent</span>
                    <button @click="copyText(data.terminal.user_agent)" class="text-primary-600 hover:underline dark:text-primary-400" title="点击复制完整 UA">
                      复制 UA
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- 2. 工作区与地理位置 -->
            <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <!-- 工作区 -->
              <div class="rounded-lg border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-900/50">
                <div class="flex items-center gap-2 border-b border-gray-100 pb-2 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  <svg class="h-4 w-4 text-violet-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                  </svg>
                  工作区与 Git 上下文
                </div>
                <div class="mt-3 space-y-2 text-xs">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">系统用户名</span>
                    <span class="font-medium text-gray-900 dark:text-white">{{ data.workspace.username }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">Git 当前分支</span>
                    <span class="inline-flex items-center rounded bg-gray-200 px-1.5 py-0.5 font-mono text-[11px] text-gray-800 dark:bg-dark-700 dark:text-gray-200">
                      {{ data.workspace.git_branch }}
                    </span>
                  </div>
                  <div class="flex flex-col gap-0.5">
                    <span class="text-gray-500 dark:text-gray-400">项目工作区路径</span>
                    <span class="truncate font-mono text-[11px] text-gray-800 dark:text-gray-300" :title="data.workspace.workspace_path">
                      {{ data.workspace.workspace_path }}
                    </span>
                  </div>
                </div>
              </div>

              <!-- 地理时区 -->
              <div class="rounded-lg border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-900/50">
                <div class="flex items-center gap-2 border-b border-gray-100 pb-2 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  <svg class="h-4 w-4 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17.657 16.657L13.414 20.9a1.998 1.998 0 01-2.827 0l-4.244-4.243a8 8 0 1111.314 0z" />
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 11a3 3 0 11-6 0 3 3 0 016 0z" />
                  </svg>
                  网络出口与时区对齐
                </div>
                <div class="mt-3 space-y-2 text-xs">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">物理位置</span>
                    <span class="font-medium text-gray-900 dark:text-white">{{ data.geo.city }}, {{ data.geo.state }} (USA)</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">本地 IANA 时区</span>
                    <span class="font-mono font-medium text-gray-900 dark:text-white">{{ data.geo.timezone }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">语言首选链</span>
                    <span class="font-medium text-gray-900 dark:text-white">{{ data.geo.accept_language }}</span>
                  </div>
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">出口对齐状态</span>
                    <span class="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                      <span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
                      已物理闭环 (CA/US)
                    </span>
                  </div>
                </div>
              </div>
            </div>

            <!-- 3. 设备与会话标识 -->
            <div class="rounded-lg border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-900/50">
              <div class="flex items-center justify-between border-b border-gray-100 pb-2 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <div class="flex items-center gap-2">
                  <svg class="h-4 w-4 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                  </svg>
                  出站身份标识 (Identifiers)
                </div>
                <span class="text-[11px] font-normal text-gray-400">种子截断: {{ data.identifiers.seed_masked }}</span>
              </div>
              <div class="mt-3 space-y-2 text-xs">
                <div class="flex items-center justify-between gap-4">
                  <span class="w-28 text-gray-500 dark:text-gray-400">Installation ID</span>
                  <span class="flex-1 truncate font-mono text-[11px] text-gray-800 dark:text-gray-300">{{ data.identifiers.installation_id }}</span>
                  <button @click="copyText(data.identifiers.installation_id)" class="text-primary-600 hover:underline dark:text-primary-400">复制</button>
                </div>
                <div class="flex items-center justify-between gap-4">
                  <span class="w-28 text-gray-500 dark:text-gray-400">Session ID</span>
                  <span class="flex-1 truncate font-mono text-[11px] text-gray-800 dark:text-gray-300">{{ data.identifiers.session_id }}</span>
                  <button @click="copyText(data.identifiers.session_id)" class="text-primary-600 hover:underline dark:text-primary-400">复制</button>
                </div>
                <div class="flex items-center justify-between gap-4">
                  <span class="w-28 text-gray-500 dark:text-gray-400">Window ID</span>
                  <span class="flex-1 truncate font-mono text-[11px] text-gray-800 dark:text-gray-300">{{ data.identifiers.window_id }}</span>
                  <button @click="copyText(data.identifiers.window_id)" class="text-primary-600 hover:underline dark:text-primary-400">复制</button>
                </div>
                <div class="flex items-center justify-between gap-4">
                  <span class="w-28 text-gray-500 dark:text-gray-400">W3C Traceparent</span>
                  <span class="flex-1 truncate font-mono text-[11px] text-gray-800 dark:text-gray-300">{{ data.telemetry.traceparent }}</span>
                  <button @click="copyText(data.telemetry.traceparent)" class="text-primary-600 hover:underline dark:text-primary-400">复制</button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Footer -->
        <div class="mt-6 flex justify-end border-t border-gray-200 pt-4 dark:border-dark-700">
          <button @click="emit('close')" class="btn btn-secondary px-4 py-2 text-sm">
            关闭
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { getFingerprint, type AccountFingerprintResponse } from '@/api/admin/accounts'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  show: boolean
  account: any
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const appStore = useAppStore()
const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<AccountFingerprintResponse | null>(null)

const fetchDetails = async () => {
  if (!props.account?.id) return
  loading.value = true
  error.value = null
  try {
    data.value = await getFingerprint(props.account.id)
  } catch (err: any) {
    error.value = err?.response?.data?.message || err?.message || '读取指纹画像失败'
  } finally {
    loading.value = false
  }
}

watch(
  () => props.show,
  (val) => {
    if (val) {
      fetchDetails()
    } else {
      data.value = null
      error.value = null
    }
  }
)

const copyText = async (text: string) => {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess('已复制到剪贴板')
  } catch {
    appStore.showError('复制失败')
  }
}
</script>
