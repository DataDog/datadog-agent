from pathlib import Path

from tasks.debugging.path_store import PathStore


class SymbolStore:
    def __init__(self, path: Path | str):
        self.path_store = PathStore(path)

    def add(self, version: str, platform: str, arch: str, path: str | Path) -> Path:
        k = f'{version}/{platform}/{arch}/symbols'
        self.path_store.add_directory(k, Path(path))
        return Path(self.path_store.path, k)

    def get(self, version: str, platform: str, arch: str) -> Path | None:
        k = f'{version}/{platform}/{arch}/symbols'
        return self.path_store.get_directory(k)
