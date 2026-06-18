use std::collections::{HashMap, HashSet};

use crate::datadog::{AiProcessConfig, AiProcessMatchScope};

pub(crate) fn builtin_ai_process_names() -> Vec<AiProcessConfig> {
    vec![
        ai_process(
            &["Cursor.exe", "Cursor"],
            "Cursor",
            "Anysphere",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["Claude.exe", "Claude"],
            "Claude",
            "Anthropic",
            AiProcessMatchScope::Direct,
        ),
        ai_process(
            &["claude.exe", "claude"],
            "Claude Code",
            "Anthropic",
            AiProcessMatchScope::HostedChild,
        ),
        secondary_ai_process(
            &["cowork-svc.exe", "cowork-svc"],
            "Claude Cowork",
            "Anthropic",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["Codex.exe", "codex"],
            "Codex",
            "OpenAI",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &[
                "OpenClaw.exe",
                "openclaw-desktop.exe",
                "OpenClaw.Tray.WinUI.exe",
                "OpenClaw",
                "openclaw-desktop",
            ],
            "OpenClaw",
            "OpenClaw",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["Junie.exe"],
            "Junie",
            "JetBrains",
            AiProcessMatchScope::Direct,
        ),
        ai_process(
            &["gemini.exe", "gemini-cli.exe"],
            "Gemini CLI",
            "Google",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["hermes.exe", "hermes", "hermes-agent.exe", "hermes-agent"],
            "Hermes Agent",
            "Nous Research",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["autohand.exe", "autohand-code.exe"],
            "Autohand Code CLI",
            "Autohand",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["opencode.exe"],
            "OpenCode",
            "OpenCode",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["openhands.exe"],
            "OpenHands",
            "OpenHands",
            AiProcessMatchScope::Both,
        ),
        ai_process(&["mux.exe"], "Mux", "Coder", AiProcessMatchScope::Both),
        ai_process(
            &["amp.exe"],
            "Amp",
            "Sourcegraph",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["letta.exe", "letta-code.exe"],
            "Letta",
            "Letta",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["firebender.exe"],
            "Firebender",
            "Firebender",
            AiProcessMatchScope::Direct,
        ),
        ai_process(&["goose.exe"], "Goose", "Block", AiProcessMatchScope::Both),
        ai_process(
            &["Piebald.exe", "piebald.exe"],
            "Piebald",
            "Piebald",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["factory.exe"],
            "Factory",
            "Factory",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["trae.exe", "Trae.exe"],
            "TRAE",
            "ByteDance",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["roo-code.exe", "roocode.exe"],
            "Roo Code",
            "Roo Code",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["mistral-vibe.exe", "vibe.exe"],
            "Mistral AI Vibe",
            "Mistral AI",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["command-code.exe", "commandcode.exe"],
            "Command Code",
            "Command Code",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["vtcode.exe", "vt-code.exe"],
            "VT Code",
            "VT Code",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["qodo.exe", "qodo-cli.exe"],
            "Qodo",
            "Qodo",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["kiro.exe", "Kiro.exe"],
            "Kiro",
            "Kiro",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["workshop.exe", "Workshop.exe"],
            "Workshop",
            "Workshop",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["nanobot.exe"],
            "nanobot",
            "nanobot",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["fast-agent.exe", "fastagent.exe"],
            "fast-agent",
            "fast-agent",
            AiProcessMatchScope::HostedChild,
        ),
        ai_process(
            &["tabnine.exe", "tabnine-cli.exe"],
            "Tabnine",
            "Tabnine",
            AiProcessMatchScope::Both,
        ),
        ai_process(
            &["emdash.exe"],
            "Emdash",
            "Emdash",
            AiProcessMatchScope::Both,
        ),
    ]
}

pub(crate) fn builtin_host_process_names() -> Vec<String> {
    strings(&[
        "cmd.exe",
        "powershell.exe",
        "pwsh.exe",
        "WindowsTerminal.exe",
        "wt.exe",
        "conhost.exe",
        "Code.exe",
        "Cursor.exe",
        "devenv.exe",
        "idea64.exe",
        "pycharm64.exe",
        "webstorm64.exe",
        "rider64.exe",
        "wezterm-gui.exe",
        "Terminal",
        "iTerm2",
        "Ghostty",
        "WezTerm",
        "wezterm-gui",
        "Alacritty",
        "alacritty",
        "kitty",
        "Code",
        "Cursor",
        "IntelliJ IDEA",
        "PyCharm",
        "WebStorm",
        "Rider",
        "alacritty.exe",
        "mintty.exe",
        "Tabby.exe",
        "Hyper.exe",
    ])
}

pub(crate) fn merge_ai_process_names(
    mut builtins: Vec<AiProcessConfig>,
    overrides: Vec<AiProcessConfig>,
) -> Vec<AiProcessConfig> {
    let mut index_by_tool: HashMap<String, usize> = builtins
        .iter()
        .enumerate()
        .map(|(index, process)| (process.tool.clone(), index))
        .collect();

    for process in overrides {
        if let Some(index) = index_by_tool.get(&process.tool).copied() {
            builtins[index] = process;
        } else {
            index_by_tool.insert(process.tool.clone(), builtins.len());
            builtins.push(process);
        }
    }

    builtins
}

pub(crate) fn merge_host_process_names(
    builtins: Vec<String>,
    overrides: Vec<String>,
) -> Vec<String> {
    let mut seen: HashSet<String> = HashSet::new();
    let mut merged = Vec::new();
    for name in builtins.into_iter().chain(overrides) {
        if seen.insert(normalize_process_name(&name)) {
            merged.push(name);
        }
    }
    merged
}

fn ai_process(
    process_names: &[&str],
    tool: &str,
    provider: &str,
    match_scope: AiProcessMatchScope,
) -> AiProcessConfig {
    AiProcessConfig {
        process_names: strings(process_names),
        tool: tool.to_string(),
        provider: provider.to_string(),
        match_scope,
        approved: false,
        secondary: false,
    }
}

fn secondary_ai_process(
    process_names: &[&str],
    tool: &str,
    provider: &str,
    match_scope: AiProcessMatchScope,
) -> AiProcessConfig {
    AiProcessConfig {
        secondary: true,
        ..ai_process(process_names, tool, provider, match_scope)
    }
}

fn strings(values: &[&str]) -> Vec<String> {
    values.iter().map(|value| value.to_string()).collect()
}

fn normalize_process_name(name: &str) -> String {
    let lower = name.to_ascii_lowercase();
    lower.strip_suffix(".exe").unwrap_or(&lower).to_string()
}
