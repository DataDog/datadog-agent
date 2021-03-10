import os


class GoModule:
    """A Go module abstraction."""

    def __init__(self, path, targets=None, condition=lambda: True, dependencies=None):
        self.path = path
        self.targets = targets if targets else ["."]
        self.dependencies = dependencies if dependencies else []
        self.condition = condition

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

    def dependency_path(self, agent_version):
        """Return the versioned dependency path of the Go module
        >>> mods = [GoModule("."), GoModule("pkg/util/log")]
        >>> [mod.dependency_path("7.27.0") for mod in mods]
        ["github.com/DataDog/datadog-agent@v7.27.0", "github.com/DataDog/datadog-agent/pkg/util/log@v0.27.0"]        
        """
        go_path = "github.com/DataDog/datadog-agent"
        if self.path != ".":
            go_path += "/" + self.path
        return "{go_path}@{version}".format(go_path=go_path, version=self.__version(agent_version))


DEFAULT_MODULES = {
    ".": GoModule(".", targets=["./pkg", "./cmd"]),
}
