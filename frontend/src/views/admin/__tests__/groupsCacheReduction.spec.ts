import { describe, expect, it } from 'vitest';
import {
  cacheReductionDecimalToPercent,
  cacheReductionPercentToDecimal,
  isCacheReductionPlatform,
  validateCacheReductionFormState,
  type CacheReductionFormState,
} from '../groupsCacheReduction';

describe('groupsCacheReduction', () => {
  it('converts percent to decimal correctly', () => {
    expect(cacheReductionPercentToDecimal(10)).toBe(0.1);
    expect(cacheReductionPercentToDecimal(5.5)).toBe(0.055);
    expect(cacheReductionPercentToDecimal(0)).toBe(0);
    expect(cacheReductionPercentToDecimal(-5)).toBe(0);
    expect(cacheReductionPercentToDecimal(null)).toBe(0);
  });

  it('converts decimal to percent correctly', () => {
    expect(cacheReductionDecimalToPercent(0.1)).toBe(10);
    expect(cacheReductionDecimalToPercent(0.055)).toBe(5.5);
    expect(cacheReductionDecimalToPercent(0)).toBe(0);
    expect(cacheReductionDecimalToPercent(null)).toBe(0);
  });

  it('identifies supported platforms', () => {
    expect(isCacheReductionPlatform('openai')).toBe(true);
    expect(isCacheReductionPlatform('composite')).toBe(true);
    expect(isCacheReductionPlatform('anthropic')).toBe(false);
    expect(isCacheReductionPlatform('gemini')).toBe(false);
  });

  it('validates form state', () => {
    const validState: CacheReductionFormState = {
      platform: 'openai',
      cache_reduction_enabled: true,
      cache_reduction_min_ratio_percent: 5,
      cache_reduction_max_ratio_percent: 10,
    };
    expect(validateCacheReductionFormState(validState)).toBeNull();

    const disabledState: CacheReductionFormState = {
      platform: 'openai',
      cache_reduction_enabled: false,
      cache_reduction_min_ratio_percent: 50,
      cache_reduction_max_ratio_percent: 10,
    };
    expect(validateCacheReductionFormState(disabledState)).toBeNull();

    const minGreaterState: CacheReductionFormState = {
      platform: 'openai',
      cache_reduction_enabled: true,
      cache_reduction_min_ratio_percent: 20,
      cache_reduction_max_ratio_percent: 10,
    };
    expect(validateCacheReductionFormState(minGreaterState)).toBe('minGreaterThanMax');

    const outOfRangeState: CacheReductionFormState = {
      platform: 'composite',
      cache_reduction_enabled: true,
      cache_reduction_min_ratio_percent: -5,
      cache_reduction_max_ratio_percent: 10,
    };
    expect(validateCacheReductionFormState(outOfRangeState)).toBe('ratioRangeError');
  });
});
