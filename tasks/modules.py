import os
import sys

from invoke import task


class GoModule:
    """A Go module abstraction."""

    def __init__(self, path, targets=None, condition=lambda: True, dependencies=None, should_tag=True):
        self.path = path
        self.targets = targets if targets else ["."]
        self.dependencies = dependencies if dependencies else []
        self.condition = condition
        self.should_tag = should_tag

    def __version(self, agent_version):
        """Return the module version for a given Agent version.
        >>> mods = [GoModule("."), GoModule("pkg/util/log")]
        >>> [mod.__version("7.27.0") for mod in mods]
        ["v7.27.0", "v0.27.0"]
        """
        if self.path == ".":
            return "v" + agent_version

        return "v0" + agent_version[1:]

    # FIXME: Change when Agent 6 and Agent 7 releases are decoupled
    def tag(self, agent_version):
        """Return the module tag name for a given Agent version.
        >>> mods = [GoModule("."), GoModule("pkg/util/log")]
        >>> [mod.tag("7.27.0") for mod in mods]
        [["6.27.0", "7.27.0"], ["pkg/util/log/v0.27.0"]]
        """
        if self.path == ".":
            return ["6" + agent_version[1:], "7" + agent_version[1:]]

        return ["{}/{}".format(self.path, self.__version(agent_version))]

    def full_path(self):
        """Return the absolute path of the Go module."""
        return os.path.abspath(self.path)

    def go_mod_path(self):
        """Return the absolute path of the Go module go.mod file."""
        return self.full_path() + "/go.mod"

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
        return "{import_path}@{version}".format(import_path=self.import_path, version=self.__version(agent_version))


DEFAULT_MODULES = {
    ".": GoModule(".", targets=["./pkg", "./cmd"], dependencies=["pkg/util/log", "pkg/util/winutil"]),
    "pkg/util/log": GoModule("pkg/util/log"),
    "internal/tools": GoModule("internal/tools", condition=lambda: False, should_tag=False),
    "pkg/util/winutil": GoModule(
        "pkg/util/winutil", condition=lambda: sys.platform == 'win32', dependencies=["pkg/util/log"]
    ),
}

MAIN_TEMPLATE = """package main

import (
{imports}
)

func main() {{}}
"""

PACKAGE_TEMPLATE = '	_ "{}"'


@task
def generate_dummy_package(ctx, folder):
    import_paths = []
    for mod in DEFAULT_MODULES.values():
        if mod.path != "." and mod.condition():
            import_paths.append(mod.import_path)

    os.mkdir(folder)
    with ctx.cd(folder):
        print("Creating dummy 'main.go' file... ", end="")
        with open(os.path.join(ctx.cwd, 'main.go'), 'w') as main_file:
            main_file.write(
                MAIN_TEMPLATE.format(imports="\n".join(PACKAGE_TEMPLATE.format(path) for path in import_paths))
            )
        print("Done")

        ctx.run("go mod init")
        for mod in DEFAULT_MODULES.values():
            if mod.path != ".":
                ctx.run("go mod edit -require={}".format(mod.dependency_path("0.0.0")))
                ctx.run("go mod edit -replace {}=../{}".format(mod.import_path, mod.path))
