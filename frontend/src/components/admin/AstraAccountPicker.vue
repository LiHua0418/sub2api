<template>
  <fieldset class="min-w-0 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <legend class="px-1 text-sm font-medium text-gray-800 dark:text-gray-100">
        {{ label }} · {{ modelValue.length }}/{{ maxLimit }}
      </legend>
      <div class="flex flex-wrap items-center gap-1.5 text-xs">
        <button
          type="button"
          class="btn btn-secondary btn-xs"
          :disabled="disabled || modelValue.length >= maxLimit || selectableAccounts.length === 0"
          @click="selectAll"
        >
          {{ t('admin.astraGateway.selectAll') }}
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-xs"
          :disabled="disabled || modelValue.length === 0"
          @click="clearAll"
        >
          {{ t('admin.astraGateway.clearAll') }}
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-xs"
          :disabled="disabled"
          @click="showPaste = !showPaste"
        >
          {{ t('admin.astraGateway.batchPaste') }}
        </button>
        <button
          type="button"
          class="btn btn-xs"
          :class="onlySelected ? 'btn-primary' : 'btn-secondary'"
          @click="onlySelected = !onlySelected"
        >
          {{ onlySelected ? t('admin.astraGateway.allAccounts') : t('admin.astraGateway.onlySelected') }} ({{ modelValue.length }})
        </button>
      </div>
    </div>

    <!-- Batch paste panel -->
    <div v-if="showPaste" class="mt-3 rounded-lg border border-primary-200 bg-primary-50/50 p-3 dark:border-primary-900/50 dark:bg-primary-950/20">
      <p class="text-xs text-gray-600 dark:text-gray-300">{{ t('admin.astraGateway.pasteHint') }}</p>
      <textarea
        v-model="pasteText"
        rows="2"
        class="input mt-2 w-full font-mono text-xs"
        placeholder="53901, 53902, 53903"
      ></textarea>
      <div class="mt-2 flex items-center justify-end gap-2">
        <button type="button" class="btn btn-secondary btn-xs" @click="showPaste = false; pasteText = ''">
          {{ t('admin.astraGateway.pasteCancel') }}
        </button>
        <button type="button" class="btn btn-primary btn-xs" :disabled="!pasteText.trim()" @click="handleBatchPaste">
          {{ t('admin.astraGateway.pasteConfirm') }}
        </button>
      </div>
    </div>

    <!-- Selected pills / badges -->
    <div v-if="selectedBadges.length" class="mt-3 flex max-h-24 flex-wrap gap-1.5 overflow-y-auto">
      <span
        v-for="item in selectedBadges"
        :key="item.id"
        class="inline-flex items-center gap-1 rounded-full bg-primary-50 px-2.5 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
      >
        #{{ item.id }} {{ item.name }}
        <button
          type="button"
          class="ml-0.5 hover:text-red-600 focus:outline-none dark:hover:text-red-400"
          :disabled="disabled"
          @click="select(item.id, false)"
        >
          ✕
        </button>
      </span>
    </div>

    <input
      v-model="search"
      type="search"
      class="input mb-2 mt-3 w-full"
      :aria-label="t('admin.astraGateway.search')"
      :placeholder="t('admin.astraGateway.search')"
    />

    <div class="max-h-52 space-y-1 overflow-y-auto">
      <label
        v-for="account in filtered"
        :key="account.id"
        class="flex cursor-pointer items-center justify-between rounded-lg px-2 py-1.5 text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
        :class="{ 'opacity-50 cursor-not-allowed': isExcluded(account.id) }"
      >
        <div class="flex min-w-0 items-center gap-2">
          <input
            type="checkbox"
            :checked="modelValue.includes(account.id)"
            :disabled="disabled || isExcluded(account.id) || (!modelValue.includes(account.id) && modelValue.length >= maxLimit)"
            @change="select(account.id, ($event.target as HTMLInputElement).checked)"
          />
          <span class="min-w-0 break-words text-gray-700 dark:text-gray-200">
            #{{ account.id }} · {{ account.name }}
          </span>
        </div>
        <span
          v-if="isExcluded(account.id)"
          class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400"
        >
          {{ excludeLabel || t('admin.astraGateway.selectedAsTarget') }}
        </span>
      </label>
      <p v-if="!filtered.length" class="py-3 text-sm text-gray-500">{{ t('admin.astraGateway.noAccounts') }}</p>
    </div>

    <div v-if="missing.length" class="mt-3 space-y-2">
      <p class="text-xs text-amber-600">{{ t('admin.astraGateway.missingAccounts') }}</p>
      <button
        v-for="id in missing"
        :key="id"
        type="button"
        class="btn btn-secondary btn-sm mr-2"
        :disabled="disabled"
        @click="select(id, false)"
      >
        #{{ id }} · {{ t('common.remove') }}
      </button>
    </div>
  </fieldset>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

