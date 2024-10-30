import json
import os
import subprocess
import sys
from collections import defaultdict
from contextlib import contextmanager
from pathlib import Path

from invoke import Context, Exit, task

from tasks.libs.common.color import Color, color_message

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
    "internal/tools": GoModule("internal/tools", condition=lambda: False, should_tag=False),
    "internal/tools/independent-lint": GoModule(
        "internal/tools/independent-lint", condition=lambda: False, should_tag=False
    ),
    "internal/tools/modformatter": GoModule("internal/tools/modformatter", condition=lambda: False, should_tag=False),
    "internal/tools/modparser": GoModule("internal/tools/modparser", condition=lambda: False, should_tag=False),
    "internal/tools/proto": GoModule("internal/tools/proto", condition=lambda: False, should_tag=False),
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
    "pkg/security/seclwin": GoModule("pkg/security/seclwin", independent=True, condition=lambda: False),
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
    "pkg/util/cgroups": GoModule(
        "pkg/util/cgroups", independent=True, condition=lambda: sys.platform == "linux", used_by_otel=True
    ),
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
    "tools/retry_file_dump": GoModule("tools/retry_file_dump", condition=lambda: False, should_tag=False),
}

# Folder containing a `go.mod` file but that should not be added to the DEFAULT_MODULES
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
                    # todo: remove once datadogconnector fix is released.
                    if mod.import_path == "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl":
                        ctx.run(
                            "go mod edit -replace github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector=github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector@v0.103.0"
                        )
                    if (
                        mod.import_path == "github.com/DataDog/datadog-agent/comp/otelcol/configstore/impl"
                        or mod.import_path == "github.com/DataDog/datadog-agent/comp/otelcol/configstore/def"
                    ):
                        ctx.run("go mod edit -exclude github.com/knadh/koanf/maps@v0.1.1")
                        ctx.run("go mod edit -exclude github.com/knadh/koanf/providers/confmap@v0.1.0")
                        ctx.run("go mod edit -exclude github.com/knadh/koanf/providers/confmap@v0.1.0-dev0")
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
def for_each(
    ctx: Context,
    cmd: str,
    skip_untagged: bool = False,
    ignore_errors: bool = False,
    use_targets_path: bool = False,
    use_lint_targets_path: bool = False,
    skip_condition: bool = False,
):
    """
    Run the given command in the directory of each module.
    """
    assert not (
        use_targets_path and use_lint_targets_path
    ), "Only one of use_targets_path and use_lint_targets_path can be set"

    for mod in DEFAULT_MODULES.values():
        if skip_untagged and not mod.should_tag:
            continue
        if skip_condition and not mod.condition():
            continue

        targets = [mod.full_path()]
        if use_targets_path:
            targets = [os.path.join(mod.full_path(), target) for target in mod.targets]
        if use_lint_targets_path:
            targets = [os.path.join(mod.full_path(), target) for target in mod.lint_targets]

        for target in targets:
            with ctx.cd(target):
                res = ctx.run(cmd, warn=True)
                assert res is not None
                if res.failed and not ignore_errors:
                    raise Exit(f"Command failed in {target}")


@task
def validate(_: Context):
    """
    Test if every module was properly added in the DEFAULT_MODULES list.
    """
    missing_modules: list[str] = []
    default_modules_paths = {Path(p) for p in DEFAULT_MODULES}

    # Find all go.mod files and make sure they are registered in DEFAULT_MODULES
    for root, dirs, files in os.walk("."):
        dirs[:] = [d for d in dirs if Path(root) / d not in IGNORED_MODULE_PATHS]

        if "go.mod" in files and Path(root) not in default_modules_paths:
            missing_modules.append(root)

    if missing_modules:
        message = f"{color_message('ERROR', Color.RED)}: some modules are missing from DEFAULT_MODULES\n"
        for module in missing_modules:
            message += f"  {module} is missing from DEFAULT_MODULES\n"

        message += "Please add them to the DEFAULT_MODULES list or exclude them from the validation."

        raise Exit(message)


@task
def validate_used_by_otel(ctx: Context):
    """
    Verify whether indirect local dependencies of modules labeled "used_by_otel" are also marked with the "used_by_otel" tag.
    """
    otel_mods = [path for path, module in DEFAULT_MODULES.items() if module.used_by_otel]
    missing_used_by_otel_label: dict[str, list[str]] = defaultdict(list)

    # for every module labeled as "used_by_otel"
    for otel_mod in otel_mods:
        gomod_path = f"{otel_mod}/go.mod"
        # get the go.mod data
        result = ctx.run(f"go mod edit -json {gomod_path}", hide='both')
        if result.failed:
            raise Exit(f"Error running go mod edit -json on {gomod_path}: {result.stderr}")

        go_mod_json = json.loads(result.stdout)
        # get module dependencies
        reqs = go_mod_json.get("Require", [])
        if not reqs:  # Module don't have dependencies, continue
            continue
        for require in reqs:
            # we are only interested into local modules
            if not require["Path"].startswith("github.com/DataDog/datadog-agent/"):
                continue
            # we need the relative path of module (without github.com/DataDog/datadog-agent/ prefix)
            rel_path = require['Path'].removeprefix("github.com/DataDog/datadog-agent/")
            # check if indirect module is labeled as "used_by_otel"
            if rel_path not in DEFAULT_MODULES or not DEFAULT_MODULES[rel_path].used_by_otel:
                missing_used_by_otel_label[rel_path].append(otel_mod)
    if missing_used_by_otel_label:
        message = f"{color_message('ERROR', Color.RED)}: some indirect local dependencies of modules labeled \"used_by_otel\" are not correctly labeled in DEFAULT_MODULES\n"
        for k, v in missing_used_by_otel_label.items():
            message += f"\t{color_message(k, Color.RED)} is missing (used by {v})\n"
        message += "Please label them as \"used_by_otel\" in the DEFAULT_MODULES list."

        raise Exit(message)


def get_module_by_path(path: Path) -> GoModule | None:
    """
    Return the GoModule object corresponding to the given path.
    """
    for module in DEFAULT_MODULES.values():
        if Path(module.path) == path:
            return module

    return None
