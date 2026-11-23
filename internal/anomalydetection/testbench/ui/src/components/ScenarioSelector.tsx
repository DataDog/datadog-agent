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
        <span className="relative mt-2 block">
          <select aria-label="Scenario" value={activeScenario ?? ''} disabled={loading || busy || scenarios.length === 0}
            onChange={e => void load(e.target.value)}
            className="peer w-full appearance-none rounded border border-slate-600 bg-slate-900 py-2 pl-2 pr-8 text-sm font-normal normal-case tracking-normal text-slate-100 focus:outline-none focus:ring-2 focus:ring-purple-500 disabled:opacity-50">
            <option value="" disabled>Select a recording…</option>
            {scenarios.map(s => <option key={s.name} value={s.name}>{s.name}</option>)}
          </select>
          <svg aria-hidden="true" className="pointer-events-none absolute right-2 top-1/2 h-3 w-3 -translate-y-1/2 text-slate-400 peer-disabled:opacity-50" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="m4 6 4 4 4-4" />
          </svg>
        </span>
      </label>
      <div className="flex items-center justify-between text-xs text-slate-400">
        <span role="status">{loading || busy ? 'Loading recording and running detectors…' : 'Recorded scenario'}</span>
        <button type="button" disabled={!activeScenario || loading || busy} onClick={() => void load(activeScenario!)} className="text-purple-300 hover:underline disabled:opacity-40">Reload</button>
      </div>
    </div>
  );
}
