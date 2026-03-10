import { useEffect, useState, useCallback, useRef } from "react";
import { getLog } from "../commands/agent";

const LINES_PER_PAGE = 200;

export function LogPage() {
  const [allLines, setAllLines] = useState<string[]>([]);
  const [visibleCount, setVisibleCount] = useState(LINES_PER_PAGE);
  const [loading, setLoading] = useState(true);
  const [order, setOrder] = useState<"recent" | "oldest">("recent");
  const [error, setError] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const fetchLog = useCallback(async (mostRecentFirst: boolean) => {
    setLoading(true);
    setError(null);
    try {
      const lines = await getLog(mostRecentFirst);
      setAllLines(lines);
      setVisibleCount(LINES_PER_PAGE);
    } catch (e: unknown) {
      setError(typeof e === "string" ? e : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchLog(order === "recent");
  }, [order, fetchLog]);

  const loadMore = () => {
    setVisibleCount((v) => v + LINES_PER_PAGE);
  };

  const visibleLines = allLines.slice(0, visibleCount);

  if (error) {
    return (
      <div className="card">
        <div className="card-body">
          <div className="error-panel">
            <h3>Error loading log</h3>
            <p>{error}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="card">
      <div className="card-header">
        <h2>Agent Log</h2>
        <select
          className="form-input form-select"
          style={{ width: "auto" }}
          value={order}
          onChange={(e) => setOrder(e.target.value as "recent" | "oldest")}
        >
          <option value="recent">Most recent first</option>
          <option value="oldest">Oldest first</option>
        </select>
      </div>
      <div className="card-body">
        {loading ? (
          <div className="spinner" />
        ) : (
          <>
            <div className="log-viewer" ref={containerRef}>
              {visibleLines.map((line, i) => (
                <div key={i}>{line}</div>
              ))}
            </div>
            {visibleCount < allLines.length && (
              <div style={{ textAlign: "center", marginTop: 12 }}>
                <button className="btn btn-secondary" onClick={loadMore}>
                  Load more ({allLines.length - visibleCount} remaining)
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
