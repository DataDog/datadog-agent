# MCP Evaluation Framework

Automated evaluation framework for testing three MCP modes (bash, self-shell, tools) across 15 SRE scenarios using Claude as the agent.

## Requirements

- **Python 3.12+**
- **uv** - Python package manager
- **Lima** - Lightweight VM manager
- **Anthropic API key** - For Claude Code agent

## Setup

Install Python dependencies:
```bash
uv sync
```

## Scripts

### VM Management
- **start-vm.sh** - Start a Lima VM with MCP server
  ```bash
  ./scripts/start-vm.sh mcp-eval-tools lima-tools.yaml
  ```
- **teardown-vm.sh** - Stop and delete a Lima VM
  ```bash
  ./scripts/teardown-vm.sh mcp-eval-tools
  ```

### Evaluation
- **evaluate.py** - Run evaluations across scenarios and modes
  ```bash
  export ANTHROPIC_API_KEY=your-key

  # Run all scenarios for all modes (default)
  uv run python scripts/evaluate.py

  # Run specific scenarios for specific modes
  uv run python scripts/evaluate.py --modes bash tools --scenarios high-cpu-usage port-conflict

  # Run all scenarios for one mode
  uv run python scripts/evaluate.py --modes bash

  # Grade existing transcripts without re-running investigations
  uv run python scripts/evaluate.py --grade-only --run-dir results/run-20260120_120000
  ```
  Creates timestamped run directory: `results/run-YYYYMMDD_HHMMSS/`

- **consolidate_results.py** - Generate Excel report from evaluation results
  ```bash
  uv run python scripts/consolidate_results.py [run-directory]
  # If no directory specified, uses latest run
  ```
  Outputs: `results/run-*/results.xlsx` (importable to Google Sheets)

## Directory Structure

```
scenarios/          # 15 SRE scenarios (setup.sh, teardown.sh, workload.py, PROMPT.md)
results/            # Timestamped evaluation runs
  run-*/
    evaluation-*.jsonl
    results.xlsx
    transcripts/
scripts/            # Evaluation and VM management scripts
lima-*.yaml         # Lima VM configurations (bash, safe-shell, tools modes)
```

## Development

Edit scenarios in `scenarios/`, adjust VM configs in `lima-*.yaml`, or modify evaluation logic in `scripts/evaluate.py`.
