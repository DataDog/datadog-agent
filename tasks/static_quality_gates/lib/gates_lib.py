import glob
import json
import math
import os
import sys
import tempfile
from abc import abstractmethod
from datetime import datetime
from io import UnsupportedOperation

import yaml
from invoke import Context

from tasks.libs.common.color import color_message
from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.datadog_api import create_gauge, send_metrics
from tasks.libs.common.utils import get_metric_origin
from tasks.libs.package.size import InfraError, directory_size, extract_package, file_size

# arch definitions are different
# depending on OS, that is why
# we have to map those
ARCH_MAPPING = {
    "amd64": "x86_64",
    "arm64": "aarch64",
    "armhf": "armv7hl",
}

PACKAGE_OS_MAPPING = {
    "deb": "debian",
    "rpm": "centos",
    "suse": "suse",
    "heroku": "debian",
    "msi": "windows",
}


def byte_to_string(size: int, unit_power: int = None, with_unit: bool = True) -> str:
    """
    Convert a byte size to a string with unit suffix.
    param: size: the size along with the unit suffix
    param: unit_power: the power of the unit to use
    param: with_unit: whether to include the unit suffix in the returned string
    return: size as a string
    """
    if not size:
        return f"0{' B' if with_unit else ''}"
    sign = ""
    if size < 0:
        size *= -1
        sign = "-"
    size_name = ("B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB")
    if unit_power is None:
        unit_power = int(math.log(size, 1024))
    p = math.pow(1024, unit_power)
    s = round(size / p, 2)
    # If s is not exactly 0 but rounded like (0.0 or -0.0)
    # Goal is to output +0 / -0 for very small changes and 0 for no changes at all
    if id(s) != id(0) and s == 0:
        s = 0
    return f"{sign}{s}{' ' + size_name[unit_power] if with_unit else ''}"


def string_to_latex_color(text: str) -> str:
    """
    Convert a string to a latex color.
    param: text: the text to convert
    return: the text as a latex color
    """
    # Github latex colors are currently broken, we are disabling this function's color temporarily for now
    return r"$${" + text + "}$$"


def string_to_byte(size: str) -> int:
    """
    Convert a string to a byte size.
    param: size: the size as a string
    return: the size in bytes
    """
    if not size:
        return 0
    size_name = ("KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB")
    value = None
    power = 0
    for k, unit in enumerate(size_name):
        if unit in size:
            value = float(size.replace(unit, ""))
            power = k + 1  # We start with KiB = 1024^1
            break
    if value:
        return int(value * math.pow(1024, power))
    elif "B" in size:
        return int(float(size.replace("B", "")))
    else:
        return int(size)


def read_byte_input(byte_input: str | int) -> int:
    """
    Read a byte input and return the size in bytes.
    param: byte_input: the byte input
    return: the size in bytes
    """
    if isinstance(byte_input, str):
        return string_to_byte(byte_input)
    else:
        return byte_input


