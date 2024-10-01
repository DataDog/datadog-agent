import os
from pathlib import Path

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.vscode import VSCODE_DIR, VSCODE_LAUNCH_FILE


@task(default=True)
def debug(_, wait=True, host='localhost', port=5678):
    """
    Launch debugger to debug in vs-code or other IDEs using debugpy.

    Usage to debug `inv invoke-unit-tests.run`:
    > inv debug invoke-unit-tests.run
    > # In vscode, launch the debugger with the configuration "Remote Debug Tasks"
    > # The debugger is attached !
    """
    try:
        import debugpy
    except ImportError as e:
        raise Exit(
            'debugpy is not installed, you should update your requirements within tasks/requirements.txt', code=1
        ) from e

    os.environ['TASKS_DEBUG'] = '1'

    if not (Path(VSCODE_DIR) / VSCODE_LAUNCH_FILE).exists():
        print(
            f"{color_message('warning:', Color.ORANGE)} {color_message('(For VS Code users)', Color.BLUE)} No launch.json file found, you should run `inv vscode.setup-launch` to have a debug configuration.",
        )

    debugpy.listen((host, port))
    if wait:
        print(color_message('info:', Color.BLUE), f'Waiting for debugger to attach on port {port}...')
        debugpy.wait_for_client()
