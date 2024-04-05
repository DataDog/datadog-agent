import os
import re
import subprocess
import sys
from contextlib import contextmanager

from invoke import Context, task

from tasks.libs.common.color import color_message

FORBIDDEN_CODECOV_FLAG_CHARS = re.compile(r'[^\w\.\-]')
AGENT_MODULE_PATH_PREFIX = "github.com/DataDog/datadog-agent/"


class GoModule:
    """
    A Go module abstraction.
    independent specifies whether this modules is supposed to exist independently of the datadog-agent module.
    If True, a check will run to ensure this is true.
    """

    def __init__(
        self,
        path,
        targets=None,
        condition=lambda: True,
        should_tag=True,
        importable=True,
        independent=False,
        lint_targets=None,
        used_by_otel=False,
        legacy_go_mod_version=False,
    ):
        self.path = path
        self.targets = targets if targets else ["."]
        self.lint_targets = lint_targets if lint_targets else self.targets
        self.condition = condition
        self.should_tag = should_tag
        # HACK: Workaround for modules that can be tested, but not imported (eg. gohai), because
        # they define a main package
        # A better solution would be to automatically detect if a module contains a main package,
        # at the cost of spending some time parsing the module.
        self.importable = importable
        self.independent = independent
        self.used_by_otel = used_by_otel
        self.legacy_go_mod_version = legacy_go_mod_version

        self._dependencies = None

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

    def codecov_path(self):
        """Return the path of the Go module, normalized to satisfy Codecov
        restrictions on flags.
        https://docs.codecov.com/docs/flags
        """
        if self.path == ".":
            return "main"

        return re.sub(FORBIDDEN_CODECOV_FLAG_CHARS, '_', self.path)

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


