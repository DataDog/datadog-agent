import { useState, useMemo, useRef, useEffect } from 'react';

interface SeriesInfo {
  namespace: string;
  name: string;
  displayName?: string;  // Optional display name (without aggregation suffix)
}

interface TreeNode {
  name: string;
  fullPath: string;
  children: Map<string, TreeNode>;
  seriesKeys: string[]; // All series keys under this node (for subtree selection)
  isLeaf: boolean;
}

interface SeriesTreeProps {
  series: SeriesInfo[];
  selectedSeries: Set<string>;
  anomalousSources: Set<string>;
  onSelectionChange: (newSelection: Set<string>) => void;
}

// Build a tree structure from series names
function buildTree(series: SeriesInfo[], anomalousSources: Set<string>): TreeNode {
  const root: TreeNode = {
    name: '',
    fullPath: '',
    children: new Map(),
    seriesKeys: [],
    isLeaf: false,
  };

  // Sort series: anomalous first, then alphabetically
  const sorted = [...series].sort((a, b) => {
    const aName = a.displayName ?? a.name;
    const bName = b.displayName ?? b.name;
    const aHas = anomalousSources.has(a.name) || anomalousSources.has(aName);
    const bHas = anomalousSources.has(b.name) || anomalousSources.has(bName);
    if (aHas && !bHas) return -1;
    if (!aHas && bHas) return 1;
    return aName.localeCompare(bName);
  });

  for (const s of sorted) {
    const key = `${s.namespace}/${s.name}`;
    // Use displayName if available, otherwise use name
    const nameForTree = s.displayName ?? s.name;
    // Split on . to create hierarchy (not on : since we stripped the suffix)
    const parts = nameForTree.split('.');

    let current = root;
    let pathSoFar = '';

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      pathSoFar = pathSoFar ? `${pathSoFar}.${part}` : part;

      if (!current.children.has(part)) {
        current.children.set(part, {
          name: part,
          fullPath: pathSoFar,
          children: new Map(),
          seriesKeys: [],
          isLeaf: false,
        });
      }

      current = current.children.get(part)!;
      current.seriesKeys.push(key);
    }

    // Mark the last node as a leaf with the actual series key
    current.isLeaf = true;
  }

  return root;
}

// Get all series keys under a node
function getSeriesKeysUnderNode(node: TreeNode): string[] {
  return node.seriesKeys;
}

interface TreeNodeComponentProps {
  node: TreeNode;
  depth: number;
  selectedSeries: Set<string>;
  anomalousSources: Set<string>;
  onToggleNode: (keys: string[]) => void;
  expandedPaths: Set<string>;
  onToggleExpanded: (path: string) => void;
  seriesNameMap: Map<string, string[]>; // key -> names (including displayName)
}

