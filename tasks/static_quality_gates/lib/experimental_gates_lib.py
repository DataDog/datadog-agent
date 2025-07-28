from dataclasses import dataclass
from enum import Enum

from invoke import Context

from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler


class Arch(Enum):
    AMD64 = "amd64"
    ARM64 = "arm64"


class OS(Enum):
    DEBIAN = "debian"
    HEROKU = "heroku"
    MSI = "msi"
    RPM = "rpm"
    SUSE = "suse"
    DOCKER = "docker"


@dataclass
class StaticQualityGate:
    """
    Base class for all static quality gates
    that contains the common attributes for all static quality gates
    """

    gate_name: str
    arch: Arch
    os: OS
    gate_metric_handler: GateMetricHandler
    max_on_wire_size: int
    max_on_disk_size: int
    ctx: Context

    def configure_metric_handler(self):
        """
        Register the gate tags and metrics to the metric handler
        to send data to Datadog
        """
        self.gate_metric_handler.register_gate_tags(
            self.gate_name, gate_name=self.gate_name, arch=self.arch, os=self.os
        )
        self.gate_metric_handler.register_metric(self.gate_name, "max_on_wire_size", self.max_on_wire_size)
        self.gate_metric_handler.register_metric(self.gate_name, "max_on_disk_size", self.max_on_disk_size)

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


@dataclass
class DockerStaticQualityGate(StaticQualityGate):
    os = OS.DOCKER
    flavor: str


@dataclass
class PackageStaticQualityGate(StaticQualityGate):
    flavor: str
