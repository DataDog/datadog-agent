"""Linting-related tasks for shell scripts (mostly shellcheck)"""

from __future__ import annotations

from tempfile import TemporaryDirectory

from tasks.libs.linter.gitlab_exceptions import (
    FailureLevel,
    MultiGitlabLintFailure,
    SingleGitlabLintFailure,
)

# - SC2086 corresponds to using variables in this way $VAR instead of "$VAR" (used in every jobs).
# - SC2016 corresponds to avoid using '$VAR' inside single quotes since it doesn't expand.
# - SC2046 corresponds to avoid using $(...) to prevent word splitting.
DEFAULT_SHELLCHECK_EXCLUDES = 'SC2059,SC2028,SC2086,SC2016,SC2046'


def flatten_script(script: str | list[str]) -> str:
    """Flatten a script into a single string."""

    if isinstance(script, list):
        return '\n'.join(flatten_script(line) for line in script)

    if script is None:
        return ''

    return script.strip()


def shellcheck_linter(
    ctx,
    scripts: dict[str, str],
    exclude: str,
    shellcheck_args: str,
    fail_fast: bool,
    only_errors=False,
):
    """Lints bash scripts within `scripts` using shellcheck.

    Args:
        scripts: A dictionary of job names and their scripts.
        exclude: A comma separated list of shellcheck error codes to exclude.
        shellcheck_args: Additional arguments to pass to shellcheck.
        fail_fast: If True, will stop at the first error.
        only_errors: Show only errors, not warnings.

    Note:
        Will raise an Exit if any errors are found.
    """

    exclude = ' '.join(f'-e {e}' for e in exclude.split(','))

    results = []
    with TemporaryDirectory() as tmpdir:
        for i, (script_name, script) in enumerate(scripts.items()):
            with open(f'{tmpdir}/{i}.sh', 'w') as f:
                f.write(script)

            res = ctx.run(f"shellcheck {shellcheck_args} {exclude} '{tmpdir}/{i}.sh'", warn=True, hide=True)
            if res.stderr or res.stdout:
                if res.return_code or not only_errors:
                    results.append(
                        SingleGitlabLintFailure(
                            _details=f"Shellcheck failed ! {res.stderr} - {res.stdout}".strip(),
                            failing_job_name=script_name,
                            _level=FailureLevel.ERROR if res.return_code != 0 else FailureLevel.WARNING,
                        )
                    )

                if res.return_code and fail_fast:
                    break

        if results:
            if len(results) == 1:
                raise results[0]

            raise MultiGitlabLintFailure(
                failures=results,
            )