const props = withDefaults(
  defineProps<{
    label: string
    modelValue: number[]
    accounts: { id: number; name: string }[]
    disabled?: boolean
    excludeIds?: number[]
    excludeLabel?: string
    max?: number
  }>(),
  {
    disabled: false,
    excludeIds: () => [],
    excludeLabel: '',
    max: 256
  }
)

const emit = defineEmits<{ (event: 'update:modelValue', value: number[]): void }>()
const { t } = useI18n()

const search = ref('')
const onlySelected = ref(false)
const showPaste = ref(false)
const pasteText = ref('')

const maxLimit = computed(() => props.max || 256)

function isExcluded(id: number): boolean {
  return props.excludeIds ? props.excludeIds.includes(id) : false
}

// Selectable accounts: valid, not excluded
const selectableAccounts = computed(() => {
  return props.accounts.filter(a => !isExcluded(a.id))
})

const filtered = computed(() => {
  const query = search.value.trim().toLowerCase()
  return props.accounts.filter(a => {
    if (onlySelected.value && !props.modelValue.includes(a.id)) {
      return false
    }
    if (!query) return true
    return `${a.id} ${a.name}`.toLowerCase().includes(query)
  })
})

const selectedBadges = computed(() => {
  return props.modelValue.map(id => {
    const acc = props.accounts.find(a => a.id === id)
    return {
      id,
      name: acc ? acc.name : ''
    }
  })
})

const missing = computed(() => props.modelValue.filter(id => !props.accounts.some(a => a.id === id)))

function select(id: number, checked: boolean) {
  if (checked) {
    if (isExcluded(id)) return
    if (props.modelValue.length >= maxLimit.value && !props.modelValue.includes(id)) return
    emit('update:modelValue', [...new Set([...props.modelValue, id])])
  } else {
    emit('update:modelValue', props.modelValue.filter(value => value !== id))
  }
}

function selectAll() {
  const currentSet = new Set(props.modelValue)
  const candidates = filtered.value.filter(a => !isExcluded(a.id))
  for (const acc of candidates) {
    if (currentSet.size >= maxLimit.value) break
    currentSet.add(acc.id)
  }
  emit('update:modelValue', Array.from(currentSet))
}

function clearAll() {
  emit('update:modelValue', [])
}

function handleBatchPaste() {
  const parts = pasteText.value.split(/[\s,，\n]+/).map(s => s.trim().replace(/^#/, '')).filter(Boolean)
  const ids = parts.map(s => parseInt(s, 10)).filter(n => !isNaN(n) && n > 0)
  const currentSet = new Set(props.modelValue)
  for (const id of ids) {
    if (currentSet.size >= maxLimit.value) break
    if (props.accounts.some(a => a.id === id) && !isExcluded(id)) {
      currentSet.add(id)
    }
  }
  emit('update:modelValue', Array.from(currentSet))
  showPaste.value = false
  pasteText.value = ''
}
</script>
