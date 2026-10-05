---
paths:
  - ".bazel*"
  - "bazel/**"
  - "deps/**"
  - "**/*.{bazel,bzl}"
---

# Bazel

Ingest bazel/AGENTS.md prior to any Bazel-related activity:
- reading or editing Bazel files,
- running `bazel` commands,
- inspecting Bazel-managed directories.

## config_setting — design review required

If a solution requires a new `config_setting`, **stop before implementing it**.
Explain the proposed design to the user and wait for explicit approval.
`config_setting` additions affect the build graph globally and require design review before being added to the project.
