"""Definitions and utilities for configuring the Agent build.
"""

ALL_BUILD_TAGS = [
    # Full list of known tags borrowed from /tasks/build_tags.py
    "bundle_installer",
    "clusterchecks",
    "consul",
    "containerd",
    "cri",
    "crio",
    "datadog.no_waf",
    "docker",
    "ec2",
    "etcd",
    "fargateprocess",
    "goexperiment.systemcrypto",  # used for FIPS mode
    "grpcnotrace",  # used to disable gRPC tracing
    "jetson",
    "jmx",
    "kubeapiserver",
    "kubelet",
    # Deferring the linux_bpf tag until we build system-probe, since it requires changing tags in files to make it work
    #"linux_bpf",
    "netcgo",  # Force the use of the CGO resolver. This will also have the effect of making the binary non-static
    "netgo",
    "npm",
    "nvml",  # used for the nvidia go-nvml library
    "no_dynamic_plugins",
    "oracle",
    "orchestrator",
    "osusergo",
    "otlp",
    "pcap",  # used by system-probe to compile packet filters using google/gopacket/pcap, which requires cgo to link libpcap
    "podman",
    "python",
    "requirefips",  # used for Linux FIPS mode to avoid having to set GOFIPS
    "sds",
    "serverless",
    "serverlessfips",  # used for FIPS mode in the serverless build in datadog-lambda-extension
    "systemd",
    "test",
    "trivy",
    "wmi",
    "zk",
    "zlib",
    "zstd",
]

REPO_PATH = "github.com/DataDog/datadog-agent"

def with_repo_path(mapping, repo_path=REPO_PATH):
    """Create a copy of a mapping with a path to repo"""
    return {repo_path + k: v for k, v in mapping.items()}