class GateMetricHandler:
    # All metric_name -> metric_key
    METRICS_DICT = {
        "datadog.agent.static_quality_gate.on_wire_size": "current_on_wire_size",
        "datadog.agent.static_quality_gate.on_disk_size": "current_on_disk_size",
        "datadog.agent.static_quality_gate.max_allowed_on_wire_size": "max_on_wire_size",
        "datadog.agent.static_quality_gate.max_allowed_on_disk_size": "max_on_disk_size",
    }
    S3_REPORT_PATH = "s3://dd-ci-artefacts-build-stable/datadog-agent/static_quality_gates"

    def __init__(self, git_ref, bucket_branch, filename=None):
        self.metrics = {}
        self.metadata = {}
        self.git_ref = git_ref
        self.bucket_branch = bucket_branch
        self.series_is_complete = True

        if filename is not None:
            self._load_metrics_report(filename)

    def get_formatted_metric(self, gate_name, metric_name, with_unit=False):
        value = self.metrics[gate_name][metric_name]
        string_value = byte_to_string(value, with_unit=with_unit, unit_power=2)
        if value > 0:
            string_value = "+" + string_value
            return string_to_latex_color(string_value)
        elif value < 0:
            return string_to_latex_color(string_value)
        else:
            return string_to_latex_color(string_value)

    def get_formatted_metric_comparison(self, gate_name, first_metric, limit_metric):
        first_value = self.metrics[gate_name][first_metric]
        second_value = self.metrics[gate_name][limit_metric]
        limit_value_string = r"$${" + byte_to_string(second_value, unit_power=2, with_unit=False) + "}$$"
        if first_value > second_value:
            return f"{string_to_latex_color(byte_to_string(first_value, unit_power=2, with_unit=False))} > {limit_value_string}"
        elif first_value < second_value:
            return f"{string_to_latex_color(byte_to_string(first_value, unit_power=2, with_unit=False))} < {limit_value_string}"
        else:
            return f"{string_to_latex_color(byte_to_string(first_value, unit_power=2, with_unit=False))} = {limit_value_string}"

    def register_metric(self, gate_name, metric_name, metric_value):
        if self.metrics.get(gate_name, None) is None:
            self.metrics[gate_name] = {}

        self.metrics[gate_name][metric_name] = metric_value

    def register_gate_tags(self, gate, **kwargs):
        if self.metadata.get(gate, None) is None:
            self.metadata[gate] = {}

        for key in kwargs:
            self.metadata[gate][key] = kwargs[key]

    def _load_metrics_report(self, filename):
        with open(filename) as f:
            self.metrics = json.load(f)

    def _add_gauge(self, timestamp, common_tags, gate, metric_name, metric_key):
        if self.metrics[gate].get(metric_key):
            return create_gauge(
                metric_name,
                timestamp,
                self.metrics[gate][metric_key],
                tags=common_tags,
                metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
                unit="byte",
            )
        return None

    def generate_relative_size(
        self, ctx, filename="static_gate_report.json", report_path="static_gate_report.json", ancestor=None
    ):
        if ancestor:
            # Fetch the ancestor's static quality gates report json file
            out = ctx.run(
                f"aws s3 cp --only-show-errors --region us-east-1 --sse AES256 {self.S3_REPORT_PATH}/{ancestor}/{filename} {report_path}",
                hide=True,
                warn=True,
            )
            if out.exited == 0:
                # Load the report inside of a GateMetricHandler specific to the ancestor
                ancestor_metric_handler = GateMetricHandler(ancestor, self.bucket_branch, report_path)
                for gate in self.metrics:
                    ancestor_gate = ancestor_metric_handler.metrics.get(gate)
                    if not ancestor_gate:
                        continue
                    # Compute the difference between the wire and disk size of common gates between the ancestor and the current pipeline
                    for metric_key in ["current_on_wire_size", "current_on_disk_size"]:
                        if self.metrics[gate].get(metric_key) and ancestor_gate.get(metric_key):
                            relative_metric_size = self.metrics[gate][metric_key] - ancestor_gate[metric_key]
                            self.register_metric(gate, metric_key.replace("current", "relative"), relative_metric_size)
            else:
                print(
                    color_message(
                        f"[WARN] Unable to fetch quality gates {report_path} from {ancestor} !\nstdout:\n{out.stdout}\nstderr:\n{out.stderr}",
                        "orange",
                    )
                )
        else:
            print(
                color_message(
                    "[WARN] Unable to find this commit ancestor",
                    "orange",
                )
            )

    def _generate_series(self):
        if not self.git_ref or not self.bucket_branch:
            return None

        series = []
        timestamp = int(datetime.utcnow().timestamp())
        for gate in self.metrics:
            common_tags = [
                f"git_ref:{self.git_ref}",
                f"bucket_branch:{self.bucket_branch}",
            ]

            if self.metadata.get(gate, None) is None:
                print(color_message(f"[WARN] gate {gate} doesn't have gate tags registered ! skipping...", "orange"))
                continue

            for tag in self.metadata[gate]:
                common_tags.append(f"{tag}:{self.metadata[gate][tag]}")

            for metric_name, metric_key in self.METRICS_DICT.items():
                gauge = self._add_gauge(timestamp, common_tags, gate, metric_name, metric_key)
                if gauge:
                    series.append(gauge)
                else:
                    print(
                        color_message(
                            f"[WARN] gate {gate} doesn't have the {metric_name} metric registered ! skipping metric...",
                            "orange",
                        )
                    )
                    self.series_is_complete = False
        return series

    def send_metrics_to_datadog(self):
        series = self._generate_series()

        if series:
            send_metrics(series=series)
        print(color_message("Metric sending finished !", "blue"))

    def generate_metric_reports(self, ctx, filename="static_gate_report.json", branch=None, is_nightly=False):
        if not self.series_is_complete:
            print(
                color_message(
                    "[WARN] Some static quality gates are missing some metrics, the generated report might not be trustworthy.",
                    "orange",
                )
            )

        with open(filename, "w") as f:
            json.dump(self.metrics, f)

        CI_COMMIT_SHA = os.environ.get("CI_COMMIT_SHA")
        if not is_nightly and branch == "main" and CI_COMMIT_SHA:
            ctx.run(
                f"aws s3 cp --only-show-errors --region us-east-1 --sse AES256 {filename} {self.S3_REPORT_PATH}/{CI_COMMIT_SHA}/{filename}",
                hide="stdout",
            )


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
        elif "msi" in self.gate_name:
            # MSI packages are always amd64 (x86_64) on Windows
            self.arch = "amd64"
        else:
            raise ValueError(f"Unknown arch for gate: {self.gate_name}")

    def print_results(self) -> None:
        """
        Print the results of the gate
        in case of success
        """
        from .static_quality_gates_reporter import QualityGateOutputFormatter

        QualityGateOutputFormatter.print_enhanced_gate_result(
            self.gate_name,
            self.artifact_path,
            self.artifact_on_wire_size,
            self.max_on_wire_size,
            self.artifact_on_disk_size,
            self.max_on_disk_size,
        )

    @abstractmethod
    def _measure_on_disk_and_on_wire_size(self):
        """
        Measure the size of the artifact on disk and on wire
        """
        raise NotImplementedError("This method should be implemented by the subclass")

    def check_artifact_size(self):
        """
        Check the size of the artifact on wire and on disk
        against max_on_wire_size and max_on_disk_size.
        If the artifact exceeds the maximum allowed size, raise a StaticQualityGateFailed exception.
        """
        error_message = ""
        if self.artifact_on_wire_size > self.max_on_wire_size:
            error_message += f"On wire size (compressed artifact size) {self.artifact_on_wire_size / 1024 / 1024} MB is higher than the maximum allowed {self.max_on_wire_size / 1024 / 1024} MB by the gate !\n"
        if self.artifact_on_disk_size > self.max_on_disk_size:
            error_message += f"On disk size (uncompressed artifact size) {self.artifact_on_disk_size / 1024 / 1024} MB is higher than the maximum allowed {self.max_on_disk_size / 1024 / 1024} MB by the gate !\n"
        if error_message:
            error_message = color_message(f"{self.gate_name} failed!\n" + error_message, "red")
            raise StaticQualityGateFailed(error_message)

    def execute_gate(self):
        """
        Execute the quality gate.
        """
        from .static_quality_gates_reporter import QualityGateOutputFormatter

        # To ensure execute_gate is generic we define an abstract method
        # to measure the size of the artifact on disk and on wire
        # and a method to check the size of the artifact against the maximum allowed size.
        # This way we postpone the processing of the artifact until all the gates are loaded
        # and then can handle exceptions centrally in quality_gates.py
        # TODO: quality_gates.py should be refactored. Most probably, the task should be closer
        # to this lib.
        self._measure_on_disk_and_on_wire_size()
        QualityGateOutputFormatter.print_enhanced_gate_execution(self.gate_name, self.artifact_path)
        self.check_artifact_size()
        QualityGateOutputFormatter.print_enhanced_gate_success(self.gate_name)
        self.print_results()
        print("-" * 80)


