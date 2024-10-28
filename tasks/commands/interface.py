from __future__ import annotations

import sys
from typing import TYPE_CHECKING

from tasks.libs.common.color import color_message

if TYPE_CHECKING:
    from collections.abc import Callable, Iterable

    from invoke import Result


class CLI:
    """
    CLI interface to run command lines.
    """

    def _format_command(self, command: Iterable[str]) -> str:
        return " ".join(c if ' ' not in c else f"'{c}'" for c in command)

    def run_command(self, command: Iterable[str]) -> Result | None:
        from invoke import run

        cmd = self._format_command(command)
        print(color_message(cmd, "orange"))
        return run(cmd, pty=True)

    @staticmethod
    def _isatty() -> bool:
        isatty: Callable[[], bool] | None = getattr(sys.stdout, 'isatty', None)
        if isatty is not None:
            try:
                return isatty()
            except ValueError:
                pass

        return False
