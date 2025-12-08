"""
Tasks for logs processor token rule generation
"""

import sys
from pathlib import Path

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit


@task
def translate_regex_to_tokens(ctx, regex_pattern, rule_name, replacement="[REDACTED]", description=""):
    """
    Translate a regex pattern to token-based rule using Qwen2.5-Coder-3B.
    
    Args:
        regex_pattern: The regex pattern to translate
        rule_name: Name for the rule
        replacement: Replacement text for matches
        description: Description of what the pattern matches
    
    Example:
        dda inv logs-processor.translate-regex-to-tokens --regex-pattern="\\d{3}-\\d{2}-\\d{4}" --rule-name="ssn" --replacement="[SSN]"
    """
    # Find the standalone script
    script_path = Path(__file__).parent.parent / "pkg" / "logs" / "processor" / "translate_regex.py"
    
    if not script_path.exists():
        raise Exit(f"Could not find translate_regex.py at {script_path}", code=1)
    
    # Build command arguments
    cmd_parts = [
        sys.executable,  # Use the same Python interpreter as invoke
        str(script_path),
        "--regex", regex_pattern,
        "--name", rule_name,
        "--replacement", replacement,
    ]
    
    if description:
        cmd_parts.extend(["--description", description])
    
    # Run the standalone script
    print(f"Translating regex pattern: {regex_pattern}")
    print(f"Rule name: {rule_name}")
    print()
    
    result = ctx.run(" ".join(f'"{part}"' if " " in str(part) else str(part) for part in cmd_parts), pty=True)
    
    if result.exited != 0:
        raise Exit(f"Translation failed with exit code {result.exited}", code=result.exited)
