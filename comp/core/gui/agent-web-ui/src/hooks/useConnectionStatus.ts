import { useState, useEffect, useCallback } from "react";
import { ping, getVersion, getHostname } from "../commands/agent";

interface ConnectionState {
  connected: boolean;
  version: string | null;
  hostname: string | null;
  needsRestart: boolean;
  setNeedsRestart: (v: boolean) => void;
}

export function useConnectionStatus(): ConnectionState {
  const [connected, setConnected] = useState(false);
  const [version, setVersion] = useState<string | null>(null);
  const [hostname, setHostname] = useState<string | null>(null);
  const [needsRestart, setNeedsRestart] = useState(false);

  const checkStatus = useCallback(async () => {
    try {
      const running = await ping();
      setConnected(running);
    } catch {
      setConnected(false);
    }
  }, []);

  useEffect(() => {
    checkStatus();
    const id = setInterval(checkStatus, 2000);
    return () => clearInterval(id);
  }, [checkStatus]);

  useEffect(() => {
    getVersion()
      .then((v) => setVersion(v.trim()))
      .catch(() => {});

    getHostname()
      .then((h) => setHostname(h.trim()))
      .catch(() => {});
  }, []);

  return { connected, version, hostname, needsRestart, setNeedsRestart };
}
