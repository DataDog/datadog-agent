# AI agent assets

This directory stores shared configuration and skills for coding agents.

## Skills

Repository skills live under `.agents/skills/`.
The existing `.claude/skills/` link makes them available to Claude and Cursor.

## Shared tools

Declare shared plugins and Model Context Protocol (MCP) servers in `/agents.toml`.
The SMP plugins supply performance workflows from the central marketplace.

Use [dotagents](https://github.com/getsentry/dotagents) through [Mise](https://mise.jdx.dev) after a fresh checkout or a declaration change:

```sh
mise run dotagents install
```

This command installs or refreshes shared plugins and writes the declared MCP configuration for each target tool.
It uses project scope and preserves unrelated configuration.
The source needs Git and SSH access to `ddoghq/claude-marketplace`.
If Mise reports an untrusted configuration, inspect `mise.toml`, then run `mise trust` for this checkout.

Installed SMP skills appear under `.agents/skills/`. OpenCode also receives links under `.opencode/skills/`.
The generated dependencies and catalogs stay outside version control.
The [SMP entry skill](skills/smp/SKILL.md) tells agents to refresh before the first SMP workflow in each session.
The SMP CLI installs only when a workflow needs it.

## Maintained by hand

- **Zed** (`/.zed/settings.json`): dotagents does not support this target yet.
- **Slack**: Claude and Cursor use separate, registered Slack apps with fixed redirect URIs.
  Other clients need a registered Slack app or support for dynamic client registration.
