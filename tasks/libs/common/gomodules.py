"""Provides functions to import / export go modules from / to yaml files."""

from __future__ import annotations

import os
import subprocess
import sys
from collections.abc import Callable
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path
from typing import ClassVar

import yaml

import tasks


class ConfigDumper(yaml.SafeDumper):
    """SafeDumper that ignores aliases. (no references for readability)"""

    def ignore_aliases(self, _):  # noqa
        return True


@dataclass
class Configuration:
    """Represents the top level configuration of the modules."""

    FILE_NAME: ClassVar[str] = 'modules.yml'
    INFO_COMMENT: ClassVar[str] = """
# This file contains the configuration of the modules.
# See {file} for more information.
"""

    # Where this file has been loaded from
    base_dir: Path
    # All GoModule to be taken into account (module.path: module)
    modules: dict[str, GoModule]
    # Name of each ignored module (not within `modules`)
    ignored_modules: set[str]

    @staticmethod
    def from_dict(data: dict[str, dict[str, object]], base_dir: Path | None = None) -> Configuration:
        base_dir = base_dir or Path.cwd()

        modules = {}
        ignored_modules = set()

        for name, module_data in data.get('modules', {}).items():
            if module_data == 'ignored':
                ignored_modules.add(name)
            elif module_data == 'default':
                modules[name] = GoModule.from_dict(name, {})
            else:
                modules[name] = GoModule.from_dict(name, module_data)

        return Configuration(base_dir, modules, ignored_modules)

    @classmethod
    def from_file(cls, base_dir: Path | None = None) -> Configuration:
        """Load the configuration from a yaml file."""

        base_dir = base_dir or Path.cwd()

        with open(base_dir / cls.FILE_NAME) as file:
            data = yaml.safe_load(file)

        return Configuration.from_dict(data)

    def to_dict(self) -> dict[str, object]:
        modules_config = {}
        modules_config.update(
            {name: module.to_dict(remove_path=True) or 'default' for name, module in self.modules.items()}
        )
        modules_config.update({module: 'ignored' for module in self.ignored_modules})

        return {
            'modules': modules_config,
        }

    def to_file(self):
        """Save the configuration to a yaml file at <base_dir/FILE_NAME>."""

        with open(self.base_dir / self.FILE_NAME, "w") as file:
            path = f'tasks/{Path(__file__).relative_to(Path(tasks.__file__).parent).as_posix()}'
            print(self.INFO_COMMENT.format(file=path).strip() + '\n', file=file)

            yaml.dump(self.to_dict(), file, Dumper=ConfigDumper)


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
        > independent: true
        > lint_targets:
        > - .
        > should_tag: true
        > targets:
        > - .
        > used_by_otel: false

        If a module should be ignored and not included within get_default_modules(), add the following to the module.yml:
        > ignored: true
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
    independent: bool = True
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
    def get_default_attributes() -> dict[str, object]:
        attrs = GoModule('.').to_dict(remove_defaults=False)
        attrs.pop('path')

        return attrs

    def __post_init__(self):
        self.targets = self.targets or ["."]
        self.lint_targets = self.lint_targets or self.targets

        self._dependencies = None

    def to_dict(self, remove_defaults=True, remove_path=False) -> dict[str, object]:
        """Convert to dictionary.

        Args:
            remove_defaults: Remove default values from the dictionary.
            remove_path: Remove the path from the dictionary.
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

        if remove_path:
            del attrs['path']

        if remove_defaults:
            default_attrs = GoModule.get_default_attributes()

            for key, value in default_attrs.items():
                if key in attrs and attrs[key] == value:
                    del attrs[key]

        return attrs

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
        return str(self.path)

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


@lru_cache
def get_default_modules(base_dir: Path | None = None) -> dict[str, GoModule]:
    """Load the default modules from all the module.yml files.

    All the module.yml files are supposed to be next to a go.mod file.
    If no module.yml file is found, a module is still created with default values except if the file contains `ignored: true`.

    Args:
        base_dir: Root directory of the agent repository ('.' by default).
    """

    return Configuration.from_file(base_dir).modules
