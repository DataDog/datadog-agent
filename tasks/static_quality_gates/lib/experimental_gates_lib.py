import glob
import os
from dataclasses import dataclass

import yaml
from invoke import Context
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler, read_byte_input


@dataclass
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
    ctx: Context

    def __init__(self, gate_name: str, gate_max_size_values: dict):
        self.gate_name = gate_name
        self.max_on_wire_size = read_byte_input(gate_max_size_values["max_on_wire_size"])
        self.max_on_disk_size = read_byte_input(gate_max_size_values["max_on_disk_size"])
        self._set_arch(gate_name)
        self._set_os(gate_name)
        self._register_gate_metrics()

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

    def _find_package_path(self, extension: str = None) -> str:
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
        return package_paths[0]

    def entrypoint(self):
        """
        Entrypoint for the gate to measure the size of the package
        """
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


def parse_gate_config(gate_name: str, gate_max_size_values: dict) -> StaticQualityGate:
    """
    Parse the gate configuration
    param: config: the configuration dictionary
    return: a StaticQualityGate object
    """
    is_fips = 'fips' in gate_name

    # print(gate_name)
    # print(gate_max_size_values)
    return None


def get_quality_gates_list(config_path: str) -> list[StaticQualityGate]:
    """
    Get the list of quality gates from the configuration file
    param: config_path: the path to the configuration file
    return: a list of StaticQualityGate objects
    """
    with open(config_path) as file:
        config = yaml.safe_load(file)

    for key in config:
        gate = parse_gate_config(key, config[key])
    return []


get_quality_gates_list("test/static/static_quality_gates.yml")
