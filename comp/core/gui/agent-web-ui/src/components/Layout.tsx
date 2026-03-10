import { Outlet } from "react-router";
import { Sidebar } from "./Sidebar";
import { useConnectionStatus } from "../hooks/useConnectionStatus";
import { createContext, useContext } from "react";

interface AppContext {
  needsRestart: boolean;
  setNeedsRestart: (v: boolean) => void;
}

const AppCtx = createContext<AppContext>({
  needsRestart: false,
  setNeedsRestart: () => {},
});

export function useAppContext() {
  return useContext(AppCtx);
}

export function Layout() {
  const { connected, version, hostname, needsRestart, setNeedsRestart } =
    useConnectionStatus();

  return (
    <AppCtx.Provider value={{ needsRestart, setNeedsRestart }}>
      <div className="app-layout">
        <Sidebar />

        <header className="topbar">
          <span className="topbar-title">Datadog Agent Manager</span>
          <div className="topbar-info">
            {needsRestart && (
              <span className="topbar-restart-banner">
                Restart Agent to apply changes
              </span>
            )}
            <div className="topbar-meta">
              {version && <span>Version: {version}</span>}
              {hostname && <span>Hostname: {hostname}</span>}
            </div>
            <span
              className={`connection-badge ${connected ? "connected" : "disconnected"}`}
            >
              <span className="connection-dot" />
              {connected ? "Connected" : "Disconnected"}
            </span>
          </div>
        </header>

        <main className="main-content">
          <Outlet />
        </main>
      </div>
    </AppCtx.Provider>
  );
}
