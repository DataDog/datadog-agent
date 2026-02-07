import { useState } from 'react';

interface CorrelatorSectionProps {
  name: string;
  displayName: string;
  enabled: boolean;
  data: unknown;
  onToggle: () => void;
}

// Format a number for display
function formatNumber(v: number): string {
  if (Number.isInteger(v)) return v.toString();
  if (Math.abs(v) < 0.01 || Math.abs(v) > 10000) return v.toExponential(2);
  return v.toFixed(2);
}

// Format a field name from camelCase/PascalCase to Title Case
function formatFieldName(key: string): string {
  return key
    .replace(/_/g, ' ')
    .replace(/([A-Z])/g, ' $1')
    .replace(/^./, (s) => s.toUpperCase())
    .trim();
}

// Render a value as a formatted string
function renderValue(value: unknown): string {
  if (typeof value === 'number') return formatNumber(value);
  if (typeof value === 'boolean') return value ? 'Yes' : 'No';
  if (typeof value === 'string') return value;
  return String(value);
}

// Detect if data is an array of objects (table-like)
function isTableData(data: unknown): data is Record<string, unknown>[] {
  if (!Array.isArray(data)) return false;
  if (data.length === 0) return true;
  return typeof data[0] === 'object' && data[0] !== null && !Array.isArray(data[0]);
}

// Get column headers from array of objects
function getColumns(data: Record<string, unknown>[]): string[] {
  if (data.length === 0) return [];
  // Use first row's keys as columns
  return Object.keys(data[0]);
}

export function CorrelatorSection({
  name,
  displayName,
  enabled,
  data,
  onToggle,
}: CorrelatorSectionProps) {
  const [expanded, setExpanded] = useState(false);

  const tableData = isTableData(data) ? data : null;
  const itemCount = tableData ? tableData.length : 0;

  return (
    <div className="bg-slate-800 rounded-lg">
      {/* Header */}
      <div className="flex items-center p-4">
        {/* Toggle switch */}
        <button
          onClick={(e) => {
            e.stopPropagation();
            onToggle();
          }}
          className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors mr-3 flex-shrink-0 ${
            enabled ? 'bg-purple-600' : 'bg-slate-600'
          }`}
        >
          <span
            className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
              enabled ? 'translate-x-5' : 'translate-x-1'
            }`}
          />
        </button>

        {/* Expand/collapse button */}
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex-1 flex items-center gap-2 hover:bg-slate-700/30 rounded -my-2 py-2 -mx-1 px-1"
        >
          <span className="text-slate-500">{expanded ? '▼' : '▶'}</span>
          <h2 className="text-sm font-semibold text-slate-300">{displayName}</h2>
          {itemCount > 0 && (
            <span className="text-xs px-1.5 py-0.5 bg-slate-700 rounded text-slate-400">
              {itemCount}
            </span>
          )}
        </button>
      </div>

      {/* Content */}
      {expanded && enabled && tableData && tableData.length > 0 && (
        <div className="px-4 pb-4">
          <GenericTable data={tableData} name={name} />
        </div>
      )}
      {expanded && enabled && (!tableData || (tableData && tableData.length === 0)) && (
        <div className="px-4 pb-4 text-sm text-slate-500">
          No data available yet
        </div>
      )}
      {expanded && !enabled && (
        <div className="px-4 pb-4 text-sm text-slate-500">
          Disabled - toggle to enable
        </div>
      )}
    </div>
  );
}

function GenericTable({ data, name }: { data: Record<string, unknown>[]; name: string }) {
  const columns = getColumns(data);
  const maxRows = 500;
  const displayData = data.length > maxRows ? data.slice(0, maxRows) : data;

  // Sort by the most meaningful numeric column (last numeric column, usually score/frequency/confidence)
  const sortedData = [...displayData].sort((a, b) => {
    // Find a good sort column - prefer known ones, then last numeric
    const sortCols = columns.filter((c) => typeof a[c] === 'number');
    const preferredSort = ['confidence', 'Frequency', 'lift', 'frequency', 'score'].find(
      (c) => sortCols.includes(c)
    );
    const sortCol = preferredSort ?? sortCols[sortCols.length - 1];
    if (!sortCol) return 0;
    return (b[sortCol] as number) - (a[sortCol] as number);
  });

  return (
    <div>
      {/* Column headers */}
      <div
        className="flex items-center gap-2 px-2 py-1.5 text-xs text-slate-500 border-b border-slate-700 mb-1"
      >
        {columns.map((col) => (
          <span key={col} className="flex-1 min-w-0 truncate">
            {formatFieldName(col)}
          </span>
        ))}
      </div>

      {/* Rows */}
      <div className="max-h-64 overflow-y-auto space-y-1">
        {sortedData.map((row, i) => (
          <div
            key={`${name}-${i}`}
            className="flex items-center gap-2 p-2 bg-slate-700/30 rounded text-sm"
          >
            {columns.map((col) => (
              <span
                key={col}
                className="flex-1 min-w-0 truncate text-xs text-slate-300 font-mono"
                title={String(row[col] ?? '')}
              >
                {renderValue(row[col])}
              </span>
            ))}
          </div>
        ))}
      </div>

      {data.length > maxRows && (
        <div className="text-xs text-slate-500 mt-2">
          Showing top {maxRows} of {data.length} rows
        </div>
      )}
    </div>
  );
}
