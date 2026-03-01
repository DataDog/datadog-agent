"""
Static Quality Gates implementation using composition pattern.

This module provides quality gates that eliminate inheritance-based design issues:
- No subclassing - uses composition and strategy pattern
- No state mutation - immutable data flow through objects
- All attributes guaranteed to be defined - validation at creation
"""

import glob
import json
import math
import os
import tempfile
from dataclasses import dataclass
from datetime import datetime
from io import UnsupportedOperation
from typing import Protocol

import yaml
from invoke import Context

from tasks.libs.common.color import color_message
from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.datadog_api import create_gauge, send_metrics
from tasks.libs.common.git import is_a_release_branch
from tasks.libs.common.utils import get_metric_origin
from tasks.libs.package.size import InfraError, directory_size, extract_package, file_size

# Architecture definitions are different depending on OS
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


class StaticQualityGateError(Exception):
    """
    Exception raised when a static quality gate fails
    """

    def __init__(self, message: str):
        super().__init__(color_message(message, "red"))


@dataclass(frozen=True)
class ArtifactMeasurement:
    """
    Data class containing artifact measurement results.
    """

    artifact_path: str
    on_wire_size: int  # Compressed size in bytes
    on_disk_size: int  # Uncompressed size in bytes

    def __post_init__(self):
        """Validate measurement data"""
        if not self.artifact_path:
            raise ValueError("artifact_path cannot be empty")
        if self.on_wire_size < 0:
            raise ValueError("on_wire_size must be non-negative")
        if self.on_disk_size < 0:
            raise ValueError("on_disk_size must be non-negative")


@dataclass(frozen=True)
class QualityGateConfig:
    """
    Configuration for a quality gate.
    The name and max values are read from the yaml file.
    The arch and os are inferred from the gate name.
    """

    gate_name: str
    max_on_wire_size: int
    max_on_disk_size: int
    arch: str
    os: str

    def __post_init__(self):
        """Validate configuration data"""
        if not self.gate_name:
            raise ValueError("gate_name cannot be empty")
        if self.max_on_wire_size <= 0:
            raise ValueError("max_on_wire_size must be strictly positive")
        if self.max_on_disk_size <= 0:
            raise ValueError("max_on_disk_size must be strictly positive")
        if not self.arch:
            raise ValueError("arch cannot be empty")
        if not self.os:
            raise ValueError("os cannot be empty")


@dataclass(frozen=True)
class SizeViolation:
    """Represents a size limit violation"""

    measurement_type: str  # "wire" or "disk"
    current_size: int
    max_size: int

    @property
    def excess_bytes(self) -> int:
        """Number of bytes over the limit"""
        return self.current_size - self.max_size

    @property
    def excess_mb(self) -> float:
        """Number of MB over the limit"""
        return self.excess_bytes / (1024 * 1024)


@dataclass(frozen=True)
class GateResult:
    """
    Result of executing a quality gate.
    Contains all information needed for reporting.
    """

    config: QualityGateConfig
    measurement: ArtifactMeasurement
    violations: list[SizeViolation]
    success: bool

    @property
    def wire_remaining_bytes(self) -> int:
        """Remaining wire size capacity in bytes"""
        return max(0, self.config.max_on_wire_size - self.measurement.on_wire_size)

    @property
    def disk_remaining_bytes(self) -> int:
        """Remaining disk size capacity in bytes"""
        return max(0, self.config.max_on_disk_size - self.measurement.on_disk_size)


class ArtifactMeasurer(Protocol):
    """
    Protocol for measuring artifacts.
    Implementations handle specific artifact types (Docker, packages, etc.)
    """

    def measure(self, ctx: Context, config: QualityGateConfig) -> ArtifactMeasurement:
        """
        Measure an artifact's on-wire and on-disk sizes.

        Args:
            ctx: Invoke context for running commands
            config: Quality gate configuration

        Returns:
            ArtifactMeasurement with populated sizes

        Raises:
            StaticQualityGateFailed: If measurement fails
            InfraError: If there's an infrastructure issue (retryable)
        """
        ...


