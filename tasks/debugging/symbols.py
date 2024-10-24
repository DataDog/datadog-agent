from pathlib import Path

from tasks.debugging.path_store import PathStore


class SymbolStore(PathStore):
    def add(self, version: str, path: str | Path) -> Path:
        dst = Path(self.path, version, 'symbols')
        self.add_directory(str(dst), path)
        return dst

    def get(self, version: str) -> Path | None:
        p = Path(self.path, version, 'symbols')
        return self.get_directory(str(p))
