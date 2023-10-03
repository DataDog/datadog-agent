import os
import re
import subprocess
import sys
from contextlib import contextmanager

FORBIDDEN_CODECOV_FLAG_CHARS = re.compile(r'[^\w\.\-]')


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
        prefix = "github.com/DataDog/datadog-agent/"
        base_path = os.getcwd()
        mod_parser_path = os.path.join(base_path, "internal", "tools", "modparser")

        if not os.path.isdir(mod_parser_path):
            raise Exception(f"Cannot find go.mod parser in {mod_parser_path}")

        try:
            output = subprocess.check_output(
                ["go", "run", ".", "-path", os.path.join(base_path, self.path), "-prefix", prefix],
                cwd=mod_parser_path,
            ).decode("utf-8")
        except subprocess.CalledProcessError as e:
            print(f"Error while calling go.mod parser: {e.output}")
            raise e

        # Remove github.com/DataDog/datadog-agent/ from each line
        return [line[len(prefix) :] for line in output.strip().splitlines()]

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
        path = "github.com/DataDog/datadog-agent"
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


DEFAULT_MODULES = {
    ".": GoModule(
        ".",
        targets=["./pkg", "./cmd", "./comp"],
    ),
    "internal/tools": GoModule("internal/tools", condition=lambda: False, should_tag=False),
    "internal/tools/proto": GoModule("internal/tools/proto", condition=lambda: False, should_tag=False),
    "internal/tools/modparser": GoModule("internal/tools/modparser", condition=lambda: False, should_tag=False),
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
    "pkg/obfuscate": GoModule("pkg/obfuscate", independent=True),
    "pkg/gohai": GoModule("pkg/gohai", independent=True, importable=False),
    "pkg/proto": GoModule("pkg/proto", independent=True),
    "pkg/trace": GoModule("pkg/trace", independent=True),
    "pkg/security/secl": GoModule("pkg/security/secl", independent=True),
    "pkg/remoteconfig/state": GoModule("pkg/remoteconfig/state", independent=True),
    "pkg/util/cgroups": GoModule("pkg/util/cgroups", independent=True, condition=lambda: sys.platform == "linux"),
    "pkg/util/log": GoModule("pkg/util/log", independent=True),
    "pkg/util/pointer": GoModule("pkg/util/pointer", independent=True),
    "pkg/util/scrubber": GoModule("pkg/util/scrubber", independent=True),
    "pkg/networkdevice/profile": GoModule("pkg/networkdevice/profile", independent=True),
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
