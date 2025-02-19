from pathlib import Path

from tasks.debugging.path_store import PathStore


class Artifacts:
    def __init__(self, project: str, job: str, path_store: PathStore):
        self.__path_store = path_store
        self._project = project
        self._job = job
        self._version = None  # noqa
        self._pipeline = None  # noqa

    def get(self) -> Path | None:
        return self.__path_store.get_directory(f"{self.key()}/artifacts")

    def add(self, path: str | Path) -> None:
        path = Path(path)
        if not path.is_dir():
            raise ValueError(f"{path} is not a directory")
        if not list(path.iterdir()):
            return
        self.__path_store.add_directory(f"{self.key()}/artifacts", path)

    def _get_text_property(self, attr: str) -> str | None:
        value = getattr(self, f"_{attr}")
        if value is None:
            data = self.__path_store.get(f"{self.key()}/{attr}.txt")
            if not data:
                return None
            value = data.decode('utf-8').strip()
            setattr(self, f"_{attr}", value)
        return value

    def _set_text_property(self, attr: str, value: str) -> None:
        self.__path_store.add(f"{self.key()}/{attr}.txt", value.encode('utf-8'))
        setattr(self, f"_{attr}", value)

    @property
    def version(self) -> str | None:
        return self._get_text_property("version")

    @version.setter
    def version(self, value: str) -> None:
        self._set_text_property("version", value)

    @property
    def pipeline(self) -> str | None:
        return self._get_text_property("pipeline")

    @pipeline.setter
    def pipeline(self, value: str) -> None:
        self._set_text_property("pipeline", value)

    @property
    def project(self) -> str | None:
        return self._get_text_property("project")

    @project.setter
    def project(self, value: str) -> None:
        self._set_text_property("project", value)

    def key(self) -> str:
        return self.makekey(self._project, self._job)

    @classmethod
    def makekey(cls, project: str, job: str) -> str:
        return f"{project}/{job}"


class ArtifactStore:
    def __init__(self, path: str | Path):
        self.path_store = PathStore(Path(path))

    def add(self, project_id: str, job_id: str, artifacts_path: str | Path | None = None) -> Artifacts:
        artifacts = Artifacts(project_id, job_id, self.path_store)
        if artifacts_path:
            artifacts.add(artifacts_path)
        return artifacts

    def get(self, project_id: str, job_id: str) -> Artifacts | None:
        key = Artifacts.makekey(project_id, job_id)
        if self.path_store.get_directory(key):
            return Artifacts(project_id, job_id, self.path_store)
        return None
