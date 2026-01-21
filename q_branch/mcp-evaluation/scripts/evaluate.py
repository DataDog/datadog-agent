#!/usr/bin/env python3
"""
MCP Evaluation Script
Runs Claude Code investigations across 15 scenarios in 3 modes (bash, safe-shell, tools)
in parallel, and grades the results against SCENARIO.md rubrics.
"""

import asyncio
import json
import subprocess
import os
import argparse
from datetime import datetime
from pathlib import Path
from typing import Dict, List

import httpx
from claude_agent_sdk import query, ClaudeAgentOptions, ResultMessage
from anthropic import (
    Anthropic,
    RateLimitError,
    APIError,
    APIStatusError,
    APIConnectionError,
    APITimeoutError,
)
from tenacity import retry, stop_after_attempt, wait_exponential, retry_if_exception_type, retry_if_exception

# Configuration
# Script is at: q_branch/mcp-evaluation/scripts/evaluate.py
# So parent.parent gives us: q_branch/mcp-evaluation/
MCP_EVAL_DIR = Path(__file__).parent.parent
SCENARIOS_DIR = MCP_EVAL_DIR / "scenarios"
RESULTS_DIR = MCP_EVAL_DIR / "results"
SCRIPTS_DIR = MCP_EVAL_DIR / "scripts"
MODES = ["bash", "safe-shell", "tools"]
VM_PORTS = {"bash": 8081, "safe-shell": 8082, "tools": 8083}

# Scenarios list
SCENARIOS = [
    # Easy
    "high-cpu-usage", "disk-space-full", "port-conflict",
    "zombie-processes", "dns-resolution-failure",
    # Medium
    "memory-leak", "connection-exhaustion", "log-rotation-failure",
    "swap-thrashing", "file-descriptor-leak",
    # Hard
    "tcp-close-wait", "io-wait", "context-switching-storm",
    "inode-exhaustion", "tcp-syn-flood"
]


def is_retryable_api_error(exception):
    """Check if an API exception should be retried (transient errors only)"""
    # Always retry rate limit errors
    if isinstance(exception, RateLimitError):
        return True

    # Retry connection and timeout errors (transient network issues)
    if isinstance(exception, (APIConnectionError, APITimeoutError)):
        return True

    # For APIStatusError (HTTP status code errors)
    if isinstance(exception, APIStatusError):
        status_code = exception.status_code
        # 429 Too Many Requests
        if status_code == 429:
            return True
        # Retryable 5xx server errors
        if status_code in (500, 502, 503, 504, 529) or (500 <= status_code < 600):
            return True

    # For generic APIError, check if it has status code or error message
    if isinstance(exception, APIError):
        # Check for specific retryable status codes
        if hasattr(exception, 'status_code'):
            status_code = exception.status_code
            if status_code == 429 or status_code in (500, 502, 503, 504, 529) or (500 <= status_code < 600):
                return True

        # Also check error message for overloaded/rate limit keywords
        error_message = str(exception).lower()
        if any(keyword in error_message for keyword in ['overloaded', 'rate limit', 'too many requests']):
            return True

    return False


