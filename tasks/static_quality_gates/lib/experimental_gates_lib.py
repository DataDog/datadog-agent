import glob
import os
import tempfile

import yaml
from invoke import Context
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.package.size import directory_size, extract_package, file_size
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler, read_byte_input


class StaticQualityGateFailed(Exception):
    """
    Exception raised when a static quality gate fails
    """
    def __init__(self, message: str):
        self.message = color_message(message, "red")
        super().__init__(self.message)

class StaticQualityGate:
    """
    Base class for all static quality gates
    that contains the common attributes for all static quality gates
    """

    gate_name: str
    arch: str
    os: str
    metric_handler: GateMetricHandler
    max_on_wire_size: int
    max_on_disk_size: int
    artifact_on_disk_size: int
    artifact_on_wire_size: int
    artifact_path: str # Path to the artifact to be used for the gate
    ctx: Context

    def __init__(self, gate_name: str, gate_max_size_values: dict, ctx: Context):
        self.gate_name = gate_name
        self.max_on_wire_size = read_byte_input(gate_max_size_values["max_on_wire_size"])
        self.max_on_disk_size = read_byte_input(gate_max_size_values["max_on_disk_size"])
        self._set_arch(gate_name)
        self._set_os(gate_name)
        self._register_gate_metrics()
        self.ctx = ctx

    def _set_arch(self, gate_name: str):
        if "amd64" in gate_name:
            self.arch = "amd64"
        elif "arm64" in gate_name:
            self.arch = "arm64"
        elif "armhf" in gate_name:
            self.arch = "armhf"
        else:
            raise ValueError(f"Unknown arch for gate: {gate_name}")

    def _set_os(self, gate_name: str):
        if "deb" in gate_name:
            self.os = "debian"
        elif "rpm" in gate_name:
            self.os = "centos"
        elif "suse" in gate_name:
            self.os = "suse"
        elif "heroku" in gate_name:
            self.os = "debian"
        else:
            raise ValueError(f"Unknown os for gate: {gate_name}")

    def _register_gate_metrics(self):
        """
        Register the gate tags and metrics to the metric handler
        to send data to Datadog
        """
        self.metric_handler = GATE_METRIC_HANDLER
        self.metric_handler.register_gate_tags(self.gate_name, gate_name=self.gate_name, arch=self.arch, os=self.os)
        self.metric_handler.register_metric(self.gate_name, "max_on_wire_size", self.max_on_wire_size)
        self.metric_handler.register_metric(self.gate_name, "max_on_disk_size", self.max_on_disk_size)

    def _find_package_path(self, extension: str = None) -> None:
        """
        Find the package path based on os, arch and flavor
        of the agent.
        param: extension: the extension of the package
        return: the path to the package
        """
        flavor = "datadog-agent"
        if "fips" in self.gate_name:
            flavor = "datadog-fips-agent"
        elif "iot" in self.gate_name:
            flavor = "datadog-iot-agent"
        elif "dogstatsd" in self.gate_name:
            flavor = "datadog-dogstatsd"
        elif "cluster-agent" in self.gate_name:
            flavor = "cluster-agent"

        package_dir = os.environ['OMNIBUS_PACKAGE_DIR']
        separator = '_' if self.os == 'debian' else '-'
        if not extension:
            extension = "deb" if self.os == 'debian' else "rpm"
        if self.os == "windows":
            package_dir = f"{package_dir}/pipeline-{os.environ['CI_PIPELINE_ID']}"
        glob_pattern = f'{package_dir}/{flavor}{separator}7*{self.arch}.{extension}'
        package_paths = glob.glob(glob_pattern)
        if len(package_paths) > 1:
            raise Exit(code=1, message=color_message(f"Too many files matching {glob_pattern}: {package_paths}", "red"))
        elif len(package_paths) == 0:
            raise Exit(code=1, message=color_message(f"Couldn't find any file matching {glob_pattern}", "red"))
        self.artifact_path = package_paths[0]

    def _calculate_package_size(self) -> None:
        """
        Calculate the size of the package on wire and on disk
        TODO: Think of testing this function, unit tests are not possible because real
        packages are used.
        """
        with tempfile.TemporaryDirectory() as extract_dir:
            extract_package(ctx=self.ctx, package_os=self.os, package_path=self.artifact_path, extract_dir=extract_dir)
            package_on_wire_size = file_size(path=self.artifact_path)
            package_on_disk_size = directory_size(path=extract_dir)

        self.metric_handler.register_metric(self.gate_name, "current_on_wire_size", package_on_wire_size)
        self.metric_handler.register_metric(self.gate_name, "current_on_disk_size", package_on_disk_size)
        self.artifact_on_wire_size = package_on_wire_size
        self.artifact_on_disk_size = package_on_disk_size

    def _check_package_size(self):
        """
        Check the size of the package on wire and on disk
        """
        error_message = ""
        if self.artifact_on_wire_size > self.max_on_wire_size:
            err_msg = f"Package size on wire (compressed package size) {self.artifact_on_wire_size} is higher than the maximum allowed {self.max_on_wire_size} by the gate !\n"
            error_message += err_msg

        if self.artifact_on_disk_size > self.max_on_disk_size:
            err_msg = f"Package size on disk (uncompressed package size) {self.artifact_on_disk_size} is higher than the maximum allowed {self.max_on_disk_size} by the gate !\n"
            print(color_message(err_msg, "red"))
            error_message += err_msg

        if error_message:
            raise StaticQualityGateFailed(error_message)

    def print_results(self) -> None:
        """
        Print the results of the gate
        in case of success
        """
        print(
                color_message(
                f"package_on_wire_size <= max_on_wire_size, ({self.artifact_on_wire_size}) <= ({self.max_on_wire_size})",
                "green",
            )
        )
        print(
                color_message(
                f"package_on_disk_size <= max_on_disk_size, ({self.artifact_on_disk_size}) <= ({self.max_on_disk_size})",
                "green",
            )
        )
    def entrypoint(self):
        """
        Entrypoint for the gate to measure the size of the package
        """
        if self.os not in ["docker", "windows"]:
            print(f"Triggering package quality gate for {self.gate_name}")
            self._find_package_path()
            print(f"Package path found: {self.artifact_path}")
            self._calculate_package_size()
            print(f"Package size calculated: {self.artifact_on_wire_size} on wire, {self.artifact_on_disk_size} on disk")
            self._check_package_size()
            print(color_message(f"✅ Package size check passed for {self.gate_name}", "green")) 
        if self.os == "windows":
            # TODO: trigger MSI quality gate
            pass
        if self.os == "docker":
            # TODO: trigger docker quality gate
            pass

    def debug_entrypoint(self):
        """
        Entrypoint for the gate to measure the size of the package
        to debug the gate
        """
        pass


# We are using the same metric handler for all gates
# to send data to Datadog
GATE_METRIC_HANDLER = GateMetricHandler(
    git_ref=os.environ["CI_COMMIT_REF_SLUG"], bucket_branch=os.environ["BUCKET_BRANCH"]
)


def get_quality_gates_list(config_path: str, ctx: Context) -> list[StaticQualityGate]:
    """
    Get the list of quality gates from the configuration file
    param: config_path: the path to the configuration file
    return: a list of StaticQualityGate objects
    """
    with open(config_path) as file:
        config = yaml.safe_load(file)
    print(f"{config_path} correctly parsed !")
    gates = [StaticQualityGate(gate_name, config[gate_name], ctx) for gate_name in config]
    newline_tab = "\n\t"
    print(f"The following gates are going to run:{newline_tab}- {(newline_tab + '- ').join(gate.gate_name for gate in gates)}")
    return gates