# Default Modules on which will run tests / linters. When `condition=lambda: False` is defined for a module, it will be skipped.
DEFAULT_MODULES = {
    ".": GoModule(
        ".",
        targets=["./pkg", "./cmd", "./comp"],
    ),
    "internal/tools": GoModule("internal/tools", condition=lambda: False, should_tag=False),
    "internal/tools/proto": GoModule("internal/tools/proto", condition=lambda: False, should_tag=False),
    "internal/tools/modparser": GoModule("internal/tools/modparser", condition=lambda: False, should_tag=False),
    "internal/tools/independent-lint": GoModule(
        "internal/tools/independent-lint", condition=lambda: False, should_tag=False
    ),
    "internal/tools/modformatter": GoModule("internal/tools/modformatter", condition=lambda: False, should_tag=False),
    "test/e2e/containers/otlp_sender": GoModule(
        "test/e2e/containers/otlp_sender", condition=lambda: False, should_tag=False
    ),
    "test/new-e2e": GoModule(
        "test/new-e2e",
        independent=True,
        targets=["./pkg/runner", "./pkg/utils/e2e/client"],
        lint_targets=["."],
    ),
    "test/fakeintake": GoModule("test/fakeintake", independent=True),
    "pkg/aggregator/ckey": GoModule("pkg/aggregator/ckey", independent=True),
    "pkg/errors": GoModule("pkg/errors", independent=True),
    "pkg/obfuscate": GoModule("pkg/obfuscate", independent=True, used_by_otel=True),
    "pkg/gohai": GoModule("pkg/gohai", independent=True, importable=False),
    "pkg/proto": GoModule("pkg/proto", independent=True, used_by_otel=True),
    "pkg/trace": GoModule("pkg/trace", independent=True, used_by_otel=True),
    "pkg/tagger/types": GoModule("pkg/tagger/types", independent=True),
    "pkg/tagset": GoModule("pkg/tagset", independent=True),
    "pkg/metrics": GoModule("pkg/metrics", independent=True),
    "pkg/telemetry": GoModule("pkg/telemetry", independent=True),
    "comp/core/flare/types": GoModule("comp/core/flare/types", independent=True),
    "comp/core/hostname/hostnameinterface": GoModule("comp/core/hostname/hostnameinterface", independent=True),
    "comp/core/config": GoModule("comp/core/config", independent=True),
    "comp/core/log": GoModule("comp/core/log", independent=True),
    "comp/core/secrets": GoModule("comp/core/secrets", independent=True),
    "comp/core/status": GoModule("comp/core/status", independent=True),
    "comp/core/status/statusimpl": GoModule("comp/core/status/statusimpl", independent=True),
    "comp/serializer/compression": GoModule("comp/serializer/compression", independent=True),
    "comp/core/telemetry": GoModule("comp/core/telemetry", independent=True),
    "comp/forwarder/defaultforwarder": GoModule("comp/forwarder/defaultforwarder", independent=True),
    "comp/forwarder/orchestrator/orchestratorinterface": GoModule(
        "comp/forwarder/orchestrator/orchestratorinterface", independent=True
    ),
    "comp/otelcol/otlp/components/exporter/serializerexporter": GoModule(
        "comp/otelcol/otlp/components/exporter/serializerexporter", independent=True
    ),
    "comp/otelcol/otlp/components/exporter/logsagentexporter": GoModule(
        "comp/otelcol/otlp/components/exporter/logsagentexporter", independent=True
    ),
    "comp/otelcol/otlp/testutil": GoModule("comp/otelcol/otlp/testutil", independent=True),
    "comp/logs/agent/config": GoModule("comp/logs/agent/config", independent=True),
    "comp/netflow/payload": GoModule("comp/netflow/payload", independent=True),
    "cmd/agent/common/path": GoModule("cmd/agent/common/path", independent=True),
    "pkg/api": GoModule("pkg/api", independent=True),
    "pkg/config/model": GoModule("pkg/config/model", independent=True),
    "pkg/config/env": GoModule("pkg/config/env", independent=True),
    "pkg/config/setup": GoModule("pkg/config/setup", independent=True),
    "pkg/config/utils": GoModule("pkg/config/utils", independent=True),
    "pkg/config/logs": GoModule("pkg/config/logs", independent=True),
    "pkg/config/remote": GoModule("pkg/config/remote", independent=True),
    "pkg/logs/auditor": GoModule("pkg/logs/auditor", independent=True),
    "pkg/logs/client": GoModule("pkg/logs/client", independent=True),
    "pkg/logs/diagnostic": GoModule("pkg/logs/diagnostic", independent=True),
    "pkg/logs/processor": GoModule("pkg/logs/processor", independent=True),
    "pkg/logs/util/testutils": GoModule("pkg/logs/util/testutils", independent=True),
    "pkg/logs/message": GoModule("pkg/logs/message", independent=True),
    "pkg/logs/metrics": GoModule("pkg/logs/metrics", independent=True),
    "pkg/logs/pipeline": GoModule("pkg/logs/pipeline", independent=True),
    "pkg/logs/sender": GoModule("pkg/logs/sender", independent=True),
    "pkg/logs/sources": GoModule("pkg/logs/sources", independent=True),
    "pkg/logs/status/statusinterface": GoModule("pkg/logs/status/statusinterface", independent=True),
    "pkg/logs/status/utils": GoModule("pkg/logs/status/utils", independent=True),
    "pkg/serializer": GoModule("pkg/serializer", independent=True),
    "pkg/security/secl": GoModule("pkg/security/secl", independent=True, legacy_go_mod_version=True),
    "pkg/security/seclwin": GoModule(
        "pkg/security/seclwin", independent=True, condition=lambda: False, legacy_go_mod_version=True
    ),
    "pkg/status/health": GoModule("pkg/status/health", independent=True),
    "pkg/remoteconfig/state": GoModule("pkg/remoteconfig/state", independent=True, used_by_otel=True),
    "pkg/util/cgroups": GoModule(
        "pkg/util/cgroups", independent=True, condition=lambda: sys.platform == "linux", used_by_otel=True
    ),
    "pkg/util/http": GoModule("pkg/util/http", independent=True),
    "pkg/util/log": GoModule("pkg/util/log", independent=True, used_by_otel=True),
    "pkg/util/pointer": GoModule("pkg/util/pointer", independent=True, used_by_otel=True),
    "pkg/util/scrubber": GoModule("pkg/util/scrubber", independent=True, used_by_otel=True),
    "pkg/util/startstop": GoModule("pkg/util/startstop", independent=True),
    "pkg/util/backoff": GoModule("pkg/util/backoff", independent=True),
    "pkg/util/cache": GoModule("pkg/util/cache", independent=True),
    "pkg/util/common": GoModule("pkg/util/common", independent=True),
    "pkg/util/executable": GoModule("pkg/util/executable", independent=True),
    "pkg/util/flavor": GoModule("pkg/util/flavor", independent=True),
    "pkg/util/filesystem": GoModule("pkg/util/filesystem", independent=True),
    "pkg/util/fxutil": GoModule("pkg/util/fxutil", independent=True),
    "pkg/util/buf": GoModule("pkg/util/buf", independent=True),
    "pkg/util/hostname/validate": GoModule("pkg/util/hostname/validate", independent=True),
    "pkg/util/json": GoModule("pkg/util/json", independent=True),
    "pkg/util/sort": GoModule("pkg/util/sort", independent=True),
    "pkg/util/optional": GoModule("pkg/util/optional", independent=True),
    "pkg/util/statstracker": GoModule("pkg/util/statstracker", independent=True),
    "pkg/util/system": GoModule("pkg/util/system", independent=True),
    "pkg/util/system/socket": GoModule("pkg/util/system/socket", independent=True),
    "pkg/util/testutil": GoModule("pkg/util/testutil", independent=True),
    "pkg/util/uuid": GoModule("pkg/util/uuid", independent=True),
    "pkg/util/winutil": GoModule("pkg/util/winutil", independent=True),
    "pkg/util/grpc": GoModule("pkg/util/grpc", independent=True),
    "pkg/version": GoModule("pkg/version", independent=True),
    "pkg/networkdevice/profile": GoModule("pkg/networkdevice/profile", independent=True),
    "pkg/collector/check/defaults": GoModule("pkg/collector/check/defaults", independent=True),
    "pkg/orchestrator/model": GoModule("pkg/orchestrator/model", independent=True),
    "pkg/process/util/api": GoModule("pkg/process/util/api", independent=True),
}

