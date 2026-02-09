import { useState, useMemo, useRef, useEffect } from 'react';

interface SeriesInfo {
  key: string;
  name: string;
  displayName?: string;
}

interface TreeNode {
  name: string;
  fullPath: string;
  children: Map<string, TreeNode>;
  seriesKeys: string[];
  isLeaf: boolean;
}

interface SeriesTreeProps {
  series: SeriesInfo[];
  selectedSeries: Set<string>;
  anomalousSources: Set<string>;
  onSelectionChange: (newSelection: Set<string>) => void;
}

function buildTree(series: SeriesInfo[], anomalousSources: Set<string>): TreeNode {
  const root: TreeNode = {
    name: '',
    fullPath: '',
    children: new Map(),
    seriesKeys: [],
    isLeaf: false,
  };

  const sorted = [...series].sort((a, b) => {
    const aName = a.displayName ?? a.name;
    const bName = b.displayName ?? b.name;
    const aHas = anomalousSources.has(a.key);
    const bHas = anomalousSources.has(b.key);
    if (aHas && !bHas) return -1;
    if (!aHas && bHas) return 1;
    return aName.localeCompare(bName);
  });

  for (const s of sorted) {
    const key = s.key;
    const nameForTree = s.displayName ?? s.name;
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

    current.isLeaf = true;
  }

  return root;
}

interface TreeNodeComponentProps {
  node: TreeNode;
  depth: number;
  selectedSeries: Set<string>;
  anomalousSources: Set<string>;
  onToggleNode: (keys: string[]) => void;
  expandedPaths: Set<string>;
  onToggleExpanded: (path: string) => void;
}

function TreeNodeComponent({
  node,
  depth,
  selectedSeries,
  anomalousSources,
  onToggleNode,
  expandedPaths,
  onToggleExpanded,
}: TreeNodeComponentProps) {
  const isExpanded = expandedPaths.has(node.fullPath);
  const hasChildren = node.children.size > 0;
  const keys = node.seriesKeys;

  const allSelected = keys.length > 0 && keys.every((k) => selectedSeries.has(k));
  const someSelected = keys.some((k) => selectedSeries.has(k));
  const hasAnomaly = keys.some((k) => anomalousSources.has(k));

  const childNodes = Array.from(node.children.values());

  return (
    <div>
      <div
        className={`flex items-center gap-1 py-0.5 px-1 rounded cursor-pointer ${
          hasAnomaly ? 'bg-red-900/20' : 'hover:bg-slate-700/50'
        }`}
        style={{ paddingLeft: `${depth * 12}px` }}
      >
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

        <span
          className={`text-sm truncate flex-1 ${hasAnomaly ? 'text-red-400' : 'text-slate-400'}`}
          onClick={() => {
            if (hasChildren && !node.isLeaf) {
              onToggleExpanded(node.fullPath);
            }
          }}
        >
          {node.name}
        </span>

        {hasChildren && !node.isLeaf && (
          <span className="text-xs text-slate-500">{keys.length}</span>
        )}

        {hasAnomaly && <span className="text-red-500 text-xs">!</span>}
      </div>

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
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());

  const tree = useMemo(() => buildTree(series, anomalousSources), [series, anomalousSources]);

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
      keys.forEach((k) => newSelection.delete(k));
    } else {
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

    for (const s of series) {
      if (selectedSeries.has(s.key)) {
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

  const hasAutoExpanded = useRef(false);
  const prevSeriesLength = useRef(0);

  useEffect(() => {
    if (Math.abs(series.length - prevSeriesLength.current) > 5) {
      hasAutoExpanded.current = false;
    }
    prevSeriesLength.current = series.length;
  }, [series.length]);

  useEffect(() => {
    if (hasAutoExpanded.current) return;
    if (anomalousSources.size === 0 || series.length === 0) return;

    hasAutoExpanded.current = true;
    const pathsToExpand = new Set<string>();

    for (const s of series) {
      if (anomalousSources.has(s.key)) {
        const nameForTree = s.displayName ?? s.name;
        const parts = nameForTree.split('.');
        let pathSoFar = '';
        for (let i = 0; i < parts.length - 1; i++) {
          pathSoFar = pathSoFar ? `${pathSoFar}.${parts[i]}` : parts[i];
          pathsToExpand.add(pathSoFar);
        }
      }
    }

    setExpandedPaths(pathsToExpand);
  }, [anomalousSources, series]);

  if (series.length === 0) {
    return <div className="text-sm text-slate-500">No series available</div>;
  }

  return (
    <div className="flex flex-col h-full min-h-0">
      <div className="flex gap-1 mb-2 flex-wrap flex-shrink-0">
        <button
          onClick={expandAll}
          className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
          title="Expand all"
        >
          +
        </button>
        <button
          onClick={collapseAll}
          className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
          title="Collapse all"
        >
          -
        </button>
        <button
          onClick={collapseUnselected}
          className="text-xs px-1.5 py-0.5 bg-slate-700 hover:bg-slate-600 rounded text-slate-400"
          title="Focus selected"
        >
          Focus
        </button>
      </div>

      <div className="overflow-y-auto flex-1 min-h-0 pr-1">
        {Array.from(tree.children.values()).map((child) => (
          <TreeNodeComponent
            key={child.fullPath}
            node={child}
            depth={0}
            selectedSeries={selectedSeries}
            anomalousSources={anomalousSources}
            onToggleNode={toggleNode}
            expandedPaths={expandedPaths}
            onToggleExpanded={toggleExpanded}
          />
        ))}
      </div>
    </div>
  );
}
