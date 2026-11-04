import { useState } from 'react';
import { parseTagFilter } from '../filters';

const TAG_PREVIEW_COUNT = 3;

function levelBadgeColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'error':   return 'text-red-400 bg-red-900/40';
    case 'warn':
    case 'warning': return 'text-amber-400 bg-amber-900/40';
    case 'info':    return 'text-blue-400 bg-blue-900/40';
    default:        return 'text-slate-400 bg-slate-700/40';
  }
}

interface TagFilterGroupsProps {
  tagGroups: Map<string, string[]>;
  tagFilterInput: string;
  onToggleTag: (tag: string) => void;
  accentColor?: 'purple' | 'teal';
  /** When true, the "status" key gets level-badge styling and shows only the value */
  statusAware?: boolean;
}

export function TagFilterGroups({
  tagGroups,
  tagFilterInput,
  onToggleTag,
  accentColor = 'purple',
  statusAware = false,
}: TagFilterGroupsProps) {
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());

  if (tagGroups.size === 0) return null;

  const { include: activeTags, exclude: excludedTags } = parseTagFilter(tagFilterInput);

  const activeAccent = accentColor === 'teal'
    ? 'bg-teal-600/40 text-teal-300 ring-teal-500/60'
    : 'bg-purple-600/40 text-purple-300 ring-purple-500/60';

  const toggleExpanded = (key: string) =>
    setExpandedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });

  return (
    <div className="space-y-2">
      {[...tagGroups.entries()].map(([key, tags]) => {
        const isStatus = statusAware && key === 'status';
        const isExpanded = expandedKeys.has(key);
        const visibleTags = tags.length > TAG_PREVIEW_COUNT && !isExpanded
          ? tags.slice(0, TAG_PREVIEW_COUNT)
          : tags;
        const hiddenCount = tags.length - TAG_PREVIEW_COUNT;

        return (
          <div key={key}>
            <div className="text-[10px] text-slate-500 mb-1">{key}</div>
            <div className="flex flex-wrap gap-1">
              {visibleTags.map((tag) => {
                const active = activeTags.get(key)?.has(tag) ?? false;
                const excluded = excludedTags.has(tag) || excludedTags.has(key);
                const value = tag.slice(tag.indexOf(':') + 1);

                let cls: string;
                if (isStatus) {
                  cls = `text-[10px] px-1.5 py-0.5 rounded font-mono font-medium uppercase transition-colors ring-1 ${
                    excluded ? 'bg-red-600/40 text-red-300 ring-red-500/60'
                    : active  ? `${levelBadgeColor(value)} ring-teal-500/60`
                    :           `${levelBadgeColor(value)} ring-transparent opacity-60 hover:opacity-100`
                  }`;
                } else {
                  cls = `text-[10px] px-1.5 py-0.5 rounded font-mono transition-colors ${
                    accentColor === 'teal' ? 'ring-1 font-medium ' : ''
                  }${
                    excluded ? 'bg-red-600/40 text-red-300 ring-1 ring-red-500/60'
                    : active  ? `${activeAccent} ring-1`
                    : accentColor === 'teal'
                      ? 'bg-slate-700 text-slate-400 ring-transparent hover:bg-slate-600 hover:text-slate-300'
                      : 'bg-slate-700 text-slate-400 hover:bg-slate-600 hover:text-slate-300'
                  }`;
                }

                return (
                  <button key={tag} onClick={() => onToggleTag(tag)} className={cls}>
                    {isStatus ? value : tag}
                  </button>
                );
              })}

              {tags.length > TAG_PREVIEW_COUNT && (
                <button
                  onClick={() => toggleExpanded(key)}
                  className="text-[10px] px-1.5 py-0.5 rounded font-mono text-slate-500 hover:text-slate-300 bg-slate-700/50 hover:bg-slate-700 transition-colors"
                >
                  {isExpanded ? '− less' : `+${hiddenCount} more`}
                </button>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