function TreeNodeComponent({
  node,
  depth,
  selectedSeries,
  anomalousSources,
  onToggleNode,
  expandedPaths,
  onToggleExpanded,
  seriesNameMap,
}: TreeNodeComponentProps) {
  const isExpanded = expandedPaths.has(node.fullPath);
  const hasChildren = node.children.size > 0;
  const keys = getSeriesKeysUnderNode(node);

  // Check selection state
  const allSelected = keys.length > 0 && keys.every((k) => selectedSeries.has(k));
  const someSelected = keys.some((k) => selectedSeries.has(k));

  // Check if any series under this node has anomalies
  const hasAnomaly = keys.some((k) => {
    const names = seriesNameMap.get(k);
    return names && names.some((name) => anomalousSources.has(name));
  });

  const childNodes = Array.from(node.children.values());

  return (
    <div>
      <div
        className={`flex items-center gap-1 py-0.5 px-1 rounded cursor-pointer ${
          hasAnomaly ? 'bg-red-900/20' : 'hover:bg-slate-700/50'
        }`}
        style={{ paddingLeft: `${depth * 12}px` }}
      >
        {/* Expand/collapse toggle */}
        {hasChildren && !node.isLeaf ? (
          <button
            onClick={(e) => {
              e.stopPropagation();
              onToggleExpanded(node.fullPath);
            }}
            className="w-4 h-4 flex items-center justify-center text-slate-500 hover:text-slate-300"
          >
            {isExpanded ? '▼' : '▶'}
          </button>
        ) : (
          <span className="w-4" />
        )}

        {/* Checkbox */}
        <input
          type="checkbox"
          checked={allSelected}
          ref={(el) => {
            if (el) el.indeterminate = someSelected && !allSelected;
          }}
          onChange={() => onToggleNode(keys)}
          className="rounded border-slate-600 bg-slate-700 text-purple-600 focus:ring-purple-500 focus:ring-offset-0"
          onClick={(e) => e.stopPropagation()}
        />

        {/* Label */}
        <span
          className={`text-sm truncate flex-1 ${
            hasAnomaly ? 'text-red-400' : 'text-slate-400'
          }`}
          onClick={() => {
            if (hasChildren && !node.isLeaf) {
              onToggleExpanded(node.fullPath);
            }
          }}
        >
          {node.name}
        </span>

        {/* Count badge for non-leaf nodes */}
        {hasChildren && !node.isLeaf && (
          <span className="text-xs text-slate-500">
            {keys.length}
          </span>
        )}

        {/* Anomaly indicator */}
        {hasAnomaly && <span className="text-red-500 text-xs">!</span>}
      </div>

      {/* Children */}
      {isExpanded && hasChildren && (
        <div>
          {childNodes.map((child) => (
            <TreeNodeComponent
              key={child.fullPath}
              node={child}
              depth={depth + 1}
              selectedSeries={selectedSeries}
              anomalousSources={anomalousSources}
              onToggleNode={onToggleNode}
              expandedPaths={expandedPaths}
              onToggleExpanded={onToggleExpanded}
              seriesNameMap={seriesNameMap}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function SeriesTree({
  series,
  selectedSeries,
  anomalousSources,
  onSelectionChange,
}: SeriesTreeProps) {
  // Track which paths are expanded - start with first level expanded
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(() => {
    const initial = new Set<string>();
    // Auto-expand nodes that have anomalies
    return initial;
  });

  // Build tree structure
  const tree = useMemo(() => buildTree(series, anomalousSources), [series, anomalousSources]);

  // Map from key to series name for anomaly lookup (includes both name and displayName)
  const seriesNameMap = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const s of series) {
      const names = [s.name];
      if (s.displayName) names.push(s.displayName);
      map.set(`${s.namespace}/${s.name}`, names);
    }
    return map;
  }, [series]);

  const toggleExpanded = (path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  const toggleNode = (keys: string[]) => {
    const allSelected = keys.every((k) => selectedSeries.has(k));
    const newSelection = new Set(selectedSeries);

    if (allSelected) {
      // Deselect all
      keys.forEach((k) => newSelection.delete(k));
    } else {
      // Select all
      keys.forEach((k) => newSelection.add(k));
    }

    onSelectionChange(newSelection);
  };

  const expandAll = () => {
    const allPaths = new Set<string>();
    const collectPaths = (node: TreeNode) => {
      if (node.fullPath) allPaths.add(node.fullPath);
      node.children.forEach((child) => collectPaths(child));
    };
    collectPaths(tree);
    setExpandedPaths(allPaths);
  };

  const collapseAll = () => {
    setExpandedPaths(new Set());
  };

  const collapseUnselected = () => {
    const pathsToKeep = new Set<string>();

    // Find paths that lead to selected series
    for (const s of series) {
      const key = `${s.namespace}/${s.name}`;
      if (selectedSeries.has(key)) {
        const nameForTree = s.displayName ?? s.name;
        const parts = nameForTree.split('.');
        let pathSoFar = '';
        for (let i = 0; i < parts.length - 1; i++) {
          pathSoFar = pathSoFar ? `${pathSoFar}.${parts[i]}` : parts[i];
          pathsToKeep.add(pathSoFar);
        }
      }
    }

    setExpandedPaths(pathsToKeep);
  };

  // Track if we've done initial auto-expand and for which series set
  const hasAutoExpanded = useRef(false);
  const prevSeriesLength = useRef(0);

  // Reset auto-expand flag when scenario changes (series array changes significantly)
  useEffect(() => {
    if (Math.abs(series.length - prevSeriesLength.current) > 5) {
      hasAutoExpanded.current = false;
    }
    prevSeriesLength.current = series.length;
  }, [series.length]);

  // Auto-expand to show anomalies on first render only
  useEffect(() => {
    if (hasAutoExpanded.current) return;
    if (anomalousSources.size === 0 || series.length === 0) return;

    hasAutoExpanded.current = true;
    const pathsToExpand = new Set<string>();

    // Find paths that lead to anomalous series
    for (const s of series) {
      const nameForTree = s.displayName ?? s.name;
      if (anomalousSources.has(s.name) || anomalousSources.has(nameForTree)) {
        const parts = nameForTree.split('.');
        let pathSoFar = '';
        for (let i = 0; i < parts.length - 1; i++) {
          pathSoFar = pathSoFar ? `${pathSoFar}.${parts[i]}` : parts[i];
          pathsToExpand.add(pathSoFar);
        }
      }
    }

    if (pathsToExpand.size > 0) {
      setExpandedPaths(pathsToExpand);
    }
  }, [series, anomalousSources]);

  const childNodes = Array.from(tree.children.values());

  return (
    <div className="flex flex-col h-full">
      {/* Expand/Collapse controls */}
      <div className="flex gap-1 mb-2">
        <button
          onClick={expandAll}
          className="text-xs px-2 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
          title="Expand all nodes"
        >
          Expand
        </button>
        <button
          onClick={collapseAll}
          className="text-xs px-2 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
          title="Collapse all nodes"
        >
          Collapse
        </button>
        <button
          onClick={collapseUnselected}
          className="text-xs px-2 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
          title="Collapse unselected subtrees, keep selected visible"
        >
          Focus
        </button>
      </div>

      {/* Tree */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden">
        {childNodes.map((child) => (
          <TreeNodeComponent
            key={child.fullPath}
            node={child}
            depth={0}
            selectedSeries={selectedSeries}
            anomalousSources={anomalousSources}
            onToggleNode={toggleNode}
            expandedPaths={expandedPaths}
            onToggleExpanded={toggleExpanded}
            seriesNameMap={seriesNameMap}
          />
        ))}
      </div>
    </div>
  );
}
