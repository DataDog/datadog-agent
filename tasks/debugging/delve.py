import os
import sys
from pathlib import Path


class Delve:
    dlv_cmd: Path | str

    def __init__(self):
        if sys.platform == 'win32':
            self.dlv_cmd = 'dlv.exe'
        else:
            self.dlv_cmd = 'dlv'

    def __interactive_cmd(self, cmd):
        os.system(f'{self.dlv_cmd} {cmd}')

    def core(self, binary: Path | str, core: Path | str):
        return self.__interactive_cmd(f'core "{binary}" "{core}"')
