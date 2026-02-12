import type { CompressedGroup } from '../api/client';

// Correlator badge colors
const CORRELATOR_COLORS: Record<string, string> = {
  time_cluster: '#8b5cf6',
  lead_lag: '#3b82f6',
  surprise: '#f59e0b',
  graph_sketch: '#06b6d4',
};

function getCorrelatorColor(name: string): string {
  return CORRELATOR_COLORS[name] ?? '#64748b';
}

function formatTimestamp(ts: number): string {
  const d = new Date(ts * 1000);
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  const ss = String(d.getSeconds()).padStart(2, '0');
  return `${hh}:${mm}:${ss}`;
}

interface CompressedGroupCardProps {
  group: CompressedGroup;
}

export function CompressedGroupCard({ group }: CompressedGroupCardProps) {
  const color = getCorrelatorColor(group.correlator);

  return (
    <div className="rounded-lg p-3 bg-slate-700/40">
      {/* Header: correlator badge + title */}
      <div className="flex items-center gap-2 mb-2">
        <span
          className="text-[10px] px-1.5 py-0.5 rounded-full font-medium"
          style={{ backgroundColor: color + '22', color }}
        >
          {group.correlator.replace(/_/g, ' ')}
        </span>
        <span className="text-sm text-slate-200 font-medium truncate">{group.title}</span>
      </div>

      {/* Common tags */}
      {Object.keys(group.commonTags).length > 0 && (
        <div className="flex flex-wrap gap-1 mb-2">
          {Object.entries(group.commonTags).map(([k, v]) => (
            <span
              key={k}
              className="text-[10px] px-1.5 py-0.5 bg-slate-600/50 rounded text-slate-300"
            >
              {k}={v}
            </span>
          ))}
        </div>
      )}

      {/* Patterns */}
      <div className="space-y-1 mb-2">
        {group.patterns.map((pattern, i) => (
          <div key={i} className="flex items-center gap-2">
            <code className="text-[11px] text-slate-300 font-mono flex-1 truncate" title={pattern.pattern}>
              {pattern.pattern}
            </code>
            <span className="text-[10px] text-slate-500 flex-shrink-0">
              {pattern.matched}/{pattern.universe}
            </span>
            {/* Tiny precision bar */}
            <div className="w-12 h-1.5 bg-slate-600 rounded-full flex-shrink-0 overflow-hidden">
              <div
                className="h-full rounded-full"
                style={{
                  width: `${Math.round(pattern.precision * 100)}%`,
                  backgroundColor: pattern.precision >= 0.75 ? '#22c55e' : pattern.precision >= 0.5 ? '#f59e0b' : '#ef4444',
                }}
              />
            </div>
          </div>
        ))}
      </div>

      {/* Time range */}
      {group.firstSeen != null && group.lastUpdated != null && group.firstSeen > 0 && (
        <div className="text-[10px] text-slate-400 mb-1 font-mono">
          {formatTimestamp(group.firstSeen)} – {formatTimestamp(group.lastUpdated)}
          <span className="text-slate-500 ml-1">
            ({group.lastUpdated - group.firstSeen}s span)
          </span>
        </div>
      )}

      {/* Footer */}
      <div className="flex items-center justify-between text-[10px] text-slate-500">
        <span>{group.seriesCount} series</span>
        <span>{Math.round(group.precision * 100)}% precision</span>
      </div>
    </div>
  );
}