class PackageArtifactMeasurer:
    """
    Measures package artifacts (DEB, RPM, MSI, etc.).
    """

    def measure(self, ctx: Context, config: QualityGateConfig) -> ArtifactMeasurement:
        """Measure package artifact sizes"""
        try:
            artifact_paths = self._find_package_paths(config)
            wire_size, disk_size = self._calculate_package_sizes(ctx, config, artifact_paths)

            # For packages, the primary artifact path is the main package file
            primary_path = artifact_paths.get('primary', artifact_paths.get('msi', ''))

            return ArtifactMeasurement(artifact_path=primary_path, on_wire_size=wire_size, on_disk_size=disk_size)
        except (StaticQualityGateError, InfraError):
            raise
        except Exception as e:
            raise StaticQualityGateError(f"Failed to measure in {config.gate_name}: {str(e)}") from e

    def _find_package_paths(self, config: QualityGateConfig) -> dict:
        """
        Find package file paths based on gate configuration.

        Returns:
            Dictionary with package paths. For MSI, includes both 'zip' and 'msi' keys.
            For other packages, includes 'primary' key.
        """
        # MSI special case: requires both ZIP and MSI files
        if "msi" in config.gate_name:
            return {
                'zip': self._find_package_by_pattern("datadog-agent", "zip", config),
                'msi': self._find_package_by_pattern("datadog-agent", "msi", config),
            }

        # Determine flavor based on gate name
        flavor = self._extract_package_flavor(config.gate_name)

        # Determine separator and extension based on OS
        separator = '_' if config.os == 'debian' else '-'
        extension = 'deb' if config.os == 'debian' else 'rpm'

        return {'primary': self._find_package_by_pattern(flavor, extension, config, separator)}

    def _extract_package_flavor(self, gate_name: str) -> str:
        """Extract package flavor from gate name"""
        if "fips" in gate_name:
            return "datadog-fips-agent"
        elif "iot" in gate_name:
            return "datadog-iot-agent"
        elif "dogstatsd" in gate_name:
            return "datadog-dogstatsd"
        elif "heroku" in gate_name:
            return "datadog-heroku-agent"
        else:
            return "datadog-agent"

    def _find_package_by_pattern(
        self, flavor: str, extension: str, config: QualityGateConfig, separator: str = '-'
    ) -> str:
        """
        Find package file by pattern with proper error handling.
        """
        package_dir = os.environ['OMNIBUS_PACKAGE_DIR']
        if config.os == "windows":
            package_dir = f"{package_dir}/pipeline-{os.environ['CI_PIPELINE_ID']}"

        # Map architecture for certain OSes
        arch = config.arch
        if config.os in ['centos', 'suse', 'windows']:
            arch = ARCH_MAPPING.get(arch, arch)

        glob_pattern = f'{package_dir}/{flavor}{separator}7*{arch}.{extension}'
        package_paths = glob.glob(glob_pattern)

        if len(package_paths) > 1:
            raise ValueError(f"Too many {extension.upper()} files matching {glob_pattern}: {package_paths}")
        elif len(package_paths) == 0:
            raise ValueError(f"Couldn't find any {extension.upper()} file matching {glob_pattern}")

        return package_paths[0]

    def _calculate_package_sizes(
        self, ctx: Context, config: QualityGateConfig, artifact_paths: dict
    ) -> tuple[int, int]:
        """
        Calculate package sizes for wire and disk.

        Returns:
            Tuple of (wire_size, disk_size) in bytes
        """
        if "msi" in config.gate_name:
            # MSI special case: extract ZIP file for disk size, measure MSI file for wire size
            with tempfile.TemporaryDirectory() as extract_dir:
                extract_package(
                    ctx=ctx, package_os=config.os, package_path=artifact_paths['zip'], extract_dir=extract_dir
                )
                wire_size = file_size(path=artifact_paths['msi'])
                disk_size = directory_size(path=extract_dir)
        else:
            # Standard package handling
            primary_path = artifact_paths['primary']
            with tempfile.TemporaryDirectory() as extract_dir:
                extract_package(ctx=ctx, package_os=config.os, package_path=primary_path, extract_dir=extract_dir)
                wire_size = file_size(path=primary_path)
                disk_size = directory_size(path=extract_dir)

        return wire_size, disk_size


