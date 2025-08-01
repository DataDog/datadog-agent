import glob
import os
import sys
import tempfile
from abc import abstractmethod
from io import UnsupportedOperation

import yaml
from invoke import Context
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.package.size import InfraError, directory_size, extract_package, file_size
from tasks.static_quality_gates.lib.gates_lib import GateMetricHandler, read_byte_input

# We are using the same metric handler for all gates
# to send data to Datadog
GATE_METRIC_HANDLER = None


def get_metric_handler() -> GateMetricHandler:
    """
    Get the metric handler for the static quality gates.
    This is a lazy singleton implementation to avoid creating a new metric handler for each gate
    as we use exactly the same handler with different tags for each gate.
    """
    global GATE_METRIC_HANDLER
    if GATE_METRIC_HANDLER is None:
        GATE_METRIC_HANDLER = GateMetricHandler(
            git_ref=os.environ["CI_COMMIT_REF_SLUG"], bucket_branch=os.environ["BUCKET_BRANCH"]
        )
    return GATE_METRIC_HANDLER


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
    artifact_path: str  # Path to the artifact to be used for the gate
    ctx: Context

    def __init__(self, gate_name: str, gate_max_size_values: dict, ctx: Context):
        self.gate_name = gate_name
        self.max_on_wire_size = read_byte_input(gate_max_size_values["max_on_wire_size"])
        self.max_on_disk_size = read_byte_input(gate_max_size_values["max_on_disk_size"])
        self._set_arch()
        self.ctx = ctx

    def _set_arch(self):
        if "amd64" in self.gate_name:
            self.arch = "amd64"
        elif "arm64" in self.gate_name:
            self.arch = "arm64"
        elif "armhf" in self.gate_name:
            self.arch = "armhf"
        else:
            raise ValueError(f"Unknown arch for gate: {self.gate_name}")

    def _register_gate_metrics(self):
        """
        Register the gate tags and metrics to the metric handler
        to send data to Datadog
        """
        self.metric_handler = get_metric_handler()
        self.metric_handler.register_gate_tags(self.gate_name, gate_name=self.gate_name, arch=self.arch, os=self.os)
        self.metric_handler.register_metric(self.gate_name, "max_on_wire_size", self.max_on_wire_size)
        self.metric_handler.register_metric(self.gate_name, "max_on_disk_size", self.max_on_disk_size)

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

    def _check_artifact_size(self):
        """
        Check the size of the artifact on wire and on disk
        against max_on_wire_size and max_on_disk_size.
        If the artifact exceeds the maximum allowed size, raise a StaticQualityGateFailed exception.
        """
        error_message = ""
        if self.artifact_on_wire_size > self.max_on_wire_size:
            error_message += f"On wire size (compressed artifact size) {self.artifact_on_wire_size} is higher than the maximum allowed {self.max_on_wire_size} by the gate !\n"
        if self.artifact_on_disk_size > self.max_on_disk_size:
            error_message += f"On disk size (uncompressed artifact size) {self.artifact_on_disk_size} is higher than the maximum allowed {self.max_on_disk_size} by the gate !\n"
        if error_message:
            error_message = color_message(f"{self.gate_name} failed!\n" + error_message, "red")
            raise StaticQualityGateFailed(error_message)

    @abstractmethod
    def execute_gate(self):
        """
        Execute the quality gate.
        """
        raise NotImplementedError("This method should be implemented by the subclass")


class StaticQualityGatePackage(StaticQualityGate):
    """
    Static quality gate for packages
    """

    def _set_os(self):
        if "deb" in self.gate_name:
            self.os = "debian"
        elif "rpm" in self.gate_name:
            self.os = "centos"
        elif "suse" in self.gate_name:
            self.os = "suse"
        elif "heroku" in self.gate_name:
            self.os = "debian"
        else:
            raise ValueError(f"Unknown os for gate: {self.gate_name}")

    def __init__(self, gate_name: str, gate_max_size_values: dict, ctx: Context):
        super().__init__(gate_name, gate_max_size_values, ctx)
        self._set_os()
        self._register_gate_metrics()

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

    def execute_gate(self):
        """
        Execute the package quality gate.
        This function will:
        - Find the package path
        - Calculate the size of the package on wire and on disk
        - Check the size of the package on wire and on disk
        """
        print(f"Triggering {self.gate_name}")
        self._find_package_path()
        print(f"Package path found: {self.artifact_path}")
        self._calculate_package_size()
        print(f"Package size calculated: {self.artifact_on_wire_size} on wire, {self.artifact_on_disk_size} on disk")
        self._check_artifact_size()
        print(color_message(f"✅ Package size check passed for {self.gate_name}", "green"))
        self.print_results()


