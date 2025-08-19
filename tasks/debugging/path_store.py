import shutil
from pathlib import Path


class PathStore:
    def __init__(self, path: str | Path):
        self.path = Path(path)

    def add(self, key: str, data: bytes) -> None:
        file_path = self.path / key
        file_path.parent.mkdir(parents=True, exist_ok=True)
        file_path.write_bytes(data)

    def get(self, key: str) -> bytes | None:
        file_path = self.path / key
        if file_path.exists():
            return file_path.read_bytes()
        return None

    def add_directory(self, key: str, src: Path) -> None:
        dst = self.path / key
        dst.mkdir(parents=True, exist_ok=True)
        shutil.copytree(src, dst, dirs_exist_ok=True)

    def get_directory(self, key: str) -> Path | None:
        dir_path = self.path / key
        if dir_path.exists():
            return dir_path
        return None
