import glob
import json
import math
import os
import types
from datetime import datetime
from types import SimpleNamespace

from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.datadog_api import create_gauge, send_metrics
from tasks.libs.common.utils import get_metric_origin


def argument_extractor(entry_args, **kwargs) -> SimpleNamespace:
    """
    Allow clean extraction of arguments from parsed quality gates, also allows to execute pre-process function on kwargs

    :param entry_args: Dictionary containing parsed arguments from a static quality gate
    :param kwargs: Dictionary containing arguments that we want to extract (optionally pre-process function to apply as values)
    :return: SimpleNamespace with extracted arguments as attributes
    """
    for key in kwargs.keys():
        if isinstance(kwargs[key], types.FunctionType):
            kwargs[key] = kwargs[key](entry_args[key])
        else:
            kwargs[key] = entry_args[key]
    return SimpleNamespace(**kwargs)


def byte_to_string(size):
    if not size:
        return "0 B"
    size_name = ("B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB")
    i = int(math.log(size, 1024))
    p = math.pow(1024, i)
    s = round(size / p, 2)
    return f"{s} {size_name[i]}"


def string_to_byte(size: str):
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
        return int(size.replace("B", ""))
    else:
        return int(size)


def read_byte_input(byte_input):
    if isinstance(byte_input, str):
        return string_to_byte(byte_input)
    else:
        return byte_input


def find_package_path(flavor, package_os, arch, extension=None):
    package_dir = os.environ['OMNIBUS_PACKAGE_DIR']
    separator = '_' if package_os == 'debian' else '-'
    if not extension:
        extension = "deb" if package_os == 'debian' else "rpm"
    glob_pattern = f'{package_dir}/{flavor}{separator}7*{arch}.{extension}'
    package_paths = glob.glob(glob_pattern)
    if len(package_paths) > 1:
        raise Exit(code=1, message=color_message(f"Too many files matching {glob_pattern}: {package_paths}", "red"))
    elif len(package_paths) == 0:
        raise Exit(code=1, message=color_message(f"Couldn't find any file matching {glob_pattern}", "red"))
    return package_paths[0]


class GateMetricHandler:
    # All metric_name -> metric_key
    METRICS_DICT = {
        "datadog.agent.static_quality_gate.on_wire_size": "current_on_wire_size",
        "datadog.agent.static_quality_gate.on_disk_size": "current_on_disk_size",
        "datadog.agent.static_quality_gate.max_allowed_on_wire_size": "max_on_wire_size",
        "datadog.agent.static_quality_gate.max_allowed_on_disk_size": "max_on_disk_size",
    }

    def __init__(self, git_ref, bucket_branch, filename=None):
        self.metrics = {}
        self.metadata = {}
        self.git_ref = git_ref
        self.bucket_branch = bucket_branch
        self.series_is_complete = True

        if filename is not None:
            self._load_metrics_report(filename)

    def get_formatted_metric(self, gate_name, metric_name):
        return byte_to_string(self.metrics[gate_name][metric_name])

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

    def generate_metric_reports(self, ctx, filename="static_gate_report.json", branch=None):
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
        if branch == "main" and CI_COMMIT_SHA:
            ctx.run(
                f"aws s3 cp --only-show-errors --region us-east-1 --sse AES256 {filename} s3://dd-ci-artefacts-build-stable/datadog-agent/static_quality_gates/{CI_COMMIT_SHA}/{filename}",
                hide="stdout",
            )
