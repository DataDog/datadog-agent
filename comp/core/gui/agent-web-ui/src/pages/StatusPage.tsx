import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { getStatus } from "../commands/agent";

export function StatusPage() {
  const { type } = useParams<{ type: string }>();
  const statusType = type === "collector" ? "collector" : "general";

  const [output, setOutput] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setOutput(null);
    setError(null);
    getStatus(statusType as "general" | "collector")
      .then(setOutput)
      .catch((e) => setError(typeof e === "string" ? e : String(e)));
  }, [statusType]);

  if (error) {
    return (
      <div className="card">
        <div className="card-body">
          <div className="error-panel">
            <h3>Error</h3>
            <p>{error}</p>
          </div>
        </div>
      </div>
    );
  }

  if (output === null) {
    return <div className="spinner" />;
  }

  return (
    <div className="card">
      <div className="card-header">
        <h2>{statusType === "general" ? "General Status" : "Collector Status"}</h2>
      </div>
      <div className="card-body">
        <pre className="log-viewer">{output}</pre>
      </div>
    </div>
  );
}
