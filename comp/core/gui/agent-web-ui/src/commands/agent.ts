import { invoke } from "@tauri-apps/api/core";

export async function ping(): Promise<boolean> {
  return invoke<boolean>("agent_ping");
}

export async function getStatus(statusType: "general" | "collector"): Promise<string> {
  return invoke<string>("agent_status", { statusType });
}

export async function getVersion(): Promise<string> {
  return invoke<string>("agent_version");
}

export async function getHostname(): Promise<string> {
  return invoke<string>("agent_hostname");
}

export async function getLog(flip: boolean): Promise<string[]> {
  return invoke<string[]>("agent_log", { flip });
}

export async function restart(): Promise<string> {
  return invoke<string>("agent_restart");
}

export async function getConfig(): Promise<string> {
  return invoke<string>("agent_get_config");
}

export async function setConfig(config: string): Promise<string> {
  return invoke<string>("agent_set_config", { config });
}
