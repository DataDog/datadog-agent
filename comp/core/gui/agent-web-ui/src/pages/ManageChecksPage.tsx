import { useEffect, useState, useCallback, useRef } from "react";
import {
  listConfigs,
  listChecks,
  getCheckConfig,
  setCheckConfig,
  disableCheck,
} from "../commands/checks";
import { useAppContext } from "../components/Layout";

type Mode = "enabled" | "add";

function isFilteredConfig(name: string): boolean {
  return (
    name.endsWith(".example") ||
    name.endsWith(".disabled") ||
    name.endsWith("metrics.yaml") ||
    name.endsWith("auto_conf.yaml")
  );
}

export function ManageChecksPage() {
  const { setNeedsRestart } = useAppContext();
  const [mode, setMode] = useState<Mode>("enabled");
  const [configs, setConfigs] = useState<string[]>([]);
  const [checks, setChecks] = useState<string[]>([]);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [editorContent, setEditorContent] = useState<string | null>(null);
  const [description, setDescription] = useState("Select a check to configure.");
  const [feedback, setFeedback] = useState<{
    type: "success" | "error";
    msg: string;
  } | null>(null);
  const [saving, setSaving] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const loadConfigs = useCallback(async () => {
    try {
      const data = await listConfigs();
      const filtered = data
        .filter((f) => !isFilteredConfig(f))
        .sort();
      setConfigs(filtered);
      setDescription("Select a check to configure.");
    } catch (e) {
      setConfigs([]);
      setDescription(typeof e === "string" ? e : "Unable to load configurations.");
    }
  }, []);

  const loadAddableChecks = useCallback(async () => {
    try {
      const configData = await listConfigs().catch(() => [] as string[]);
      const enabledNames = new Set<string>();
      configData
        .filter((f) => !isFilteredConfig(f))
        .forEach((f) => {
          enabledNames.add(f.substring(0, f.indexOf(".")));
        });

      const checkData = await listChecks();
      const available = checkData
        .map((c) => (c.endsWith(".py") ? c.slice(0, -3) : c))
        .filter((c) => !enabledNames.has(c))
        .sort();

      setChecks(available);
      setDescription("Select a check to add.");
    } catch (e) {
      setChecks([]);
      setDescription(typeof e === "string" ? e : "Unable to load checks.");
    }
  }, []);

  useEffect(() => {
    setSelectedFile(null);
    setEditorContent(null);
    setFeedback(null);
    if (mode === "enabled") {
      loadConfigs();
    } else {
      loadAddableChecks();
    }
  }, [mode, loadConfigs, loadAddableChecks]);

  const openConfig = async (fileName: string) => {
    setSelectedFile(fileName);
    setFeedback(null);
    try {
      const data = await getCheckConfig(fileName);
      setEditorContent(data);
      if (fileName.includes(".default")) {
        setDescription(
          "Changing a default configuration file creates a new, non-default configuration file.",
        );
      } else {
        setDescription("Edit the configuration file, then save and reload.");
      }
    } catch {
      setEditorContent(null);
      setDescription("Error loading configuration file.");
    }
  };

  const handleSave = async () => {
    if (!textareaRef.current || !selectedFile) return;
    setSaving(true);
    setFeedback(null);
    try {
      let fileName = selectedFile;
      if (fileName.endsWith(".default")) {
        fileName = fileName.slice(0, -8);
      }
      const result = await setCheckConfig(fileName, textareaRef.current.value);
      if (result === "Success") {
        setFeedback({ type: "success", msg: "Saved" });
        setNeedsRestart(true);
        setSelectedFile(fileName);
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

  const handleDisable = async () => {
    if (!selectedFile) return;
    setSaving(true);
    setFeedback(null);
    try {
      const result = await disableCheck(selectedFile);
      if (result === "Success") {
        setFeedback({ type: "success", msg: "Disabled" });
        setNeedsRestart(true);
        loadConfigs();
        setEditorContent(null);
        setSelectedFile(null);
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

  const handleAddCheck = async (checkName: string) => {
    setSelectedFile(checkName);
    setFeedback(null);

    let initialContent = "# Add your configuration here";
    try {
      const allConfigs = await listConfigs();
      const disabled = allConfigs.find(
        (f) =>
          f.substring(0, f.indexOf(".")) === checkName &&
          f.endsWith(".disabled"),
      );
      const example = allConfigs.find(
        (f) =>
          f.substring(0, f.indexOf(".")) === checkName &&
          f.endsWith(".example"),
      );
      const templateFile = disabled || example;
      if (templateFile) {
        initialContent = await getCheckConfig(templateFile);
      }
    } catch {
      // use default content
    }

    setEditorContent(initialContent);
    setDescription("Create a new configuration file for this check.");
  };

  const handleSaveNewCheck = async () => {
    if (!textareaRef.current || !selectedFile) return;
    setSaving(true);
    setFeedback(null);
    try {
      const fileName = `${selectedFile}.d/conf.yaml`;
      const result = await setCheckConfig(fileName, textareaRef.current.value);
      if (result === "Success") {
        setFeedback({ type: "success", msg: "Check added" });
        setNeedsRestart(true);
        setMode("enabled");
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

  const listItems = mode === "enabled" ? configs : checks;

  return (
    <div className="card" style={{ display: "flex", flexDirection: "column", height: "calc(100vh - var(--topbar-height) - 48px)" }}>
      <div className="checks-description">{description}</div>
      <div className="checks-layout" style={{ flex: 1, minHeight: 0 }}>
        <div className="checks-sidebar">
          <div className="checks-sidebar-header">
            <select
              className="form-input form-select"
              value={mode}
              onChange={(e) => setMode(e.target.value as Mode)}
            >
              <option value="enabled">Edit Enabled Checks</option>
              <option value="add">Add a Check</option>
            </select>
          </div>
          <div className="checks-list">
            {listItems.map((item) => (
              <button
                key={item}
                className={`check-item ${selectedFile === item ? "active" : ""}`}
                onClick={() =>
                  mode === "enabled" ? openConfig(item) : handleAddCheck(item)
                }
              >
                {item}
              </button>
            ))}
            {listItems.length === 0 && (
              <div
                style={{
                  padding: 16,
                  textAlign: "center",
                  color: "var(--dd-text-secondary)",
                  fontSize: 13,
                }}
              >
                No items found
              </div>
            )}
          </div>
        </div>

        <div className="checks-content">
          {editorContent !== null ? (
            <>
              <div
                style={{
                  display: "flex",
                  gap: 8,
                  marginBottom: 12,
                  alignItems: "center",
                }}
              >
                {mode === "enabled" ? (
                  <>
                    <button
                      className="btn btn-primary"
                      onClick={handleSave}
                      disabled={saving}
                    >
                      Save
                    </button>
                    <button
                      className="btn btn-danger"
                      onClick={handleDisable}
                      disabled={saving}
                    >
                      Disable
                    </button>
                  </>
                ) : (
                  <button
                    className="btn btn-primary"
                    onClick={handleSaveNewCheck}
                    disabled={saving}
                  >
                    Add Check
                  </button>
                )}
                {feedback && (
                  <span
                    className={`feedback ${feedback.type === "success" ? "feedback-success" : "feedback-error"}`}
                  >
                    {feedback.msg}
                  </span>
                )}
              </div>
              <textarea
                ref={textareaRef}
                className="form-input"
                defaultValue={editorContent}
                key={`${selectedFile}-${mode}`}
                spellCheck={false}
              />
            </>
          ) : (
            <div
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                height: "100%",
                color: "var(--dd-text-secondary)",
              }}
            >
              {mode === "enabled"
                ? "Select a check from the list to edit its configuration"
                : "Select a check from the list to add it"}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
