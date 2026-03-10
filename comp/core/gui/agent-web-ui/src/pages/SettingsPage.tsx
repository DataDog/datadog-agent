import { useEffect, useState, useRef } from "react";
import { getConfig, setConfig } from "../commands/agent";
import { useAppContext } from "../components/Layout";

export function SettingsPage() {
  const { setNeedsRestart } = useAppContext();
  const [config, setConfigState] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<{
    type: "success" | "error";
    msg: string;
  } | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    getConfig()
      .then(setConfigState)
      .catch((e) => setError(typeof e === "string" ? e : String(e)));
  }, []);

  const handleSave = async () => {
    if (!textareaRef.current) return;
    setSaving(true);
    setFeedback(null);
    try {
      const result = await setConfig(textareaRef.current.value);
      if (result === "Success") {
        setFeedback({ type: "success", msg: "Saved. Restart agent to see changes." });
        setNeedsRestart(true);
      } else {
        setFeedback({ type: "error", msg: result });
      }
    } catch (e: unknown) {
      setFeedback({
        type: "error",
        msg: typeof e === "string" ? e : String(e),
      });
    } finally {
      setSaving(false);
    }
  };

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

  if (config === null) {
    return <div className="spinner" />;
  }

  return (
    <div className="card">
      <div className="card-header">
        <h2>Agent Configuration</h2>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          {feedback && (
            <span
              className={`feedback ${feedback.type === "success" ? "feedback-success" : "feedback-error"}`}
            >
              {feedback.msg}
            </span>
          )}
          <button
            className="btn btn-primary"
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
      <div className="card-body">
        <textarea
          ref={textareaRef}
          className="form-input"
          defaultValue={config}
          spellCheck={false}
        />
      </div>
    </div>
  );
}
