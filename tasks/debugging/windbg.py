import os
import shutil
from pathlib import Path


class Windbg:
    def __init__(self):
        # prefer newer windbg if available
        if shutil.which('windbgx.exe'):
            self.windbg_cmd = 'windbgx.exe'
        else:
            self.windbg_cmd = 'windbg.exe'

    def open_dump(self, path: Path | str):
        os.system(f'cmd.exe /c start {self.windbg_cmd} -z "{path}"')
