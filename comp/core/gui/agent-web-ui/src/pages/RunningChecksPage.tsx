import { useEffect, useState } from "react";
import { getRunningChecks } from "../commands/checks";

export function RunningChecksPage() {
  const [output, setOutput] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getRunningChecks()
      .then(setOutput)
      .catch((e) => setError(typeof e === "string" ? e : String(e)));
  }, []);

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
        <h2>Running Checks</h2>
      </div>
      <div className="card-body">
        <pre className="log-viewer">{output}</pre>
      </div>
    </div>
  );
}