class DockerArtifactMeasurer:
    """
    Measures Docker image artifacts.
    """

    def measure(self, ctx: Context, config: QualityGateConfig) -> ArtifactMeasurement:
        """Measure Docker image sizes"""
        try:
            image_url = self._get_image_url(config)
            wire_size = self._calculate_image_wire_size(ctx, image_url)
            disk_size = self._calculate_image_disk_size(ctx, image_url)

            return ArtifactMeasurement(artifact_path=image_url, on_wire_size=wire_size, on_disk_size=disk_size)
        except (StaticQualityGateError, InfraError):
            raise
        except Exception as e:
            raise StaticQualityGateError(f"Failed to measure in {config.gate_name}: {str(e)}") from e

    def _get_image_url(self, config: QualityGateConfig) -> str:
        """
        Generate Docker image URL based on gate configuration.
        """
        # Extract flavor from gate name
        if "cluster" in config.gate_name:
            flavor = "cluster-agent"
        elif "dogstatsd" in config.gate_name:
            flavor = "dogstatsd"
        elif "cws_instrumentation" in config.gate_name:
            flavor = "cws-instrumentation"
        elif "agent" in config.gate_name:
            flavor = "agent"
        else:
            raise ValueError(f"Unknown docker image flavor for gate: {config.gate_name}")

        # Handle JMX suffix
        jmx = "-jmx" if "jmx" in config.gate_name else ""

        # Handle image suffix
        image_suffix = ("-7" if flavor == "agent" else "") + jmx

        # Handle nightly builds
        if os.environ.get("BUCKET_BRANCH") == "nightly" and flavor != "dogstatsd":
            flavor += "-nightly"

        # Validate CI environment
        pipeline_id = os.environ.get("CI_PIPELINE_ID")
        commit_sha = os.environ.get("CI_COMMIT_SHORT_SHA")

        if not pipeline_id or not commit_sha:
            raise StaticQualityGateError(
                "This gate needs to be run from the CI environment. (Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA)"
            )

        return f"registry.ddbuild.io/ci/datadog-agent/{flavor}:v{pipeline_id}-{commit_sha}{image_suffix}-{config.arch}"

    def _calculate_image_wire_size(self, ctx: Context, image_url: str) -> int:
        """Calculate Docker image compressed size using manifest inspection"""
        manifest_output = ctx.run(
            "DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect -v "
            + image_url
            + " | grep size | awk -F ':' '{sum+=$NF} END {printf(\"%d\",sum)}'",
            hide=True,
        )

        return int(manifest_output.stdout)

    def _calculate_image_disk_size(self, ctx: Context, image_url: str) -> int:
        """Calculate Docker image uncompressed size by pulling and extracting"""
        # Pull image locally to get on disk size
        crane_output = ctx.run(f"crane pull {image_url} output.tar", warn=True)
        if crane_output.exited != 0:
            raise InfraError(f"Crane pull failed to retrieve {image_url}. Retrying... (infra flake)")

        # Extract and calculate uncompressed size
        ctx.run("tar -xf output.tar")
        image_content = ctx.run("tar -tvf output.tar | awk -F' ' '{print $3; print $6}'", hide=True).stdout.splitlines()

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
                on_disk_size += int(ctx.run(f"tar -xf {image} --to-stdout | wc -c", hide=True).stdout)
        else:
            print(color_message("[WARN] No tar.gz file found inside of the image", "orange"))

        return on_disk_size


class StaticQualityGate:
    """
    Static Quality Gate comprises of a configuration that is read
    from yaml file and a measurer based on the gate type.
    Right now, we support two types of measurers:
    - PackageArtifactMeasurer: for package artifacts (DEB, RPM, MSI, etc.)
    - DockerArtifactMeasurer: for Docker images
    """

    def __init__(self, config: QualityGateConfig, measurer: ArtifactMeasurer):
        """
        Initialize quality gate with configuration and measurement strategy.

        Args:
            config: Gate configuration
            measurer: Strategy for measuring artifacts
        """
        self.config = config
        self.measurer = measurer

    def execute_gate(self, ctx: Context) -> GateResult:
        """
        Execute the quality gate.

        Args:
            ctx: Invoke context

        Returns:
            GateResult with all execution information

        Raises:
            StaticQualityGateFailed: If measurement fails or limits are exceeded
            InfraError: If there's an infrastructure issue (retryable)
        """
        print(color_message(f"Starting {self.config.gate_name}...", "cyan"))
        measurement = self.measurer.measure(ctx, self.config)

        violations = self._check_size_limits(measurement)

        result = GateResult(
            config=self.config, measurement=measurement, violations=violations, success=len(violations) == 0
        )

        print(
            color_message(
                f"Result for {self.config.gate_name}: {'PASSED' if result.success else 'FAILED'}",
                "green" if result.success else "red",
            )
        )
        print(
            f"Artifact: {measurement.artifact_path if 'ddbuild' in measurement.artifact_path else measurement.artifact_path.split('/')[-1]}"
        )
        print(
            color_message(
                f"On wire size: {measurement.on_wire_size / 1024 / 1024:.1f} MB", "green" if result.success else "red"
            )
        )
        print(
            color_message(
                f"On disk size: {measurement.on_disk_size / 1024 / 1024:.1f} MB", "green" if result.success else "red"
            )
        )
        print(
            color_message(
                f"Remaining on wire quota: {result.wire_remaining_bytes / 1024 / 1024:.1f} MB",
                "green" if result.success else "red",
            )
        )
        print(
            color_message(
                f"Remaining on disk quota: {result.disk_remaining_bytes / 1024 / 1024:.1f} MB",
                "green" if result.success else "red",
            )
        )

        # To outline the end of the gate execution
        print("+" * 40)
        return result

    def _check_size_limits(self, measurement: ArtifactMeasurement) -> list[SizeViolation]:
        """
        Check measurement against configured limits.

        Args:
            measurement: Artifact measurement results

        Returns:
            List of violations (empty if all checks pass)
        """
        violations = []

        if measurement.on_wire_size > self.config.max_on_wire_size:
            violations.append(
                SizeViolation(
                    measurement_type="wire",
                    current_size=measurement.on_wire_size,
                    max_size=self.config.max_on_wire_size,
                )
            )

        if measurement.on_disk_size > self.config.max_on_disk_size:
            violations.append(
                SizeViolation(
                    measurement_type="disk",
                    current_size=measurement.on_disk_size,
                    max_size=self.config.max_on_disk_size,
                )
            )

        return violations


