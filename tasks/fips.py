"""
FIPS compliance tasks
"""

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit


@task
def verify_binary_symbols(ctx: Context, path: str, symbol: str, should_find: bool = True):
    print(f"Looking for symbol '{symbol}' in '{path}' binary ... (expecting to find: {should_find})")

    result = ctx.run(f"go tool nm {path} | grep -c '{symbol}'", hide=True, warn=True)
    if result.stderr:
        raise Exit(f"Command failed with stderr: {result.stderr}", code=1)

    count = int(result.stdout)
    if should_find:
        if count == 0:
            raise Exit(f"Failure: expected to find symbol '{symbol}' but no symbol was found.", code=2)
        print(f"Sucess: symbol '{symbol}' found {count} times in binary.")
    else:
        if count != 0:
            raise Exit(f"Failure: expected no symbol '{symbol}' but found {count} occurences.", code=2)
        print(f"Sucess: symbol '{symbol}' not found in binary as expected.")
