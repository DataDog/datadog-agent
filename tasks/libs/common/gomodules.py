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


class GoModuleDumper(yaml.SafeDumper):
    """SafeDumper that ignores aliases. (no references for readability)"""

    def ignore_aliases(self, _):  # noqa
        return True


@dataclass
class GoModule:
    """
    A Go module abstraction.
    independent specifies whether this modules is supposed to exist independently of the datadog-agent module.
    If True, a check will run to ensure this is true.
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
        return GoModule(
            path=path,
            targets=data["targets"],
            lint_targets=data["lint_targets"],
            condition=data["condition"],
            should_tag=data["should_tag"],
            importable=data["importable"],
            independent=data["independent"],
            used_by_otel=data["used_by_otel"],
        )

    @staticmethod
    def from_file(dir_path: str | Path) -> GoModule:
        dir_path = dir_path if isinstance(dir_path, Path) else Path(dir_path)

        assert dir_path.is_dir(), f"Directory {dir_path} does not exist"

        with open(dir_path / 'module.yml') as file:
            data = yaml.safe_load(file)

            return GoModule.from_dict(dir_path.as_posix(), data)

    def __post_init__(self):
        self.targets = self.targets or ["."]
        self.lint_targets = self.lint_targets or self.targets

        self._dependencies = None

    def to_dict(self) -> dict[str, object]:
        return {
            "path": self.path,
            "targets": self.targets,
            "lint_targets": self.lint_targets,
            "condition": self.condition,
            "should_tag": self.should_tag,
            "importable": self.importable,
            "independent": self.independent,
            "used_by_otel": self.used_by_otel,
        }

    def to_file(self):
        dir_path = Path(self.path)

        assert dir_path.is_dir(), f"Directory {dir_path} does not exist"

        with open(dir_path / 'module.yml', "w") as file:
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

DEFAULT_MODULES = {
    ".": GoModule(
        ".",
        targets=["./pkg", "./cmd", "./comp"],
    ),
    "pkg/util/defaultpaths": GoModule("pkg/util/defaultpaths", independent=True, used_by_otel=True),
    "comp/api/api/def": GoModule("comp/api/api/def", independent=True, used_by_otel=True),
    "comp/api/authtoken": GoModule("comp/api/authtoken", independent=True, used_by_otel=True),
    "comp/core/config": GoModule("comp/core/config", independent=True, used_by_otel=True),
    "comp/core/flare/builder": GoModule("comp/core/flare/builder", independent=True, used_by_otel=True),
    "comp/core/flare/types": GoModule("comp/core/flare/types", independent=True, used_by_otel=True),
    "comp/core/hostname/hostnameinterface": GoModule(
        "comp/core/hostname/hostnameinterface", independent=True, used_by_otel=True
    ),
    "comp/core/log/def": GoModule("comp/core/log/def", independent=True, used_by_otel=True),
    "comp/core/log/impl": GoModule("comp/core/log/impl", independent=True, used_by_otel=True),
    "comp/core/log/impl-trace": GoModule("comp/core/log/impl-trace", independent=True),
    "comp/core/log/mock": GoModule("comp/core/log/mock", independent=True, used_by_otel=True),
    "comp/core/secrets": GoModule("comp/core/secrets", independent=True, used_by_otel=True),
    "comp/core/status": GoModule("comp/core/status", independent=True, used_by_otel=True),
    "comp/core/status/statusimpl": GoModule("comp/core/status/statusimpl", independent=True),
    "comp/core/tagger/types": GoModule("comp/core/tagger/types", independent=True, used_by_otel=True),
    "comp/core/tagger/utils": GoModule("comp/core/tagger/utils", independent=True, used_by_otel=True),
    "comp/core/telemetry": GoModule("comp/core/telemetry", independent=True, used_by_otel=True),
    "comp/def": GoModule("comp/def", independent=True, used_by_otel=True),
    "comp/forwarder/defaultforwarder": GoModule("comp/forwarder/defaultforwarder", independent=True, used_by_otel=True),
    "comp/forwarder/orchestrator/orchestratorinterface": GoModule(
        "comp/forwarder/orchestrator/orchestratorinterface", independent=True, used_by_otel=True
    ),
    "comp/logs/agent/config": GoModule("comp/logs/agent/config", independent=True, used_by_otel=True),
    "comp/netflow/payload": GoModule("comp/netflow/payload", independent=True),
    "comp/otelcol/collector-contrib/def": GoModule(
        "comp/otelcol/collector-contrib/def", independent=True, used_by_otel=True
    ),
    "comp/otelcol/collector-contrib/impl": GoModule(
        "comp/otelcol/collector-contrib/impl", independent=True, used_by_otel=True
    ),
    "comp/otelcol/converter/def": GoModule("comp/otelcol/converter/def", independent=True, used_by_otel=True),
    "comp/otelcol/converter/impl": GoModule("comp/otelcol/converter/impl", independent=True, used_by_otel=True),
    "comp/otelcol/ddflareextension/def": GoModule(
        "comp/otelcol/ddflareextension/def", independent=True, used_by_otel=True
    ),
    "comp/otelcol/ddflareextension/impl": GoModule(
        "comp/otelcol/ddflareextension/impl", independent=True, used_by_otel=True
    ),
    "comp/otelcol/logsagentpipeline": GoModule("comp/otelcol/logsagentpipeline", independent=True, used_by_otel=True),
    "comp/otelcol/logsagentpipeline/logsagentpipelineimpl": GoModule(
        "comp/otelcol/logsagentpipeline/logsagentpipelineimpl", independent=True, used_by_otel=True
    ),
    "comp/otelcol/otlp/components/exporter/datadogexporter": GoModule(
        "comp/otelcol/otlp/components/exporter/datadogexporter", independent=True, used_by_otel=True
    ),
    "comp/otelcol/otlp/components/exporter/logsagentexporter": GoModule(
        "comp/otelcol/otlp/components/exporter/logsagentexporter", independent=True, used_by_otel=True
    ),
    "comp/otelcol/otlp/components/exporter/serializerexporter": GoModule(
        "comp/otelcol/otlp/components/exporter/serializerexporter", independent=True, used_by_otel=True
    ),
    "comp/otelcol/otlp/components/metricsclient": GoModule(
        "comp/otelcol/otlp/components/metricsclient", independent=True, used_by_otel=True
    ),
    "comp/otelcol/otlp/components/processor/infraattributesprocessor": GoModule(
        "comp/otelcol/otlp/components/processor/infraattributesprocessor", independent=True, used_by_otel=True
    ),
    "comp/otelcol/otlp/components/statsprocessor": GoModule(
        "comp/otelcol/otlp/components/statsprocessor", independent=True, used_by_otel=True
    ),
    "comp/otelcol/otlp/testutil": GoModule("comp/otelcol/otlp/testutil", independent=True, used_by_otel=True),
    "comp/serializer/compression": GoModule("comp/serializer/compression", independent=True, used_by_otel=True),
    "comp/trace/agent/def": GoModule("comp/trace/agent/def", independent=True, used_by_otel=True),
    "comp/trace/compression/def": GoModule("comp/trace/compression/def", independent=True, used_by_otel=True),
    "comp/trace/compression/impl-gzip": GoModule(
        "comp/trace/compression/impl-gzip", independent=True, used_by_otel=True
    ),
    "comp/trace/compression/impl-zstd": GoModule(
        "comp/trace/compression/impl-zstd", independent=True, used_by_otel=True
    ),
    "internal/tools": GoModule("internal/tools", condition='never', should_tag=False),
    "internal/tools/independent-lint": GoModule("internal/tools/independent-lint", condition='never', should_tag=False),
    "internal/tools/modformatter": GoModule("internal/tools/modformatter", condition='never', should_tag=False),
    "internal/tools/modparser": GoModule("internal/tools/modparser", condition='never', should_tag=False),
    "internal/tools/proto": GoModule("internal/tools/proto", condition='never', should_tag=False),
    "pkg/aggregator/ckey": GoModule("pkg/aggregator/ckey", independent=True, used_by_otel=True),
    "pkg/api": GoModule("pkg/api", independent=True, used_by_otel=True),
    "pkg/collector/check/defaults": GoModule("pkg/collector/check/defaults", independent=True, used_by_otel=True),
    "pkg/config/env": GoModule("pkg/config/env", independent=True, used_by_otel=True),
    "pkg/config/mock": GoModule("pkg/config/mock", independent=True, used_by_otel=True),
    "pkg/config/nodetreemodel": GoModule("pkg/config/nodetreemodel", independent=True, used_by_otel=True),
    "pkg/config/model": GoModule("pkg/config/model", independent=True, used_by_otel=True),
    "pkg/config/remote": GoModule("pkg/config/remote", independent=True),
    "pkg/config/setup": GoModule("pkg/config/setup", independent=True, used_by_otel=True),
    "pkg/config/teeconfig": GoModule("pkg/config/teeconfig", independent=True, used_by_otel=True),
    "pkg/config/structure": GoModule("pkg/config/structure", independent=True, used_by_otel=True),
    "pkg/config/utils": GoModule("pkg/config/utils", independent=True, used_by_otel=True),
    "pkg/errors": GoModule("pkg/errors", independent=True),
    "pkg/gohai": GoModule("pkg/gohai", independent=True, importable=False),
    "pkg/linters/components/pkgconfigusage": GoModule("pkg/linters/components/pkgconfigusage", should_tag=False),
    "pkg/logs/auditor": GoModule("pkg/logs/auditor", independent=True, used_by_otel=True),
    "pkg/logs/client": GoModule("pkg/logs/client", independent=True, used_by_otel=True),
    "pkg/logs/diagnostic": GoModule("pkg/logs/diagnostic", independent=True, used_by_otel=True),
    "pkg/logs/message": GoModule("pkg/logs/message", independent=True, used_by_otel=True),
    "pkg/logs/metrics": GoModule("pkg/logs/metrics", independent=True, used_by_otel=True),
    "pkg/logs/pipeline": GoModule("pkg/logs/pipeline", independent=True, used_by_otel=True),
    "pkg/logs/processor": GoModule("pkg/logs/processor", independent=True, used_by_otel=True),
    "pkg/logs/sds": GoModule("pkg/logs/sds", independent=True, used_by_otel=True),
    "pkg/logs/sender": GoModule("pkg/logs/sender", independent=True, used_by_otel=True),
    "pkg/logs/sources": GoModule("pkg/logs/sources", independent=True, used_by_otel=True),
    "pkg/logs/status/statusinterface": GoModule("pkg/logs/status/statusinterface", independent=True, used_by_otel=True),
    "pkg/logs/status/utils": GoModule("pkg/logs/status/utils", independent=True, used_by_otel=True),
    "pkg/logs/util/testutils": GoModule("pkg/logs/util/testutils", independent=True, used_by_otel=True),
    "pkg/metrics": GoModule("pkg/metrics", independent=True, used_by_otel=True),
    "pkg/networkdevice/profile": GoModule("pkg/networkdevice/profile", independent=True),
    "pkg/obfuscate": GoModule("pkg/obfuscate", independent=True, used_by_otel=True),
    "pkg/orchestrator/model": GoModule("pkg/orchestrator/model", independent=True, used_by_otel=True),
    "pkg/process/util/api": GoModule("pkg/process/util/api", independent=True, used_by_otel=True),
    "pkg/proto": GoModule("pkg/proto", independent=True, used_by_otel=True),
    "pkg/remoteconfig/state": GoModule("pkg/remoteconfig/state", independent=True, used_by_otel=True),
    "pkg/security/secl": GoModule("pkg/security/secl", independent=True),
    "pkg/security/seclwin": GoModule("pkg/security/seclwin", independent=True, condition='never'),
    "pkg/serializer": GoModule("pkg/serializer", independent=True, used_by_otel=True),
    "pkg/status/health": GoModule("pkg/status/health", independent=True, used_by_otel=True),
    "pkg/tagger/types": GoModule("pkg/tagger/types", independent=True, used_by_otel=True),
    "pkg/tagset": GoModule("pkg/tagset", independent=True, used_by_otel=True),
    "pkg/telemetry": GoModule("pkg/telemetry", independent=True, used_by_otel=True),
    "pkg/trace": GoModule("pkg/trace", independent=True, used_by_otel=True),
    "pkg/trace/stats/oteltest": GoModule("pkg/trace/stats/oteltest", independent=True, used_by_otel=True),
    "pkg/util/backoff": GoModule("pkg/util/backoff", independent=True, used_by_otel=True),
    "pkg/util/buf": GoModule("pkg/util/buf", independent=True, used_by_otel=True),
    "pkg/util/cache": GoModule("pkg/util/cache", independent=True),
    "pkg/util/cgroups": GoModule("pkg/util/cgroups", independent=True, condition='is_linux', used_by_otel=True),
    "pkg/util/common": GoModule("pkg/util/common", independent=True, used_by_otel=True),
    "pkg/util/containers/image": GoModule("pkg/util/containers/image", independent=True, used_by_otel=True),
    "pkg/util/executable": GoModule("pkg/util/executable", independent=True, used_by_otel=True),
    "pkg/util/filesystem": GoModule("pkg/util/filesystem", independent=True, used_by_otel=True),
    "pkg/util/flavor": GoModule("pkg/util/flavor", independent=True),
    "pkg/util/fxutil": GoModule("pkg/util/fxutil", independent=True, used_by_otel=True),
    "pkg/util/grpc": GoModule("pkg/util/grpc", independent=True),
    "pkg/util/hostname/validate": GoModule("pkg/util/hostname/validate", independent=True, used_by_otel=True),
    "pkg/util/http": GoModule("pkg/util/http", independent=True, used_by_otel=True),
    "pkg/util/json": GoModule("pkg/util/json", independent=True, used_by_otel=True),
    "pkg/util/log": GoModule("pkg/util/log", independent=True, used_by_otel=True),
    "pkg/util/log/setup": GoModule("pkg/util/log/setup", independent=True, used_by_otel=True),
    "pkg/util/optional": GoModule("pkg/util/optional", independent=True, used_by_otel=True),
    "pkg/util/pointer": GoModule("pkg/util/pointer", independent=True, used_by_otel=True),
    "pkg/util/scrubber": GoModule("pkg/util/scrubber", independent=True, used_by_otel=True),
    "pkg/util/sort": GoModule("pkg/util/sort", independent=True, used_by_otel=True),
    "pkg/util/startstop": GoModule("pkg/util/startstop", independent=True, used_by_otel=True),
    "pkg/util/statstracker": GoModule("pkg/util/statstracker", independent=True, used_by_otel=True),
    "pkg/util/system": GoModule("pkg/util/system", independent=True, used_by_otel=True),
    "pkg/util/system/socket": GoModule("pkg/util/system/socket", independent=True, used_by_otel=True),
    "pkg/util/testutil": GoModule("pkg/util/testutil", independent=True, used_by_otel=True),
    "pkg/util/uuid": GoModule("pkg/util/uuid", independent=True),
    "pkg/util/winutil": GoModule("pkg/util/winutil", independent=True, used_by_otel=True),
    "pkg/version": GoModule("pkg/version", independent=True, used_by_otel=True),
    "test/fakeintake": GoModule("test/fakeintake", independent=True),
    "test/new-e2e": GoModule(
        "test/new-e2e",
        independent=True,
        targets=["./pkg/runner", "./pkg/utils/e2e/client"],
        lint_targets=[".", "./examples"],  # need to explicitly list "examples", otherwise it is skipped
    ),
    "test/otel": GoModule("test/otel", independent=True, used_by_otel=True),
    "tools/retry_file_dump": GoModule("tools/retry_file_dump", condition='never', should_tag=False),
}

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
def get_default_modules():
    return DEFAULT_MODULES
