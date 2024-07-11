import os

from invoke import task

from tasks.libs.common.color import Color, color_message


@task(default=True)
def debug(_, wait=True, host='localhost', port=5678):
    """
    Launch debugger to debug in vs-code or other IDEs using debugpy.

    Usage to debug `inv invoke-unit-tests`:
    > inv debug linter.python
    > # In vscode, launch the debugger with the configuration "Remote Debug Tasks"
    > # The debugger is attached !
    """
    import debugpy

    os.environ['TASKS_DEBUG'] = '1'

    init_debug_config(_, verbose=False)

    debugpy.listen((host, port))
    if wait:
        print(color_message('info:', Color.BLUE), f'Waiting for debugger to attach on port {port}...')
        debugpy.wait_for_client()


@task
def init_debug_config(_, force=False, verbose=True):
    # Already setup
    if not force and os.path.exists(os.path.join('.vscode', 'launch.json')):
        if verbose:
            print(color_message('info:', Color.BLUE), 'Debug config already exists, skipping...')
        return

    os.makedirs('.vscode', exist_ok=True)
    with open(os.path.join('.vscode', 'launch.json'), 'w') as f:
        f.write("""{
    // Use IntelliSense to learn about possible attributes.
    // Hover to view descriptions of existing attributes.
    // For more information, visit: https://go.microsoft.com/fwlink/?linkid=830387
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Remote Debug Tasks",
            "type": "debugpy",
            "request": "attach",
            "connect": {
                "host": "localhost",
                "port": 5678
            },
            "pathMappings": [
                {
                    "localRoot": "${workspaceFolder}",
                    "remoteRoot": "."
                }
            ]
        }
    ]
}""")
    if verbose:
        print(color_message('success:', Color.GREEN), 'Debug config generated to .vscode/launch.json')
