import { useState } from "react";
import { restart } from "../commands/agent";
import { useAppContext } from "../components/Layout";

export function RestartPage() {
  const { setNeedsRestart } = useAppContext();
  const [restarting, setRestarting] = useState(false);
  const [result, setResult] = useState<{
    type: "success" | "error";
    msg: string;
  } | null>(null);

  const handleRestart = async () => {
    setRestarting(true);
    setResult(null);
    try {
      const data = await restart();
      if (data === "Success") {
        setResult({ type: "success", msg: "Agent restarted successfully." });
        setNeedsRestart(false);
      } else {
        setResult({ type: "error", msg: `Error restarting agent: ${data}` });
      }
    } catch (e: unknown) {
      setResult({
        type: "error",
        msg: typeof e === "string" ? e : String(e),
      });
    } finally {
      setRestarting(false);
    }
  };

  return (
    <div className="card">
      <div className="card-header">
        <h2>Restart Agent</h2>
      </div>
      <div className="card-body" style={{ textAlign: "center", padding: 48 }}>
        {result && (
          <div
            className={`feedback ${result.type === "success" ? "feedback-success" : "feedback-error"}`}
            style={{ marginBottom: 24, display: "inline-flex" }}
          >
            {result.msg}
          </div>
        )}

        <div>
          <p style={{ marginBottom: 20, color: "var(--dd-text-secondary)" }}>
            This will restart the Datadog Agent. The connection will be
            temporarily lost.
          </p>
          <button
            className="btn btn-danger"
            onClick={handleRestart}
            disabled={restarting}
            style={{ fontSize: 15, padding: "10px 24px" }}
          >
            {restarting ? "Restarting…" : "Restart Agent"}
          </button>
        </div>

        {restarting && (
          <div style={{ marginTop: 24 }}>
            <div className="spinner" />
          </div>
        )}
      </div>
    </div>
  );
}
