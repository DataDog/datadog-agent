"""Provides functions to import / export go modules from / to yaml files."""

from __future__ import annotations

import os
import subprocess
import sys
from collections.abc import Callable
from dataclasses import dataclass
from functools import lru_cache
from glob import glob
from pathlib import Path
from typing import ClassVar

import yaml


class GoModuleDumper(yaml.SafeDumper):
    """SafeDumper that ignores aliases. (no references for readability)"""

    def ignore_aliases(self, _):  # noqa
        return True


@dataclass
class GoModule:
    """A Go module abstraction.

    See:
        Documentation can be found in <docs/dev/modules.md>.

    Args:
        independent: specifies whether this modules is supposed to exist independently of the datadog-agent module. If True, a check will run to ensure this is true.

    Usage:
        A module is defined within a module.yml next to the go.mod file containing the following fields by default (these can be omitted if the default value is used):
        > condition: always
        > importable: true
        > independent: false
        > lint_targets:
        > - .
        > should_tag: true
        > targets:
        > - .
        > used_by_otel: false
    """

    # Possible conditions for GoModule.condition
    CONDITIONS: ClassVar[dict[str, Callable]] = {
        'always': lambda: True,
        'never': lambda: False,
        'is_linux': lambda: sys.platform == "linux",
    }

    # Posix path of the module's directory
    path: str
    targets: list[str] | None = None
    condition: str = 'always'
    should_tag: bool = True
    # HACK: Workaround for modules that can be tested, but not imported (eg. gohai), because
    # they define a main package
    # A better solution would be to automatically detect if a module contains a main package,
    # at the cost of spending some time parsing the module.
    importable: bool = True
    independent: bool = False
    lint_targets: list[str] | None = None
    used_by_otel: bool = False

    @staticmethod
    def from_dict(path: str, data: dict[str, object]) -> GoModule:
        default = GoModule.get_default_attributes()

        return GoModule(
            path=path,
            targets=data.get("targets", default["targets"]),
            lint_targets=data.get("lint_targets", default["lint_targets"]),
            condition=data.get("condition", default["condition"]),
            should_tag=data.get("should_tag", default["should_tag"]),
            importable=data.get("importable", default["importable"]),
            independent=data.get("independent", default["independent"]),
            used_by_otel=data.get("used_by_otel", default["used_by_otel"]),
        )

    @staticmethod
    def parse_path(dir_path: str | Path, base_dir: Path | None = None) -> tuple[str, Path, Path, Path]:
        """Returns path components for a module.

        Here is the path:
            <base_dir>/<dir_path>/module.yml
            ---------------------            -> full_path
                       ----------            -> module_path (contains only '/')
        """

        base_dir = base_dir or Path.cwd()
        dir_path = Path(dir_path) if isinstance(dir_path, str) else dir_path
        module_path = dir_path.as_posix()
        full_path = base_dir / (dir_path if isinstance(dir_path, Path) else Path(dir_path))

        return module_path, base_dir, dir_path, full_path

    @staticmethod
    def from_file(dir_path: str | Path, base_dir: Path | None = None) -> GoModule:
        """Load from a module.yml file.

        The absolute full path is '<base_dir>/<dir_path>/module.yml'.
        """

        module_path, base_dir, dir_path, full_path = GoModule.parse_path(dir_path, base_dir)

        assert full_path.is_dir(), f"Directory {full_path} does not exist"

        with open(full_path / 'module.yml') as file:
            data = yaml.safe_load(file)

            return GoModule.from_dict(module_path, data)

    @staticmethod
    def get_default_attributes() -> dict[str, object]:
        attrs = GoModule('.').to_dict(remove_defaults=False)
        attrs.pop('path')

        return attrs

    def __post_init__(self):
        self.targets = self.targets or ["."]
        self.lint_targets = self.lint_targets or self.targets

        self._dependencies = None

    def to_dict(self, remove_defaults=True) -> dict[str, object]:
        """Convert to dictionary.

        Args:
            remove_defaults: Remove default values from the dictionary.
        """

        attrs = {
            "path": self.path,
            "targets": self.targets,
            "lint_targets": self.lint_targets,
            "condition": self.condition,
            "should_tag": self.should_tag,
            "importable": self.importable,
            "independent": self.independent,
            "used_by_otel": self.used_by_otel,
        }

        if remove_defaults:
            default_attrs = GoModule.get_default_attributes()

            for key, value in default_attrs.items():
                if key in attrs and attrs[key] == value:
                    del attrs[key]

        return attrs

    def to_file(self, base_dir: Path | None = None):
        """Save the module to a module.yml file.

        Args:
            base_dir: Root directory of the agent repository.
        """

        _, base_dir, dir_path, full_path = GoModule.parse_path(self.path, base_dir)

        assert full_path.is_dir(), f"Directory {dir_path} does not exist"

        with open(full_path / 'module.yml', "w") as file:
            data = self.to_dict()
            del data['path']

            yaml.dump(data, file, Dumper=GoModuleDumper)

    def verify_condition(self) -> bool:
        """Verify that the module condition is met."""
        function = GoModule.CONDITIONS[self.condition]

        return function()

    def __version(self, agent_version):
        """Return the module version for a given Agent version.
        >>> mods = [GoModule("."), GoModule("pkg/util/log")]
        >>> [mod.__version("7.27.0") for mod in mods]
        ["v7.27.0", "v0.27.0"]
        """
        if self.path == ".":
            return "v" + agent_version

        return "v0" + agent_version[1:]

    def __compute_dependencies(self):
        """
        Computes the list of github.com/DataDog/datadog-agent/ dependencies of the module.
        """
        base_path = os.getcwd()
        mod_parser_path = os.path.join(base_path, "internal", "tools", "modparser")

        if not os.path.isdir(mod_parser_path):
            raise Exception(f"Cannot find go.mod parser in {mod_parser_path}")

        try:
            output = subprocess.check_output(
                ["go", "run", ".", "-path", os.path.join(base_path, self.path), "-prefix", AGENT_MODULE_PATH_PREFIX],
                cwd=mod_parser_path,
            ).decode("utf-8")
        except subprocess.CalledProcessError as e:
            print(f"Error while calling go.mod parser: {e.output}")
            raise e

        # Remove github.com/DataDog/datadog-agent/ from each line
        return [line[len(AGENT_MODULE_PATH_PREFIX) :] for line in output.strip().splitlines()]

    # FIXME: Change when Agent 6 and Agent 7 releases are decoupled
    def tag(self, agent_version):
        """Return the module tag name for a given Agent version.
        >>> mods = [GoModule("."), GoModule("pkg/util/log")]
        >>> [mod.tag("7.27.0") for mod in mods]
        [["6.27.0", "7.27.0"], ["pkg/util/log/v0.27.0"]]
        """
        if self.path == ".":
            return ["6" + agent_version[1:], "7" + agent_version[1:]]

        return [f"{self.path}/{self.__version(agent_version)}"]

    def full_path(self):
        """Return the absolute path of the Go module."""
        return os.path.abspath(self.path)

    def go_mod_path(self):
        """Return the absolute path of the Go module go.mod file."""
        return self.full_path() + "/go.mod"

    @property
    def dependencies(self):
        if not self._dependencies:
            self._dependencies = self.__compute_dependencies()
        return self._dependencies

    @property
    def import_path(self):
        """Return the Go import path of the Go module
        >>> mods = [GoModule("."), GoModule("pkg/util/log")]
        >>> [mod.import_path for mod in mods]
        ["github.com/DataDog/datadog-agent", "github.com/DataDog/datadog-agent/pkg/util/log"]
        """
        path = AGENT_MODULE_PATH_PREFIX.removesuffix('/')
        if self.path != ".":
            path += "/" + self.path
        return path

    def dependency_path(self, agent_version):
        """Return the versioned dependency path of the Go module
        >>> mods = [GoModule("."), GoModule("pkg/util/log")]
        >>> [mod.dependency_path("7.27.0") for mod in mods]
        ["github.com/DataDog/datadog-agent@v7.27.0", "github.com/DataDog/datadog-agent/pkg/util/log@v0.27.0"]
        """
        return f"{self.import_path}@{self.__version(agent_version)}"


AGENT_MODULE_PATH_PREFIX = "github.com/DataDog/datadog-agent/"

# Folder containing a `go.mod` file but that should not be added to the default modules list
IGNORED_MODULE_PATHS = [
    # Test files
    Path("./internal/tools/modparser/testdata/badformat"),
    Path("./internal/tools/modparser/testdata/match"),
    Path("./internal/tools/modparser/testdata/nomatch"),
    Path("./internal/tools/modparser/testdata/patchgoversion"),
    # This `go.mod` is a hack
    Path("./pkg/process/procutil/resources"),
    # We have test files in the tasks folder
    Path("./tasks"),
    # Test files
    Path("./test/integration/serverless/recorder-extension"),
    Path("./test/integration/serverless/src"),
]


@lru_cache
def get_default_modules(base_dir: Path | None = None) -> dict[str, GoModule]:
    """Load the default modules from all the module.yml files."""

    modules = {}

    for module_data_path in glob("./**/module.yml", recursive=True, root_dir=base_dir):
        module_data_dir = module_data_path.removesuffix("/module.yml")

        module = GoModule.from_file(module_data_dir, base_dir)
        modules[module.path] = module

    return modules