class StaticQualityGatePackage(StaticQualityGate):
    """
    Static quality gate for packages
    """

    def _set_os(self):
        for package_type in PACKAGE_OS_MAPPING.keys():
            if package_type in self.gate_name:
                self.os = PACKAGE_OS_MAPPING[package_type]
                if self.os in ['centos', 'suse']:
                    self.arch = ARCH_MAPPING[self.arch]
                elif self.os == 'windows':
                    # MSI packages use x86_64 naming convention
                    self.arch = ARCH_MAPPING[self.arch]
                return
        raise ValueError(f"Unknown os for gate: {self.gate_name}")

    def __init__(self, gate_name: str, gate_max_size_values: dict, ctx: Context):
        super().__init__(gate_name, gate_max_size_values, ctx)
        self._set_os()

    def _find_package_path(self, extension: str = None) -> None:
        """
        Find the package path based on os, arch and flavor
        of the agent.
        param: extension: the extension of the package
        return: the path to the package
        """
        # MSI special case: requires both ZIP and MSI files
        if "msi" in self.gate_name:
            self.zip_path = self._find_package_by_pattern("datadog-agent", "zip")
            self.msi_path = self._find_package_by_pattern("datadog-agent", "msi")
            self.artifact_path = self.msi_path  # Primary artifact for reporting
            return

        # Determine flavor based on gate name
        flavor = "datadog-agent"
        if "fips" in self.gate_name:
            flavor = "datadog-fips-agent"
        elif "iot" in self.gate_name:
            flavor = "datadog-iot-agent"
        elif "dogstatsd" in self.gate_name:
            flavor = "datadog-dogstatsd"
        elif "heroku" in self.gate_name:
            flavor = "datadog-heroku-agent"

        # Determine separator and extension based on OS
        separator = '_' if self.os == 'debian' else '-'
        if not extension:
            extension = 'deb' if self.os == 'debian' else 'rpm'

        # Use generic helper to find the package
        self.artifact_path = self._find_package_by_pattern(flavor, extension, separator)

    def _find_package_by_pattern(self, flavor: str, extension: str, separator: str = '-') -> str:
        """
        Generic helper to find packages by flavor, extension and separator.
        Handles common package discovery logic with proper error handling.
        """
        package_dir = os.environ['OMNIBUS_PACKAGE_DIR']
        if self.os == "windows":
            package_dir = f"{package_dir}/pipeline-{os.environ['CI_PIPELINE_ID']}"

        glob_pattern = f'{package_dir}/{flavor}{separator}7*{self.arch}.{extension}'
        package_paths = glob.glob(glob_pattern)
        if len(package_paths) > 1:
            raise ValueError(f"Too many {extension.upper()} files matching {glob_pattern}: {package_paths}")
        elif len(package_paths) == 0:
            raise ValueError(f"Couldn't find any {extension.upper()} file matching {glob_pattern}")
        return package_paths[0]

    def _calculate_package_size(self) -> None:
        """
        Calculate the size of the package on wire and on disk
        TODO: Think of testing this function, unit tests are not possible because real
        packages are used.
        """
        if "msi" in self.gate_name:
            # MSI special case: extract ZIP file for disk size, measure MSI file for wire size
            with tempfile.TemporaryDirectory() as extract_dir:
                extract_package(ctx=self.ctx, package_os=self.os, package_path=self.zip_path, extract_dir=extract_dir)
                package_on_wire_size = file_size(path=self.msi_path)
                package_on_disk_size = directory_size(path=extract_dir)
        else:
            # Standard package handling
            with tempfile.TemporaryDirectory() as extract_dir:
                extract_package(
                    ctx=self.ctx, package_os=self.os, package_path=self.artifact_path, extract_dir=extract_dir
                )
                package_on_wire_size = file_size(path=self.artifact_path)
                package_on_disk_size = directory_size(path=extract_dir)

        self.artifact_on_wire_size = package_on_wire_size
        self.artifact_on_disk_size = package_on_disk_size

    def _measure_on_disk_and_on_wire_size(self):
        """
        Measure the size of the package on disk and on wire
        """
        self._find_package_path()
        self._calculate_package_size()