class QualityGateFactory:
    """
    Factory for creating quality gates with appropriate measurement strategies.

    This factory selects the correct measurement strategy based on gate configuration.
    """

    @staticmethod
    def create_gate(gate_name: str, gate_max_size_values: dict) -> StaticQualityGate:
        """
        Create a quality gate with the appropriate measurement strategy.

        Args:
            gate_name: Technical gate name (e.g., "static_quality_gate_agent_deb_amd64")
            gate_max_size_values: Dictionary with max size configuration

        Returns:
            StaticQualityGate instance with injected measurer

        Raises:
            ValueError: If gate type cannot be determined
            UnsupportedOperation: If gate type is not supported
        """
        # Create immutable configuration
        config = create_quality_gate_config(gate_name, gate_max_size_values)

        # Select appropriate measurement strategy
        measurer = QualityGateFactory._create_measurer(gate_name)

        # Return composed quality gate (no inheritance)
        return StaticQualityGate(config, measurer)

    @staticmethod
    def _create_measurer(gate_name: str) -> ArtifactMeasurer:
        """
        Create the appropriate artifact measurer based on gate name.

        Args:
            gate_name: Technical gate name

        Returns:
            ArtifactMeasurer instance for the gate type

        Raises:
            UnsupportedOperation: If gate type is not supported
        """
        if "docker" in gate_name:
            return DockerArtifactMeasurer()
        elif any(package_type in gate_name for package_type in ["deb", "rpm", "heroku", "suse", "msi"]):
            return PackageArtifactMeasurer()
        else:
            raise UnsupportedOperation(f"Unknown gate type: {gate_name}")

    @staticmethod
    def create_gates_from_config(config_path: str) -> list[StaticQualityGate]:
        """
        Create all quality gates from a configuration file.

        Args:
            config_path: Path to YAML configuration file

        Returns:
            List of configured quality gates

        Raises:
            FileNotFoundError: If config file doesn't exist
            yaml.YAMLError: If config file is malformed
            ValueError: If any gate configuration is invalid
        """
        with open(config_path) as file:
            config = yaml.safe_load(file)

        gates = []
        for gate_name in config:
            gate = QualityGateFactory.create_gate(gate_name, config[gate_name])
            gates.append(gate)

        return gates


def create_quality_gate_config(gate_name: str, gate_max_size_values: dict) -> QualityGateConfig:
    """
    Create quality gate configuration from gate definition.

    Args:
        gate_name: Technical gate name
        gate_max_size_values: Dictionary with max size configuration

    Returns:
        Validated QualityGateConfig instance
    """
    return QualityGateConfig(
        gate_name=gate_name,
        max_on_wire_size=read_byte_input(gate_max_size_values["max_on_wire_size"]),
        max_on_disk_size=read_byte_input(gate_max_size_values["max_on_disk_size"]),
        arch=_extract_arch_from_gate_name(gate_name),
        os=_extract_os_from_gate_name(gate_name),
    )


def _extract_arch_from_gate_name(gate_name: str) -> str:
    """Extract architecture from gate name"""
    if "amd64" in gate_name:
        return "amd64"
    elif "arm64" in gate_name:
        return "arm64"
    elif "armhf" in gate_name:
        return "armhf"
    elif "msi" in gate_name:
        # MSI packages are always amd64 (x86_64) on Windows
        return "amd64"
    else:
        raise ValueError(f"Unknown arch for gate: {gate_name}")


def _extract_os_from_gate_name(gate_name: str) -> str:
    """Extract OS from gate name"""
    if "docker" in gate_name:
        return "docker"

    # Check package types
    for package_type, os_name in PACKAGE_OS_MAPPING.items():
        if package_type in gate_name:
            return os_name

    raise ValueError(f"Unknown OS for gate: {gate_name}")