class StaticQualityGateDocker(StaticQualityGate):
    """
    Static quality gate for docker images
    """

    def _set_os(self):
        self.os = "docker"

    def __init__(self, gate_name: str, gate_max_size_values: dict, ctx: Context):
        super().__init__(gate_name, gate_max_size_values, ctx)
        self._set_os()
        self._register_gate_metrics()
        self._get_image_url()

    def _get_image_url(self) -> str:
        """
        Get the url of the docker image to be used for the gate.
        We first determine the flavor of the image to be used.
        Then we check if the gate is a nightly run.
        After that we check if the docker image contains jmx.
        Finally we check if the docker image is for windows and if so we check the version of the windows image.
        return: the url of the docker image
        """

        if "agent" in self.gate_name:
            flavor = "agent"
        elif "cluster" in self.gate_name:
            flavor = "cluster-agent"
        elif "dogstatsd" in self.gate_name:
            flavor = "dogstatsd"
        elif "cws-instrumentation" in self.gate_name:
            flavor = "cws-instrumentation"
        else:
            raise ValueError(f"Unknown docker image flavor for gate: {self.gate_name}")

        jmx = ""
        if "jmx" in self.gate_name:
            jmx = "-jmx"

        windows_suffix = ""
        if "windows" in self.gate_name:
            if "1809" in self.gate_name:
                windows_suffix += "-win1809"
            elif "2022" in self.gate_name:
                windows_suffix += "-winltsc2022"
            if "core" in self.gate_name:
                windows_suffix += "-servercore"

        image_suffix = ("-7" if flavor == "agent" else "") + jmx + windows_suffix

        if os.environ["BUCKET_BRANCH"] == "nightly":
            flavor += "-nightly"

        if not os.environ["CI_PIPELINE_ID"] or not os.environ["CI_COMMIT_SHORT_SHA"]:
            raise StaticQualityGateFailed(
                color_message(
                    "This gate needs to be ran from the CI environment. (Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA)",
                    "red",
                )
            )

        pipeline_id = os.environ["CI_PIPELINE_ID"]
        commit_sha = os.environ["CI_COMMIT_SHORT_SHA"]

        self.artifact_path = (
            f"registry.ddbuild.io/ci/datadog-agent/{flavor}:v{pipeline_id}-{commit_sha}{image_suffix}-{self.arch}"
        )
        return self.artifact_path

    def _calculate_image_on_disk_size(self) -> None:
        """
        Calculate the size of the docker image on disk.
        To do so we use crane to pull the image locally
        and then we calculate the size of the image on disk.
        return: the size of the docker image on disk
        TODO: Think of testing this function, unit tests are not possible because real
        images are used.
        """
        # Pull image locally to get on disk size
        crane_output = self.ctx.run(f"crane pull {self.artifact_path} output.tar", warn=True)
        if crane_output.exited != 0:
            raise InfraError(f"Crane pull failed to retrieve {self.artifact_path}. Retrying... (infra flake)")
        # The downloaded image contains some metadata files and another tar.gz file. We are computing the sum of
        # these metadata files and the uncompressed size of the tar.gz inside of output.tar.
        self.ctx.run("tar -xf output.tar")
        image_content = self.ctx.run(
            "tar -tvf output.tar | awk -F' ' '{print $3; print $6}'", hide=True
        ).stdout.splitlines()
        on_disk_size = 0
        image_tar_gz = []
        print("Image on disk content :")
        for k, line in enumerate(image_content):
            if k % 2 == 0:
                if "tar.gz" in image_content[k + 1]:
                    image_tar_gz.append(image_content[k + 1])
                else:
                    on_disk_size += int(line)
            else:
                print(f"  - {line}")
        if image_tar_gz:
            for image in image_tar_gz:
                on_disk_size += int(self.ctx.run(f"tar -xf {image} --to-stdout | wc -c", hide=True).stdout)
        else:
            print(color_message("[WARN] No tar.gz file found inside of the image", "orange"), file=sys.stderr)

        print(f"Current image on disk size: {on_disk_size}")
        self.metric_handler.register_metric(self.gate_name, "current_on_disk_size", on_disk_size)
        self.artifact_on_disk_size = on_disk_size

    def _calculate_image_on_wire_size(self) -> None:
        """
        Calculate the size of the docker image on wire.
        To do so we use docker manifest inspect to get the size of the image on wire.
        return: the size of the docker image on wire
        TODO: Add unit test with mocked manifest output
        """
        manifest_output = self.ctx.run(
            "DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect -v "
            + self.artifact_path
            + " | grep size | awk -F ':' '{sum+=$NF} END {printf(\"%d\",sum)}'",
            hide=True,
        )

        on_wire_size = int(manifest_output.stdout)
        print(f"Current image on wire size: {on_wire_size}")
        self.metric_handler.register_metric(self.gate_name, "current_on_wire_size", on_wire_size)
        self.artifact_on_wire_size = on_wire_size

    def execute_gate(self):
        print(f"Triggering {self.gate_name}")
        self._get_image_url()
        print(f"Image url found: {self.artifact_path}")
        self._calculate_image_on_wire_size()
        self._calculate_image_on_disk_size()
        self._check_artifact_size()
        print(color_message(f"✅ Docker image size check passed for {self.gate_name}", "green"))
        self.print_results()


def get_quality_gates_list(config_path: str, ctx: Context) -> list[StaticQualityGate]:
    """
    Parse the list of quality gates from the configuration file and return a list of StaticQualityGate objects.
    param: config_path: the path to the configuration file
    return: a list of StaticQualityGate objects
    """
    with open(config_path) as file:
        config = yaml.safe_load(file)
    print(f"{config_path} correctly parsed !")
    gates: list[StaticQualityGate] = []
    for gate_name in config:
        if "docker" in gate_name:
            gates.append(StaticQualityGateDocker(gate_name, config[gate_name], ctx))
        elif any(package_type in gate_name for package_type in ["deb", "rpm", "heroku", "suse"]):
            gates.append(StaticQualityGatePackage(gate_name, config[gate_name], ctx))
        elif "msi" in gate_name:
            raise NotImplementedError(f"MSI gates are not supported yet: {gate_name}")
        else:
            raise UnsupportedOperation(f"Unknown gate type: {gate_name}")

    newline_tab = "\n\t"
    print(
        f"The following gates are going to run:{newline_tab}- {(newline_tab + '- ').join(gate.gate_name for gate in gates)}"
    )
    return gates
