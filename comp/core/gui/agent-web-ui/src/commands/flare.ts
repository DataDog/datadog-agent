import { invoke } from "@tauri-apps/api/core";

export async function makeFlare(email: string, caseId: string): Promise<string> {
  return invoke<string>("agent_flare", { email, caseId });
}
