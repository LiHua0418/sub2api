// 分组缓存命中削减表单辅助：百分比 <-> 小数换算与提交前校验。
// 后端按小数存 decimal(10,4)（0.10 = 10%），界面按百分比输入展示；
// 固定 4 位小数精度，避免浮点尾数回显。

export const cacheReductionPercentToDecimal = (
  value: number | string | null | undefined,
): number => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return 0;
  }
  return Math.round(parsed * 100) / 10000;
};

export const cacheReductionDecimalToPercent = (
  value: number | null | undefined,
): number => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return 0;
  }
  return Math.round(parsed * 1e6) / 1e4;
};

export type CacheReductionFormState = {
  platform: string;
  cache_reduction_enabled: boolean;
  cache_reduction_min_ratio_percent: number | string | null;
  cache_reduction_max_ratio_percent: number | string | null;
};

export const isCacheReductionPlatform = (platform: string): boolean =>
  ["openai", "composite"].includes(platform);

export const validateCacheReductionFormState = (
  form: CacheReductionFormState,
): string | null => {
  if (!isCacheReductionPlatform(form.platform) || !form.cache_reduction_enabled) {
    return null;
  }
  const minPercent = Number(form.cache_reduction_min_ratio_percent || 0);
  const maxPercent = Number(form.cache_reduction_max_ratio_percent || 0);
  if (!Number.isFinite(minPercent) || minPercent < 0 || minPercent > 100) {
    return "ratioRangeError";
  }
  if (!Number.isFinite(maxPercent) || maxPercent < 0 || maxPercent > 100) {
    return "ratioRangeError";
  }
  const minRatio = cacheReductionPercentToDecimal(minPercent);
  const maxRatio = cacheReductionPercentToDecimal(maxPercent);
  if (minRatio < 0 || minRatio > 1 || maxRatio < 0 || maxRatio > 1) {
    return "ratioRangeError";
  }
  if (minRatio > maxRatio) {
    return "minGreaterThanMax";
  }
  return null;
};
