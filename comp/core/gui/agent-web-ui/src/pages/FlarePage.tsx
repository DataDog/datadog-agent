import { useState } from "react";
import { makeFlare } from "../commands/flare";

export function FlarePage() {
  const [ticket, setTicket] = useState("");
  const [email, setEmail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setResult(null);

    const emailRegex = /\S+@\S+\.\S+/;
    if (!emailRegex.test(email)) {
      setError("Please enter a valid email address.");
      return;
    }

    setSubmitting(true);
    try {
      const data = await makeFlare(email, ticket || "0");
      setResult(data);
      setTicket("");
      setEmail("");
    } catch (err: unknown) {
      setError(typeof err === "string" ? err : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="card">
      <div className="card-header">
        <h2>Send a Flare</h2>
      </div>
      <div className="card-body">
        <div className="flare-form">
          <p style={{ marginBottom: 20, color: "var(--dd-text-secondary)" }}>
            Your logs and configuration files will be collected and sent to
            Datadog Support.
          </p>

          {error && (
            <div className="error-panel" style={{ marginBottom: 16 }}>
              {error}
            </div>
          )}

          {result && (
            <div
              className="feedback feedback-success"
              style={{ marginBottom: 16, display: "block" }}
            >
              {result}
            </div>
          )}

          {!result && (
            <form onSubmit={handleSubmit}>
              <div className="form-group">
                <label className="form-label" htmlFor="ticket">
                  Ticket number (optional)
                </label>
                <input
                  id="ticket"
                  type="number"
                  className="form-input"
                  placeholder="e.g. 123456"
                  value={ticket}
                  onChange={(e) => setTicket(e.target.value)}
                />
              </div>

              <div className="form-group">
                <label className="form-label" htmlFor="email">
                  Email address
                </label>
                <input
                  id="email"
                  type="email"
                  className="form-input"
                  placeholder="you@example.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
              </div>

              <button
                type="submit"
                className="btn btn-primary"
                disabled={submitting}
              >
                {submitting ? "Submitting…" : "Submit Flare"}
              </button>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
