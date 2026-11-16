// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

import type { EpisodeInfo } from './api/client';
import type { TimeWindow } from './investigation';

export type InspectionScope = 'recording' | 'baseline' | 'disruption' | 'cooldown' | 'evaluation' | 'custom';
export const INSPECTION_LABELS: Record<InspectionScope, string> = {
  recording: 'Full recording', baseline: 'Baseline', disruption: 'Disruption', cooldown: 'Cooldown', custom: 'Custom selection', evaluation: 'Healthy evaluation',
};

export function recordingWindow(episode?: EpisodeInfo | null, fallback?: TimeWindow | null): TimeWindow | null {
  const start = Date.parse(episode?.start_time ?? '') / 1000;
  const end = Date.parse(episode?.end_time ?? '') / 1000;
  return Number.isFinite(start) && end > start ? { start, end } : fallback ?? null;
}

export function inspectionWindow(scope: InspectionScope, episode?: EpisodeInfo | null, custom?: TimeWindow | null, full?: TimeWindow | null): TimeWindow | null {
  if (scope === 'recording') return full ?? null;
  if (scope === 'custom') return custom ?? null;
  const phase = scope === 'evaluation' ? episode?.evaluation_window : episode?.[scope];
  if (!phase) return null;
  const start = Date.parse(phase.start) / 1000;
  const end = Date.parse(phase.end) / 1000;
  return Number.isFinite(start) && end > start ? { start, end } : null;
}

/** Pins reserve slots; ranking changes never overwrite a manual choice. */
export function selectMetricGroups(pins: Iterable<string>, candidates: string[], limit: number, maximum = 50) {
  const pinned = [...new Set(pins)].slice(0, maximum);
  const pinnedSet = new Set(pinned);
  const available = [...new Set(candidates)].filter(key => !pinnedSet.has(key));
  const capacity = Math.max(0, maximum - pinned.length);
  const ranked = available.slice(0, Math.min(limit, capacity));
  return {
    pinned, ranked, selected: new Set([...pinned, ...ranked]),
    more: Math.max(0, Math.min(available.length, capacity) - ranked.length),
  };
}
