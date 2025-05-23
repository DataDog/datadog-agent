"""Linting-related tasks for shell scripts (mostly shellcheck)"""

from __future__ import annotations

from tempfile import TemporaryDirectory

from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import gitlab_section

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
    use_bat: str | None,
    only_errors=False,
):
    """Lints bash scripts within `scripts` using shellcheck.

    Args:
        scripts: A dictionary of job names and their scripts.
        exclude: A comma separated list of shellcheck error codes to exclude.
        shellcheck_args: Additional arguments to pass to shellcheck.
        fail_fast: If True, will stop at the first error.
        use_bat: If True (or None), will (try to) use bat to display the script.
        only_errors: Show only errors, not warnings.

    Note:
        Will raise an Exit if any errors are found.
    """

    exclude = ' '.join(f'-e {e}' for e in exclude.split(','))

    if use_bat is None:
        use_bat = ctx.run('which bat', warn=True, hide=True)
    elif use_bat.casefold() == 'false':
        use_bat = False

    results = {}
    with TemporaryDirectory() as tmpdir:
        for i, (script_name, script) in enumerate(scripts.items()):
            with open(f'{tmpdir}/{i}.sh', 'w') as f:
                f.write(script)

            res = ctx.run(f"shellcheck {shellcheck_args} {exclude} '{tmpdir}/{i}.sh'", warn=True, hide=True)
            if res.stderr or res.stdout:
                if res.return_code or not only_errors:
                    results[script_name] = {
                        'output': (res.stderr + '\n' + res.stdout + '\n').strip(),
                        'code': res.return_code,
                        'id': i,
                    }

                if res.return_code and fail_fast:
                    break

        if results:
            with gitlab_section(color_message("Shellcheck errors / warnings", color=Color.ORANGE), collapsed=True):
                for script, result in sorted(results.items()):
                    with gitlab_section(f"Shellcheck errors for {script}"):
                        print(f"--- {color_message(script, Color.BLUE)} ---")
                        print(f'[{script}] Script:')
                        if use_bat:
                            res = ctx.run(
                                f"bat --color=always --file-name={script} -l bash {tmpdir}/{result['id']}.sh", hide=True
                            )
                            # Avoid buffering issues
                            print(res.stderr)
                            print(res.stdout)
                        else:
                            with open(f'{tmpdir}/{result["id"]}.sh') as f:
                                print(f.read())
                        print(f'\n[{script}] {color_message("Error", Color.RED)}:')
                        print(result['output'])

            if any(result['code'] != 0 for result in results.values()):
                raise Exit(
                    f"{color_message('Error', Color.RED)}: {len(results)} shellcheck errors / warnings found, please fix them",
                    code=1,
                )