class EvaluationRunner:
    # Class-level locks to serialize VM operations across all runners
    # This prevents Lima race conditions on shared directories (~/.lima/_networks/, _config/)
    _restart_lock = asyncio.Lock()
    _teardown_lock = asyncio.Lock()

    def __init__(self, mode: str, run_dir: Path):
        self.mode = mode
        self.vm_name = f"mcp-eval-{mode}"
        self.port = VM_PORTS[mode]
        self.run_dir = run_dir
        self.results_file = run_dir / f"evaluation-{mode}.jsonl"
        self.transcripts_dir = run_dir / "transcripts" / mode
        self.scenarios_to_run = SCENARIOS  # Can be overridden

    async def get_vm_status(self):
        """Get the status of the VM, returns None if VM doesn't exist"""
        try:
            process = await asyncio.create_subprocess_exec(
                "limactl", "list", "--format", "{{.Name}} {{.Status}}",
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            stdout, _ = await process.communicate()
            result = stdout.decode()

            for line in result.splitlines():
                split = line.split(" ")
                if len(split) < 2:
                    continue
                name, status = split[0], split[1]
                if name == self.vm_name:
                    return status
            return None
        except Exception as e:
            print(f"[{self.mode}] Error checking VM status: {e}")
            return None

    async def run_scenarios(self):
        """Run all scenarios for this mode (VM must already be set up)"""
        print(f"\n{'='*60}")
        print(f"Starting evaluation for mode: {self.mode}")
        print(f"{'='*60}\n")

        for i, scenario in enumerate(self.scenarios_to_run):
            try:
                print(f"\n[{self.mode}] ━━━ Evaluating {scenario} ({i+1}/{len(self.scenarios_to_run)}) ━━━")
                result = await self.evaluate_scenario(scenario)
                self.write_result(result)
                print(f"[{self.mode}] ✓ {scenario} completed")

                # Restart VM between scenarios (except after the last one)
                if i < len(self.scenarios_to_run) - 1:
                    print(f"[{self.mode}] Restarting VM for clean state...")
                    await self.restart_vm()

            except Exception as e:
                print(f"[{self.mode}] ✗ Error in {scenario}: {e}")
                import traceback
                traceback.print_exc()
                self.write_result({
                    "mode": self.mode,
                    "scenario": scenario,
                    "status": "error",
                    "error": str(e),
                    "timestamp": datetime.now().isoformat()
                })

                # Try to restart VM even on error
                if i < len(self.scenarios_to_run) - 1:
                    try:
                        print(f"[{self.mode}] Attempting VM restart after error...")
                        await self.restart_vm()
                    except Exception as restart_error:
                        print(f"[{self.mode}] VM restart failed: {restart_error}")

        # Teardown at the end
        await self.teardown_vm()

        print(f"\n{'='*60}")
        print(f"Evaluation complete for mode: {self.mode}")
        print(f"Results written to: {self.results_file}")
        print(f"{'='*60}\n")

    async def setup_vm(self):
        """Start VM for this mode"""
        print(f"[{self.mode}] Starting VM {self.vm_name}")
        lima_config = MCP_EVAL_DIR / f"lima-{self.mode}.yaml"

        cmd = [str(SCRIPTS_DIR / "start-vm.sh"), self.vm_name, str(lima_config)]
        print(f"[{self.mode}] Running command: {' '.join(cmd)}")

        try:
            process = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            stdout, stderr = await process.communicate()
            if process.returncode is None or process.returncode != 0:
                print(f"[{self.mode}] start-vm.sh failed with code {process.returncode}")
                if stdout:
                    print(f"[{self.mode}] stdout: {stdout.decode()}")
                if stderr:
                    print(f"[{self.mode}] stderr: {stderr.decode()}")
                raise subprocess.CalledProcessError(process.returncode or -1, "start-vm.sh")
        except subprocess.CalledProcessError:
            # VM creation failed, possibly due to corrupted state. Clean up and retry.
            print(f"[{self.mode}] VM creation failed, cleaning up with teardown_vm()...")
            await self.teardown_vm()

            print(f"[{self.mode}] Retry - Running command: {' '.join(cmd)}")
            process = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            stdout, stderr = await process.communicate()
            if process.returncode is None or process.returncode != 0:
                print(f"[{self.mode}] start-vm.sh retry failed with code {process.returncode}")
                if stdout:
                    print(f"[{self.mode}] stdout: {stdout.decode()}")
                if stderr:
                    print(f"[{self.mode}] stderr: {stderr.decode()}")
                raise subprocess.CalledProcessError(process.returncode or -1, "start-vm.sh")

        print(f"[{self.mode}] VM {self.vm_name} is ready")
        await asyncio.sleep(5)  # Wait for VM to be fully ready

    async def restart_vm(self):
        """Restart VM to ensure clean state (serialized across all runners)"""
        async with EvaluationRunner._restart_lock:
            print(f"[{self.mode}] Restarting VM {self.vm_name}...")
            cmd = ["limactl", "restart", self.vm_name]
            print(f"[{self.mode}] Running command: {' '.join(cmd)}")
            process = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            stdout, stderr = await process.communicate()
            if process.returncode is None or process.returncode != 0:
                print(f"[{self.mode}] limactl restart failed with code {process.returncode}")
                if stdout:
                    print(f"[{self.mode}] stdout: {stdout.decode()}")
                if stderr:
                    print(f"[{self.mode}] stderr: {stderr.decode()}")
                raise subprocess.CalledProcessError(process.returncode or -1, "limactl restart")
            await asyncio.sleep(5)  # Wait for VM to be fully ready
            print(f"[{self.mode}] VM restarted successfully")

    async def teardown_vm(self):
        """Stop and delete VM (serialized across all runners)"""
        async with EvaluationRunner._teardown_lock:
            print(f"[{self.mode}] Stopping and deleting VM {self.vm_name}")
            cmd = [str(SCRIPTS_DIR / "teardown-vm.sh"), self.vm_name]
            print(f"[{self.mode}] Running command: {' '.join(cmd)}")
            process = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            stdout, stderr = await process.communicate()
            if process.returncode is None or process.returncode != 0:
                print(f"[{self.mode}] teardown-vm.sh failed with code {process.returncode}")
                if stdout:
                    print(f"[{self.mode}] stdout: {stdout.decode()}")
                if stderr:
                    print(f"[{self.mode}] stderr: {stderr.decode()}")
                raise subprocess.CalledProcessError(process.returncode or -1, "teardown-vm.sh")
        print(f"[{self.mode}] VM {self.vm_name} removed")

    async def evaluate_scenario(self, scenario: str) -> Dict:
        """Evaluate a single scenario"""
        # 1. Verify MCP server is healthy
        print(f"[{self.mode}/{scenario}] Verifying MCP server is healthy...")
        if not await self.check_mcp_server_health():
            raise Exception("MCP server is not responding")

        # 2. Deploy scenario
        print(f"[{self.mode}/{scenario}] Deploying scenario...")
        await self.deploy_scenario(scenario)

        # 3. Run Claude Code investigation
        print(f"[{self.mode}/{scenario}] Running investigation...")
        findings, duration, turns, cost = await self.run_investigation(scenario)
        findings = findings or "no findings"
        print(f"[{self.mode}/{scenario}] Investigation complete:")
        print(f"[{self.mode}/{scenario}]   Duration: {duration}ms | Turns: {turns} | Cost: ${cost}")
        print(f"[{self.mode}/{scenario}]   Findings: {findings[:100]}...")

        # 4. Grade findings
        print(f"[{self.mode}/{scenario}] Grading findings...")
        score = await self.grade_findings(scenario, findings)
        print(f"[{self.mode}/{scenario}] Overall Score: {score.get('overall_score', 'N/A')}/100")
        if 'category_scores' in score:
            print(f"[{self.mode}/{scenario}] Category Scores: {score['category_scores']}")

        # 5. Teardown scenario
        print(f"[{self.mode}/{scenario}] Cleaning up...")
        await self.teardown_scenario(scenario)

        return {
            "mode": self.mode,
            "scenario": scenario,
            "findings": findings,
            "score": score,
            "status": "completed",
            "timestamp": datetime.now().isoformat(),
            "duration_ms": duration,
            "turns": turns,
            "cost": cost
        }

    async def check_mcp_server_health(self):
        """Check if MCP server is healthy via /health endpoint"""
        try:
            async with httpx.AsyncClient() as client:
                response = await client.get(
                    f"http://127.0.0.1:{self.port}/health",
                    timeout=10.0
                )
                if response.status_code == 200:
                    print(f"[{self.mode}] MCP server is healthy")
                    return True
                else:
                    print(f"[{self.mode}] MCP server health check failed: {response.status_code}")
                    return False
        except Exception as e:
            print(f"[{self.mode}] MCP server health check error: {e}")
            return False

    async def deploy_scenario(self, scenario: str):
        """Deploy scenario to VM"""
        setup_script = SCENARIOS_DIR / scenario / "setup.sh"
        cmd = [str(setup_script), self.vm_name]
        print(f"[{self.mode}/{scenario}] Running command: {' '.join(cmd)}")
        process = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE
        )
        stdout, stderr = await process.communicate()
        if process.returncode is None or process.returncode != 0:
            print(f"[{self.mode}/{scenario}] setup.sh failed with code {process.returncode}")
            if stdout:
                print(f"[{self.mode}/{scenario}] stdout: {stdout.decode()}")
            if stderr:
                print(f"[{self.mode}/{scenario}] stderr: {stderr.decode()}")
            raise subprocess.CalledProcessError(process.returncode or -1, str(setup_script))
        await asyncio.sleep(15)  # Wait for issue to manifest

    @retry(
        stop=stop_after_attempt(6),
        wait=wait_exponential(multiplier=1, min=2, max=60),
        retry=retry_if_exception_type((RateLimitError, APIError)) & retry_if_exception(is_retryable_api_error),
        before_sleep=lambda retry_state: print(
            f"[{retry_state.kwargs.get('self').mode}] API error during investigation on attempt {retry_state.attempt_number}: "
            f"{retry_state.outcome.exception() if retry_state.outcome else 'Unknown error'}. "
            f"Retrying in {retry_state.next_action.sleep:.1f if retry_state.next_action and retry_state.next_action.sleep else 0}s..."
        ) if retry_state.kwargs.get('self') else None,
        reraise=True
    )
    async def run_investigation(self, scenario: str) -> tuple[str | None, int, int, float | None]:
        """Run Claude Code investigation via SDK (with retry on API errors)"""
        # Read prompt
        prompt_file = SCENARIOS_DIR / scenario / "PROMPT.md"
        base_prompt = prompt_file.read_text()

        # Prepend VM name to the prompt to specify which system to investigate
        prompt = f"""**TARGET SYSTEM: {self.vm_name}**

You are investigating the remote server named **{self.vm_name}**. All diagnostic tools connect to this specific server.

IMPORTANT: Only investigate {self.vm_name}. Do not investigate your local machine or any other systems.

---

{base_prompt}"""

        # System prompt for consistent SRE investigation approach
        system_prompt = """You are an experienced on-call Site Reliability Engineer (SRE) investigating a production system issue.

Your investigation should be:
1. **Systematic** - Use diagnostic tools methodically to gather relevant data
2. **Thorough** - Check multiple system resources and correlate findings
3. **Analytical** - Identify patterns, anomalies, and root causes
4. **Actionable** - Provide clear findings and concrete mitigation steps

Investigation framework:
- Start with broad system health checks (CPU, memory, disk, network, processes)
- Narrow down to specific issues based on initial findings
- Identify the root cause, not just symptoms
- Document your reasoning and evidence
- Propose specific remediation steps

Provide your final analysis in a clear, structured format covering:
- **Problem Summary**: What is happening?
- **Root Cause**: Why is it happening?
- **Evidence**: What data supports this conclusion?
- **Impact**: What resources/services are affected?
- **Mitigation**: What steps should be taken to resolve this?

Use the available diagnostic tools effectively. Be thorough but efficient."""

        # Configure MCP connection
        mcp_url = f"http://127.0.0.1:{self.port}/mcp"

        # Determine allowed tools based on mode
        if self.mode == "bash":
            allowed_tools = ["mcp__mcp-eval__bash_execute"]
        elif self.mode == "safe-shell":
            allowed_tools = ["mcp__mcp-eval__safe_shell_execute"]
        else:  # tools
            allowed_tools = [
                f"mcp__mcp-eval__{tool}" for tool in [
                    "get_cpu_info", "get_memory_info", "get_disk_usage",
                    "get_io_stats", "list_processes", "get_process_info",
                    "find_process", "get_network_interfaces", "get_listening_ports",
                    "get_network_connections", "check_connectivity",
                    "read_file", "tail_file", "search_file",
                    "get_system_info", "get_environment"
                ]
            ]

        # Run investigation
        conversation = []
        try:
            async for message in query(
                prompt=prompt,
                options=ClaudeAgentOptions(
                    model="claude-opus-4-5-20251101",
                    system_prompt=system_prompt,
                    mcp_servers={
                        "mcp-eval": {
                            "type": "http",
                            "url": mcp_url
                        }
                    },
                    allowed_tools=allowed_tools,
                    permission_mode="bypassPermissions",
                    max_buffer_size=50 * 1024 * 1024,  # 50MB buffer for large tool outputs
                )
            ):
                conversation.append(message)
        except subprocess.CalledProcessError as e:
            print(f"[{self.mode}/{scenario}] 🔧 ProcessError: Command failed with exit code {e.returncode}")
            print(f"  Command: {e.cmd}")
            if e.stdout:
                print(f"  Stdout: {e.stdout.decode() if isinstance(e.stdout, bytes) else e.stdout}")
            if e.stderr:
                print(f"  Stderr: {e.stderr.decode() if isinstance(e.stderr, bytes) else e.stderr}")
            conversation.append({"error": f"ProcessError (exit {e.returncode}): {e.cmd}"})
        except RateLimitError as e:
            print(f"[{self.mode}/{scenario}] ⚠️ RateLimitError: API rate limit exceeded")
            print(f"  Error: {e}")
            if hasattr(e, 'response'):
                print(f"  Response headers: {e.response.headers if e.response else 'N/A'}")
            conversation.append({"error": f"RateLimitError: {e}"})
        except APITimeoutError as e:
            print(f"[{self.mode}/{scenario}] ⏱️ APITimeoutError: Request timed out")
            print(f"  Error: {e}")
            conversation.append({"error": f"APITimeoutError: {e}"})
        except APIConnectionError as e:
            print(f"[{self.mode}/{scenario}] 🔌 APIConnectionError: Connection failed")
            print(f"  Error: {e}")
            if hasattr(e, '__cause__'):
                print(f"  Underlying cause: {e.__cause__}")
            conversation.append({"error": f"APIConnectionError: {e}"})
        except APIStatusError as e:
            print(f"[{self.mode}/{scenario}] 🚫 APIStatusError: HTTP {e.status_code}")
            print(f"  Status code: {e.status_code}")
            print(f"  Error: {e}")
            if hasattr(e, 'response'):
                print(f"  Response body: {e.response.text if e.response else 'N/A'}")
            conversation.append({"error": f"APIStatusError {e.status_code}: {e}"})
        except APIError as e:
            print(f"[{self.mode}/{scenario}] ❌ APIError: {e}")
            print(f"  Error type: {type(e).__name__}")
            if hasattr(e, 'status_code'):
                print(f"  Status code: {e.status_code}")
            if hasattr(e, 'body'):
                print(f"  Response body: {e.body}")
            import traceback
            traceback.print_exc()
            conversation.append({"error": f"APIError: {e}"})
        except Exception as e:
            print(f"[{self.mode}/{scenario}] ❌ Unexpected error: {e}")
            print(f"[{self.mode}/{scenario}] Error type: {type(e).__name__}")
            import traceback
            print(f"[{self.mode}/{scenario}] Full traceback:")
            traceback.print_exc()
            if hasattr(e, '__dict__'):
                print(f"[{self.mode}/{scenario}] Error attributes: {e.__dict__}")
            conversation.append({"error": f"Unexpected error: {e}"})

        # Save full transcript as proper JSON
        transcript_file = self.transcripts_dir / f"{scenario}.json"
        transcript_file.parent.mkdir(parents=True, exist_ok=True)

        # Convert messages to dictionaries for JSON serialization
        conversation_json = []
        for msg in conversation:
            if hasattr(msg, "model_dump"):
                # Pydantic v2 models
                conversation_json.append(msg.model_dump())
            elif hasattr(msg, "dict"):
                # Pydantic v1 models
                conversation_json.append(msg.dict())
            elif isinstance(msg, dict):
                # Already a dict
                conversation_json.append(msg)
            else:
                # Fallback: convert to dict using __dict__ or vars()
                conversation_json.append(vars(msg) if hasattr(msg, '__dict__') else str(msg))

        transcript_file.write_text(json.dumps(conversation_json, indent=2, default=str))

        # Extract final findings
        findings = self.extract_findings(conversation)
        if findings:
            return findings.result, findings.duration_ms, findings.num_turns, findings.total_cost_usd
        return "no results", 0, 0, 0

    def extract_findings(self, conversation: List) -> ResultMessage | None:
        """Extract investigation findings from conversation"""
        # First, look for the last ResultMessage (highest priority)
        for message in reversed(conversation):
            if isinstance(message, ResultMessage):
                return message

        return None

    @retry(
        stop=stop_after_attempt(6),
        wait=wait_exponential(multiplier=1, min=2, max=60),
        retry=retry_if_exception_type((RateLimitError, APIError)) & retry_if_exception(is_retryable_api_error),
        before_sleep=lambda retry_state: print(
            f"[{retry_state.kwargs.get('self').mode}] API error on attempt {retry_state.attempt_number}: "
            f"{retry_state.outcome.exception() if retry_state.outcome else 'Unknown error'}. "
            f"Retrying in {retry_state.next_action.sleep:.1f if retry_state.next_action and retry_state.next_action.sleep else 0}s..."
        ) if retry_state.kwargs.get('self') else None,
        reraise=True
    )
    async def grade_findings(self, scenario: str, findings: str) -> Dict:
        """Grade findings using LLM and SCENARIO.md rubric (with retry on API errors)"""
        # Read rubric
        scenario_file = SCENARIOS_DIR / scenario / "SCENARIO.md"
        rubric = scenario_file.read_text()

        # Use Anthropic API to grade
        client = Anthropic()

        grading_prompt = f"""You are an expert evaluator for SRE diagnostic scenarios.

SCENARIO RUBRIC:
{rubric}

AGENT FINDINGS:
{findings}

Grade the agent's findings according to the rubric in the scenario. Provide:
1. Overall score (0-100)
2. Breakdown by rubric category
3. What was done well
4. What was missed

Return your evaluation as JSON with this structure:
{{
    "overall_score": 85,
    "category_scores": {{
        "process_identification": 25,
        "resource_identification": 20,
        "root_cause_analysis": 25,
        "mitigation_proposal": 15
    }},
    "strengths": ["Correctly identified process", "Good root cause analysis"],
    "weaknesses": ["Missing specific mitigation steps"],
    "key_terms_found": ["CPU", "100%", "workload"],
    "key_terms_missing": ["SHA256", "hashing"]
}}

Only return valid JSON, no additional text."""

        try:
            response = client.messages.create(
                model="claude-opus-4-5-20251101",
                max_tokens=2000,
                messages=[{"role": "user", "content": grading_prompt}]
            )

            # Parse JSON from response
            response_text = response.content[0].text
            # Try to extract JSON if there's extra text
            if "```json" in response_text:
                response_text = response_text.split("```json")[1].split("```")[0]
            elif "```" in response_text:
                response_text = response_text.split("```")[1].split("```")[0]

            grade_json = json.loads(response_text.strip())
            return grade_json
        except Exception as e:
            print(f"[{self.mode}/{scenario}] Grading error: {e}")
            return {
                "overall_score": 0,
                "error": str(e),
                "category_scores": {},
                "strengths": [],
                "weaknesses": ["Grading failed"],
                "key_terms_found": [],
                "key_terms_missing": []
            }

    async def teardown_scenario(self, scenario: str):
        """Teardown scenario in VM"""
        teardown_script = SCENARIOS_DIR / scenario / "teardown.sh"
        cmd = [str(teardown_script), self.vm_name]
        print(f"[{self.mode}/{scenario}] Running command: {' '.join(cmd)}")
        process = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE
        )
        stdout, stderr = await process.communicate()
        if process.returncode is None or process.returncode != 0:
            print(f"[{self.mode}/{scenario}] teardown.sh failed with code {process.returncode}")
            if stdout:
                print(f"[{self.mode}/{scenario}] stdout: {stdout.decode()}")
            if stderr:
                print(f"[{self.mode}/{scenario}] stderr: {stderr.decode()}")
            raise subprocess.CalledProcessError(process.returncode or -1, str(teardown_script))

    def write_result(self, result: Dict):
        """Write result to JSONL file"""
        self.results_file.parent.mkdir(parents=True, exist_ok=True)
        with open(self.results_file, "a") as f:
            f.write(json.dumps(result) + "\n")

        score = result.get('score', {}).get('overall_score', 'N/A')
        status = result.get('status', 'unknown')
        print(f"[{self.mode}] Result: {result['scenario']} - Score: {score} - Status: {status}")

    async def grade_existing_transcripts(self):
        """Grade existing transcripts without running investigations (for debugging/re-grading)"""
        print(f"\n{'='*60}")
        print(f"Grading existing transcripts for mode: {self.mode}")
        print(f"{'='*60}\n")

        for i, scenario in enumerate(self.scenarios_to_run):
            transcript_file = self.transcripts_dir / f"{scenario}.json"

            if not transcript_file.exists():
                print(f"[{self.mode}/{scenario}] ⚠ Transcript not found: {transcript_file}")
                self.write_result({
                    "mode": self.mode,
                    "scenario": scenario,
                    "status": "error",
                    "error": "Transcript file not found",
                    "timestamp": datetime.now().isoformat()
                })
                continue

            try:
                print(f"\n[{self.mode}] ━━━ Grading {scenario} ({i+1}/{len(self.scenarios_to_run)}) ━━━")

                # Load transcript
                with open(transcript_file, 'r') as f:
                    conversation_json = json.load(f)

                # Convert back to message objects for extract_findings
                from types import SimpleNamespace
                conversation = []
                for msg_dict in conversation_json:
                    # Create a simple object that has the same attributes
                    msg = SimpleNamespace(**msg_dict)
                    conversation.append(msg)

                # Extract findings
                findings = self.extract_findings(conversation)

                if not findings:
                    print(f"[{self.mode}/{scenario}] ⚠ No findings found in transcript")
                    self.write_result({
                        "mode": self.mode,
                        "scenario": scenario,
                        "status": "error",
                        "error": "No findings in transcript",
                        "timestamp": datetime.now().isoformat()
                    })
                    continue

                # Grade findings
                print(f"[{self.mode}/{scenario}] Grading findings...")
                grade = await self.grade_findings(scenario, findings.result or "no findings")

                # Write result
                result = {
                    "mode": self.mode,
                    "scenario": scenario,
                    "findings": findings.result,
                    "score": grade,
                    "status": "completed",
                    "timestamp": datetime.now().isoformat(),
                    "duration_ms": findings.duration_ms,
                    "turns": findings.num_turns,
                    "cost": findings.total_cost_usd,
                }

                self.write_result(result)
                print(f"[{self.mode}] ✓ {scenario} graded - Score: {grade.get('overall_score', 'N/A')}")

            except Exception as e:
                print(f"[{self.mode}] ✗ Error grading {scenario}: {e}")
                import traceback
                traceback.print_exc()
                self.write_result({
                    "mode": self.mode,
                    "scenario": scenario,
                    "status": "error",
                    "error": str(e),
                    "timestamp": datetime.now().isoformat()
                })

        print(f"\n{'='*60}")
        print(f"Grading complete for mode: {self.mode}")
        print(f"Results written to: {self.results_file}")
        print(f"{'='*60}\n")


