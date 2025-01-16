import glob
import os
from types import SimpleNamespace
from datetime import datetime
from tasks.libs.common.datadog_api import create_gauge
from tasks.libs.common.constants import ORIGIN_CATEGORY, ORIGIN_PRODUCT, ORIGIN_SERVICE
from tasks.libs.common.utils import get_metric_origin

from invoke.exceptions import Exit

from tasks.libs.common.color import color_message


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

    def register_gate_tags(self, gate_name, **kwargs):
        for key in kwargs:
            self.metadata[gate_name][key] = kwargs[key]

    def _generate_series(self):
        series = []
        timestamp = int(datetime.utcnow().timestamp())
        common_tags = [
            f"os:{package_os}",
            f"package:datadog-{flavor}",
            f"git_ref:{self.git_ref}",
            f"bucket_branch:{self.bucket_branch}",
            f"arch:{arch}",
        ]
        series.append(
            create_gauge(
                "datadog.agent.compressed_package.size",
                timestamp,
                package_compressed_size,
                tags=common_tags,
                metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
            ),
        )
        series.append(
            create_gauge(
                "datadog.agent.package.size",
                timestamp,
                package_uncompressed_size,
                tags=common_tags,
                metric_origin=get_metric_origin(ORIGIN_PRODUCT, ORIGIN_CATEGORY, ORIGIN_SERVICE),
            ),
        )

    def send_metrics(self):
        pass