class StaticQualityGateDocker(StaticQualityGate):
    """
    Static quality gate for docker images
    """

    def _set_os(self):
        self.os = "docker"

    def __init__(self, gate_name: str, gate_max_size_values: dict, ctx: Context):
        super().__init__(gate_name, gate_max_size_values, ctx)
        self._set_os()

    def _get_image_url(self) -> str:
        """
        Get the url of the docker image to be used for the gate.
        We first determine the flavor of the image to be used.
        Then we check if the gate is a nightly run.
        After that we check if the docker image contains jmx.
        Finally we check if the docker image is for windows and if so we check the version of the windows image.
        return: the url of the docker image
        """

        if "cluster" in self.gate_name:
            flavor = "cluster-agent"
        elif "dogstatsd" in self.gate_name:
            flavor = "dogstatsd"
        elif "cws_instrumentation" in self.gate_name:
            flavor = "cws-instrumentation"
        elif "agent" in self.gate_name:
            flavor = "agent"
        else:
            raise ValueError(f"Unknown docker image flavor for gate: {self.gate_name}")

        jmx = ""
        if "jmx" in self.gate_name:
            jmx = "-jmx"

        image_suffix = ("-7" if flavor == "agent" else "") + jmx

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
        for k, line in enumerate(image_content):
            if k % 2 == 0:
                if "tar.gz" in image_content[k + 1]:
                    image_tar_gz.append(image_content[k + 1])
                else:
                    on_disk_size += int(line)
        if image_tar_gz:
            for image in image_tar_gz:
                on_disk_size += int(self.ctx.run(f"tar -xf {image} --to-stdout | wc -c", hide=True).stdout)
        else:
            print(color_message("[WARN] No tar.gz file found inside of the image", "orange"), file=sys.stderr)

        print(f"Current image on disk size for {self.artifact_path}: {on_disk_size / 1024 / 1024} MB")
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
        print(f"Current image on wire size for {self.artifact_path}: {on_wire_size / 1024 / 1024} MB")
        self.artifact_on_wire_size = on_wire_size

    def _measure_on_disk_and_on_wire_size(self):
        """
        Measure the size of the docker image on disk and on wire
        """
        self._get_image_url()
        self._calculate_image_on_disk_size()
        self._calculate_image_on_wire_size()


def get_quality_gates_list(config_path: str, ctx: Context) -> list[StaticQualityGate]:
    """
    Parse the list of quality gates from the configuration file and return a list of StaticQualityGate objects.
    param: config_path: the path to the configuration file
    return: a list of StaticQualityGate objects
    """
    with open(config_path) as file:
        config = yaml.safe_load(file)

    gates: list[StaticQualityGate] = []
    for gate_name in config:
        if "docker" in gate_name:
            gates.append(StaticQualityGateDocker(gate_name, config[gate_name], ctx))
        elif any(package_type in gate_name for package_type in ["deb", "rpm", "heroku", "suse", "msi"]):
            gates.append(StaticQualityGatePackage(gate_name, config[gate_name], ctx))
        else:
            raise UnsupportedOperation(f"Unknown gate type: {gate_name}")

    from .static_quality_gates_reporter import QualityGateOutputFormatter

    QualityGateOutputFormatter.print_startup_message(len(gates), config_path)
    return gates