def parse_args():
    """Parse command-line arguments"""
    parser = argparse.ArgumentParser(
        description="MCP Evaluation Framework - Run Claude Code investigations across scenarios and modes",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Run all scenarios for all modes (default)
  python evaluate.py

  # Run specific scenarios for specific modes
  python evaluate.py --modes bash tools --scenarios high-cpu-usage port-conflict

  # Run all scenarios for one mode
  python evaluate.py --modes bash

  # Grade only (use existing run directory)
  python evaluate.py --grade-only --run-dir results/run-20260120_120000
        """
    )

    parser.add_argument(
        "--modes",
        nargs="+",
        choices=["bash", "safe-shell", "tools"],
        default=None,
        help="Modes to evaluate (default: all modes)"
    )

    parser.add_argument(
        "--scenarios",
        nargs="+",
        default=None,
        help="Scenarios to evaluate (default: all scenarios)"
    )

    parser.add_argument(
        "--grade-only",
        action="store_true",
        help="Grade existing transcripts without running investigations (requires --run-dir)"
    )

    parser.add_argument(
        "--run-dir",
        type=Path,
        help="Existing run directory (required for --grade-only)"
    )

    args = parser.parse_args()

    # Validation
    if args.grade_only and not args.run_dir:
        parser.error("--grade-only requires --run-dir")

    if args.run_dir and not args.grade_only:
        parser.error("--run-dir can only be used with --grade-only")

    # Validate scenarios
    if args.scenarios:
        invalid = set(args.scenarios) - set(SCENARIOS)
        if invalid:
            parser.error(f"Invalid scenarios: {', '.join(invalid)}")

    return args


async def main():
    """Run evaluations in parallel for all modes"""
    args = parse_args()

    # Determine modes and scenarios to run
    modes = args.modes if args.modes else MODES
    scenarios = args.scenarios if args.scenarios else SCENARIOS

    print("\n" + "="*60)
    print("MCP Evaluation Framework")
    print("="*60)
    print(f"Scenarios: {len(scenarios)} - {', '.join(scenarios)}")
    print(f"Modes: {', '.join(modes)}")
    print(f"Total evaluations: {len(scenarios) * len(modes)}")
    if args.grade_only:
        print(f"Mode: GRADE ONLY (using existing transcripts)")
    print("="*60 + "\n")

    # Determine run directory
    if args.grade_only:
        run_dir = args.run_dir
        if not run_dir.exists():
            print(f"ERROR: Run directory does not exist: {run_dir}")
            return
        print(f"Using existing run directory: {run_dir}\n")
    else:
        # Create timestamped run directory
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        run_dir = RESULTS_DIR / f"run-{timestamp}"
        run_dir.mkdir(parents=True, exist_ok=True)
        print(f"Run directory: {run_dir}\n")

    # Check for API key
    if not os.getenv("ANTHROPIC_API_KEY"):
        print("ERROR: ANTHROPIC_API_KEY environment variable not set")
        print("Please set it with: export ANTHROPIC_API_KEY=your-api-key")
        return

    if not args.grade_only:
        # Setup all VMs sequentially to avoid Lima race conditions
        print("="*60)
        print("Setting up VMs sequentially to avoid race conditions...")
        print("="*60)
        runners = []
        for mode in modes:
            runner = EvaluationRunner(mode, run_dir)
            runner.scenarios_to_run = scenarios  # Set scenarios filter
            await runner.setup_vm()
            runners.append(runner)

        print("\n" + "="*60)
        print("All VMs ready. Starting parallel evaluations...")
        print("="*60 + "\n")

        # Run all scenarios in parallel
        await asyncio.gather(*[runner.run_scenarios() for runner in runners])
    else:
        # Grade-only mode: load existing transcripts and grade them
        print("="*60)
        print("Grading existing transcripts...")
        print("="*60 + "\n")

        for mode in modes:
            runner = EvaluationRunner(mode, run_dir)
            runner.scenarios_to_run = scenarios
            await runner.grade_existing_transcripts()

    print("\n" + "="*60)
    print("All evaluations complete!")
    print("="*60)
    print(f"Run directory: {run_dir}")
    print(f"\nTo consolidate results:")
    print(f"  python scripts/consolidate_results.py {run_dir}")
    print("="*60 + "\n")


if __name__ == "__main__":
    asyncio.run(main())