class GateMetricHandler:
    # All metric_name -> metric_key
    METRICS_DICT = {
        "datadog.agent.static_quality_gate.on_wire_size": "current_on_wire_size",
        "datadog.agent.static_quality_gate.on_disk_size": "current_on_disk_size",
        "datadog.agent.static_quality_gate.max_allowed_on_wire_size": "max_on_wire_size",
        "datadog.agent.static_quality_gate.max_allowed_on_disk_size": "max_on_disk_size",
        # Delta metrics (relative to ancestor)
        "datadog.agent.static_quality_gate.relative_on_wire_size": "relative_on_wire_size",
        "datadog.agent.static_quality_gate.relative_on_disk_size": "relative_on_disk_size",
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

    def _should_skip_send_metrics(self) -> bool:
        """
        Check if we should skip sending SQG metrics to Datadog.

        On main branch, we only want to send metrics for push pipelines
        (not for manually triggered, downstream, or scheduled pipelines).

        This is to avoid sending metrics for pipelines that override
        integrations-core version that leads to inconsistent metrics.

        Returns:
            True if metrics should be skipped, False otherwise.
        """
        branch = os.getenv("CI_COMMIT_BRANCH", "")
        pipeline_source = os.getenv("CI_PIPELINE_SOURCE", "")

        # On main branch, only allow push pipelines to send metrics
        if branch == "main" and pipeline_source != "push":
            return True

        return False

    def _add_gauge(self, timestamp, common_tags, gate, metric_name, metric_key):
        metric_value = self.metrics[gate].get(metric_key)
        if metric_value is not None:
            return create_gauge(
                metric_name,
                timestamp,
                metric_value,
                tags=common_tags,
                metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
                unit="byte",
            )
        return None

    def generate_relative_size(self, ancestor=None):
        """
        Calculate relative sizes by querying Datadog for ancestor metrics.

        Args:
            ancestor: The ancestor commit SHA to compare against
        """
        import time

        from tasks.libs.common.datadog_api import query_gate_metrics_for_commit

        if not ancestor:
            print(color_message("[WARN] Unable to find this commit ancestor", "orange"))
            return

        # Query Datadog once for all gates
        ancestor_metrics = query_gate_metrics_for_commit(ancestor)

        # Retry once after delay if no metrics found (race condition with ancestor job)
        if not ancestor_metrics:
            print(
                color_message(
                    "[INFO] No ancestor metrics found, waiting 3 minutes for metrics to be available...",
                    "blue",
                )
            )
            time.sleep(180)  # 3 minutes
            ancestor_metrics = query_gate_metrics_for_commit(ancestor)

        datadog_gates_found = 0
        for gate in self.metrics:
            ancestor_gate = ancestor_metrics.get(gate)

            if ancestor_gate:
                datadog_gates_found += 1
                # Calculate relative sizes using Datadog data
                for metric_key in ["current_on_wire_size", "current_on_disk_size"]:
                    current_value = self.metrics[gate].get(metric_key)
                    ancestor_value = ancestor_gate.get(metric_key)
                    if current_value is not None and ancestor_value is not None:
                        relative_metric_size = current_value - ancestor_value
                        self.register_metric(gate, metric_key.replace("current", "relative"), relative_metric_size)

        if datadog_gates_found == 0:
            print(
                color_message(
                    f"[WARN] No Datadog metrics found for ancestor {ancestor}",
                    "orange",
                )
            )
        else:
            print(
                color_message(
                    f"[INFO] Successfully fetched ancestor metrics from Datadog for {datadog_gates_found} gate(s)",
                    "green",
                )
            )

    def _generate_series(self):
        """Generate metric series for sending to Datadog."""
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
        """Send all metrics to Datadog (backward compatible)."""
        if self._should_skip_send_metrics():
            branch = os.getenv("CI_COMMIT_BRANCH", "")
            source = os.getenv("CI_PIPELINE_SOURCE", "")
            print(color_message(f"[INFO] Skipping SQG metrics: branch={branch}, pipeline_source={source}", "blue"))
            return

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
        # Store reports for main and release branches to enable delta calculation for backport PRs
        if not is_nightly and (branch == "main" or is_a_release_branch(ctx, branch)) and CI_COMMIT_SHA:
            ctx.run(
                f"aws s3 cp --only-show-errors --region us-east-1 --sse AES256 {filename} {self.S3_REPORT_PATH}/{CI_COMMIT_SHA}/{filename}",
                hide="stdout",
            )
