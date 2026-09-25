# AI agent assets

This `/.agents/` directory stores configuration and other assets that are meant to be shared amongst all agent harnesses like Claude, Codex, Pi, etc.

## Skills

Agent skills are stored in the `skills/` subdirectory. Claude does not [yet](https://github.com/anthropics/claude-code/issues/6235) support this [convention](https://agentskills.io/client-implementation/adding-skills-support#where-to-scan), so we symlink `/.claude/skills/` to that directory.

## MCP config

The MCP servers are declared in `/agents.toml`, and will be moved to this directory when our tooling supports it.

### Synchronizing

Config is written into each tool's own config file by [dotagents](https://github.com/getsentry/dotagents). With [repository tools set up](../docs/public/setup/required.md#prerequisites), run this after changing a declaration:

```
dotagents install
```

Use `dotagents sync` to reconcile or repair offline. For a shell without mise activation, use `mise exec -- dotagents install` or `mise exec -- dotagents sync`.

dotagents writes to every tool listed in the `agents` key of `agents.toml`, repairing the servers it declares while preserving undeclared servers and unrelated content in each file.

### Maintained by hand

- **Zed** (`/.zed/settings.json`) - not yet a dotagents target.
- **`dd-slack`** - Slack has no dynamic client registration, so each client needs its own pre-registered Slack app bound to a fixed redirect URI. We configure this for Claude and Cursor because that's all Slack publishes at the time of writing. Registering our own Slack app with dynamic client registration will likely be required for broader support.
