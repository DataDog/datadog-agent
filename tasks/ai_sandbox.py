"""
Provision an agent test environment, run an AI coding agent (claude or codex) on it,
and retrieve a directory from the VM.

This wraps the standalone `cmd/ai-sandbox` binary in the `test/e2e-framework` Go module,
which drives the e2e provisioning framework without `go test` (see PR #51954).
"""

from __future__ import annotations

import os
import tempfile
from pathlib import Path

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.common.color import Color, color_message

E2E_FRAMEWORK_DIR = "test/e2e-framework"
CLI_PACKAGE = "./cmd/ai-sandbox"
CLI_BIN = "bin/ai-sandbox"


def _load_e2e_local_config():
    """Load ~/.test_infra_config.yaml lazily; return the Config or None."""
    try:
        from tasks.e2e_framework import config as e2e_config

        return e2e_config.get_local_config()
    except Exception:
        return None


@task(
    auto_shortflags=False,
    help={
        "prompt": "Prompt to send to the AI agent (mutually exclusive with --prompt-file)",
        "prompt_file": "Path to a local file whose content is used as the prompt",
        "tool": "AI agent to run on the VM: claude or codex (default claude)",
        "model": "Model to use (passed to the tool's --model flag)",
        "effort": "Reasoning effort (codex: model_reasoning_effort; ignored by claude)",
        "tool_args": "Extra arguments appended to the tool invocation",
        "install_cmd": "Override the shell command used to install the tool on the VM",
        "os_descriptor": "OS descriptor flavor:version (e.g. ubuntu:22-04, amazon-linux:2023)",
        "arch": "CPU architecture: x86_64 or arm64",
        "instance_type": "EC2 instance type (empty uses the framework default)",
        "agent_version": "Agent version to install (empty installs the latest)",
        "agent_config": "datadog.yaml content (inline YAML) to apply on the agent",
        "no_fakeintake": "Do not provision a fakeintake",
        "stack_name": "Pulumi stack name to provision",
        "remote_output_dir": "Directory on the VM to run the tool in and retrieve afterwards",
        "local_output_dir": "Local directory to download the remote output directory into",
        "keep": "Keep the stack after the run (skip teardown)",
    },
)
def run(
    ctx,
    prompt="",
    prompt_file="",
    tool="claude",
    model="",
    effort="",
    tool_args="",
    install_cmd="",
    os_descriptor="ubuntu:22-04",
    arch="x86_64",
    instance_type="",
    agent_version="",
    agent_config="",
    no_fakeintake=False,
    stack_name="ai-sandbox",
    remote_output_dir="/tmp/ai-sandbox-output",
    local_output_dir="./ai-sandbox-output",
    keep=False,
):
    """
    Provision an AWS host (agent installed), run claude/codex on it, and retrieve a directory.

    Example:
        dda inv ai-sandbox.run --tool=claude --model=claude-opus-4-8 \\
            --prompt="Summarize the agent status into /tmp/ai-sandbox-output/summary.txt" \\
            --remote-output-dir=/tmp/ai-sandbox-output --local-output-dir=./out
    """
    if tool not in ("claude", "codex"):
        raise Exit(message=f"unknown --tool {tool!r} (expected claude or codex)", code=1)
    if not prompt and not prompt_file:
        raise Exit(message="one of --prompt or --prompt-file is required", code=1)
    if prompt and prompt_file:
        raise Exit(message="--prompt and --prompt-file are mutually exclusive", code=1)

    # Warn when no credential the chosen tool can use is present in the environment.
    cred_vars = ["ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"] if tool == "claude" else ["OPENAI_API_KEY"]
    if not any(os.environ.get(v) for v in cred_vars):
        joined = " or ".join(cred_vars)
        print(color_message(f"WARNING: none of {joined} is set; {tool} will likely fail to authenticate", Color.ORANGE))

    env_vars = {}
    # Export PULUMI_CONFIG_PASSPHRASE from local config when not already set, mirroring
    # new-e2e-tests.run, so developers don't need it in their shell rc.
    if "PULUMI_CONFIG_PASSPHRASE" not in os.environ:
        from tasks.e2e_framework.config import get_pulumi_passphrase

        passphrase = get_pulumi_passphrase(_load_e2e_local_config())
        if passphrase:
            env_vars["PULUMI_CONFIG_PASSPHRASE"] = passphrase

    # Resolve paths to absolute so they are independent of the binary's working directory.
    local_output_dir = str(Path(local_output_dir).expanduser().absolute())

    # Pass the prompt as a file to avoid shell-quoting issues with multi-line prompts.
    tmp_prompt = None
    if prompt:
        fd, tmp_prompt = tempfile.mkstemp(prefix="ai-sandbox-prompt-", suffix=".txt")
        with os.fdopen(fd, "w") as f:
            f.write(prompt)
        prompt_file = tmp_prompt
    prompt_file = str(Path(prompt_file).expanduser().absolute())

    # `go build -o` does not create the output's parent directory, and bin/ is
    # gitignored (absent in a clean checkout), so create it first.
    os.makedirs(os.path.join(E2E_FRAMEWORK_DIR, os.path.dirname(CLI_BIN)), exist_ok=True)

    try:
        with ctx.cd(E2E_FRAMEWORK_DIR):
            ctx.run(f"go build -o {CLI_BIN} {CLI_PACKAGE}")

            args = [
                f"--tool {_q(tool)}",
                f"--prompt-file {_q(prompt_file)}",
                f"--os {_q(os_descriptor)}",
                f"--arch {_q(arch)}",
                f"--stack-name {_q(stack_name)}",
                f"--remote-output-dir {_q(remote_output_dir)}",
                f"--local-output-dir {_q(local_output_dir)}",
            ]
            if model:
                args.append(f"--model {_q(model)}")
            if effort:
                args.append(f"--effort {_q(effort)}")
            if tool_args:
                args.append(f"--tool-args {_q(tool_args)}")
            if install_cmd:
                args.append(f"--install-cmd {_q(install_cmd)}")
            if instance_type:
                args.append(f"--instance-type {_q(instance_type)}")
            if agent_version:
                args.append(f"--agent-version {_q(agent_version)}")
            if agent_config:
                args.append(f"--agent-config {_q(agent_config)}")
            if no_fakeintake:
                args.append("--no-fakeintake")
            if keep:
                args.append("--keep")

            ctx.run(f"./{CLI_BIN} {' '.join(args)}", env=env_vars, pty=True)
    finally:
        if tmp_prompt and os.path.exists(tmp_prompt):
            os.remove(tmp_prompt)

    print(color_message(f"Output retrieved into {local_output_dir}", Color.GREEN))


def _q(value: str) -> str:
    """Single-quote a value for safe use in the shell command line."""
    return "'" + value.replace("'", "'\\''") + "'"
