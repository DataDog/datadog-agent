from time import sleep

from invoke import Context
from invoke.exceptions import Exit


def run_command_with_retry(ctx: Context, command: str, max_retry: int = 3):
    for retry in range(max_retry):
        result = ctx.run(command, warn=True)
        if result.exited is None or result.exited > 0:
            wait = 10 ** (retry + 1)
            print(f"[{retry + 1} / {max_retry}] Failed running command `{command}`, retrying in {wait} seconds")
            sleep(wait)
            continue
        break
    else:
        raise Exit("Failed to run command", code=1)
