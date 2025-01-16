import glob
import os
from datetime import datetime
from types import SimpleNamespace

from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.datadog_api import create_gauge, send_metrics
from tasks.libs.common.utils import get_metric_origin


def argument_extractor(entry_args, **kwargs) -> SimpleNamespace:
    """
    Allow clean extraction of arguments from parsed quality gates

    :param entry_args: Dictionary containing parsed arguments from a static quality gate
    :param kwargs: Dictionary containing arguments that we want to extract
    :return: SimpleNamespace with extracted arguments as attributes
    """
    for key in kwargs.keys():
        kwargs[key] = entry_args[key]
    return SimpleNamespace(**kwargs)


def find_package_path(flavor, package_os, arch):
    package_dir = os.environ['OMNIBUS_PACKAGE_DIR']
    separator = '_' if package_os == 'debian' else '-'
    extension = "deb" if package_os == 'debian' else "rpm"
    glob_pattern = f'{package_dir}/{flavor}{separator}7*{arch}.{extension}'
    package_paths = glob.glob(glob_pattern)
    if len(package_paths) > 1:
        raise Exit(code=1, message=color_message(f"Too many files matching {glob_pattern}: {package_paths}", "red"))
    elif len(package_paths) == 0:
        raise Exit(code=1, message=color_message(f"Couldn't find any file matching {glob_pattern}", "red"))
    return package_paths[0]


class GateMetricHandler:
    def __init__(self, git_ref, bucket_branch):
        self.metrics = {}
        self.metadata = {}
        self.git_ref = git_ref
        self.bucket_branch = bucket_branch

    def register_metric(self, gate_name, metric_name, metric_value):
        if self.metrics.get(gate_name, None) is None:
            self.metrics[gate_name] = {}

        self.metrics[gate_name][metric_name] = metric_value

    def register_gate_tags(self, gate, **kwargs):
        for key in kwargs:
            self.metadata[gate][key] = kwargs[key]

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
            for tag in self.metadata[gate]:
                common_tags.append(f"{tag}:{self.metadata[gate][tag]}")

            series.append(
                create_gauge(
                    "datadog.agent.static_quality_gate.on_wire_size",
                    timestamp,
                    self.metrics[gate]["current_on_wire_size"],
                    tags=common_tags,
                    metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
                ),
            )
            series.append(
                create_gauge(
                    "datadog.agent.static_quality_gate.on_disk_size",
                    timestamp,
                    self.metrics[gate]["current_on_disk_size"],
                    tags=common_tags,
                    metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
                ),
            )
            series.append(
                create_gauge(
                    "datadog.agent.static_quality_gate.max_allowed_on_wire_size",
                    timestamp,
                    self.metrics[gate]["max_on_wire_size"],
                    tags=common_tags,
                    metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
                ),
            )
            series.append(
                create_gauge(
                    "datadog.agent.static_quality_gate.max_allowed_on_disk_size",
                    timestamp,
                    self.metrics[gate]["max_on_disk_size"],
                    tags=common_tags,
                    metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
                ),
            )
        return series

    def send_metrics(self):
        series = self._generate_series()

        print(color_message("Data collected:", "blue"))
        print(series)
        if series:
            print(color_message("Sending metrics to Datadog", "blue"))
            send_metrics(series=series)
            print(color_message("Done", "green"))
