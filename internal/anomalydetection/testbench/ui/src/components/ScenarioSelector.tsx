// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

import { useState } from 'react';
import type { ScenarioInfo } from '../api/client';

/** Compact, shared scenario navigation leaves room for each view's tools. */
export function ScenarioSelector({ scenarios, activeScenario, onLoadScenario, busy = false }: {
  busy?: boolean;
  scenarios: ScenarioInfo[];
  activeScenario: string | null;
  onLoadScenario: (name: string) => Promise<void>;
}) {
  const [loading, setLoading] = useState(false);
  const load = async (name: string) => {
    if (!name) return;
    setLoading(true);
    try { await onLoadScenario(name); } finally { setLoading(false); }
  };
  return (
    <div className="p-4 border-b border-slate-700 space-y-2">
      <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider">
        Scenario <span className="font-normal normal-case">· {scenarios.length} recordings</span>
        <select aria-label="Scenario" value={activeScenario ?? ''} disabled={loading || busy || scenarios.length === 0}
          onChange={e => void load(e.target.value)}
          className="mt-2 w-full rounded border border-slate-600 bg-slate-900 p-2 text-sm font-normal normal-case tracking-normal text-slate-100 focus:outline-none focus:ring-2 focus:ring-purple-500 disabled:opacity-50">
          <option value="" disabled>Select a recording…</option>
          {scenarios.map(s => <option key={s.name} value={s.name}>{s.name}</option>)}
        </select>
      </label>
      <div className="flex items-center justify-between text-xs text-slate-400">
        <span role="status">{loading || busy ? 'Loading recording and running detectors…' : 'Recorded scenario'}</span>
        <button type="button" disabled={!activeScenario || loading || busy} onClick={() => void load(activeScenario!)} className="text-purple-300 hover:underline disabled:opacity-40">Reload</button>
      </div>
    </div>
  );
}
