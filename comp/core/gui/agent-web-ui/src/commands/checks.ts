import { invoke } from "@tauri-apps/api/core";

export async function getRunningChecks(): Promise<string> {
  return invoke<string>("checks_running");
}

export async function listChecks(): Promise<string[]> {
  return invoke<string[]>("checks_list_checks");
}

export async function listConfigs(): Promise<string[]> {
  return invoke<string[]>("checks_list_configs");
}

export async function getCheckConfig(fileName: string): Promise<string> {
  return invoke<string>("checks_get_config", { fileName });
}

export async function setCheckConfig(fileName: string, config: string): Promise<string> {
  return invoke<string>("checks_set_config", { fileName, config });
}

export async function disableCheck(fileName: string): Promise<string> {
  return invoke<string>("checks_disable", { fileName });
}