MAIN_TEMPLATE = """package main

import (
{imports}
)

func main() {{}}
"""

PACKAGE_TEMPLATE = '	_ "{}"'


@contextmanager
def generate_dummy_package(ctx, folder):
    """
    Return a generator-iterator when called.
    Allows us to wrap this function with a "with" statement to delete the created dummy pacakage afterwards.
    """
    try:
        import_paths = []
        for mod in DEFAULT_MODULES.values():
            if mod.path != "." and mod.condition() and mod.importable:
                import_paths.append(mod.import_path)

        os.mkdir(folder)
        with ctx.cd(folder):
            print("Creating dummy 'main.go' file... ", end="")
            with open(os.path.join(ctx.cwd, 'main.go'), 'w') as main_file:
                main_file.write(
                    MAIN_TEMPLATE.format(imports="\n".join(PACKAGE_TEMPLATE.format(path) for path in import_paths))
                )
            print("Done")

            ctx.run("go mod init example.com/testmodule")
            for mod in DEFAULT_MODULES.values():
                if mod.path != ".":
                    ctx.run(f"go mod edit -require={mod.dependency_path('0.0.0')}")
                    ctx.run(f"go mod edit -replace {mod.import_path}=../{mod.path}")

        # yield folder waiting for a "with" block to be executed (https://docs.python.org/3/library/contextlib.html)
        yield folder

    # the generator is then resumed here after the "with" block is exited
    finally:
        # delete test_folder to avoid FileExistsError while running this task again
        ctx.run(f"rm -rf ./{folder}")


@task
def go_work(_: Context):
    """
    Create a go.work file using the module list contained in DEFAULT_MODULES
    and the go version contained in the file .go-version.
    If there is already a go.work file, it is renamed go.work.backup and a warning is printed.
    """
    print(
        color_message(
            "WARNING: Using a go.work file is not supported and can cause weird errors "
            "when compiling the agent or running tests.\n"
            "Remember to export GOWORK=off to avoid these issues.\n",
            "orange",
        ),
        file=sys.stderr,
    )

    # read go version from the .go-version file, removing the bugfix part of the version

    with open(".go-version") as f:
        go_version = f.read().strip()

    if os.path.exists("go.work"):
        print("go.work already exists. Renaming to go.work.backup")
        os.rename("go.work", "go.work.backup")

    with open("go.work", "w") as f:
        f.write(f"go {go_version}\n\nuse (\n")
        for mod in DEFAULT_MODULES.values():
            prefix = "" if mod.condition() else "//"
            f.write(f"\t{prefix}{mod.path}\n")
        f.write(")\n")


@task
def for_each(ctx: Context, cmd: str, skip_untagged: bool = False):
    """
    Run the given command in the directory of each module.
    """
    for mod in DEFAULT_MODULES.values():
        if skip_untagged and not mod.should_tag:
            continue
        with ctx.cd(mod.full_path()):
            ctx.run(cmd)
