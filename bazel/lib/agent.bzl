"""Definitions and utilities for configuring the Agent build.
"""

AGENT_BUILD_TAGS = [
        # Full list of known tags borrowed from /tasks/build_tags.py
        "apm",
        "consul",
        "containerd",
        "cri",
        "datadog.no_waf",
        "docker",
        "ec2",
        "etcd",
        "gce",
        "jetson",
        "jmx",
        "kubeapiserver",
        "kubelet",
        "netcgo",  # Force the use of the CGO resolver. This will also have the effect of making the binary non-static
        "oracle",
        "orchestrator",
        "otlp",
        "podman",
        "process",
        "python",
        "systemd",
        "trivy",
        "zk",
        "zlib",
        "zstd",
]

REPO_PATH = "github.com/DataDog/datadog-agent"

def with_repo_path(mapping, repo_path=REPO_PATH):
    """Create a copy of a mapping with a path to repo"""
    return {repo_path + k: v for k, v in mapping.items()}
