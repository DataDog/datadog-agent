# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

"""Analysis tests for the libpcap package contents."""

load("@rules_pkg//pkg:providers.bzl", "PackageFilegroupInfo")
load("@rules_testing//lib:analysis_test.bzl", "analysis_test", "test_suite")

_PUBLIC_HEADER_DESTINATIONS = [
    "embedded/include/pcap.h",
    "embedded/include/pcap/bluetooth.h",
    "embedded/include/pcap/bpf.h",
    "embedded/include/pcap/can_socketcan.h",
    "embedded/include/pcap/compiler-tests.h",
    "embedded/include/pcap/dlt.h",
    "embedded/include/pcap/funcattrs.h",
    "embedded/include/pcap/ipnet.h",
    "embedded/include/pcap/namedb.h",
    "embedded/include/pcap/nflog.h",
    "embedded/include/pcap/pcap-inttypes.h",
    "embedded/include/pcap/pcap.h",
    "embedded/include/pcap/sll.h",
    "embedded/include/pcap/socket.h",
    "embedded/include/pcap/usb.h",
    "embedded/include/pcap/vlan.h",
]

def _test_all_files_contains_only_public_headers_impl(env, target):
    filegroup = target[PackageFilegroupInfo]
    destinations = []
    for files, _ in filegroup.pkg_files:
        destinations.extend(files.dest_src_map.keys())

    env.expect.that_collection(destinations).contains_exactly(_PUBLIC_HEADER_DESTINATIONS)
    env.expect.that_collection(filegroup.pkg_dirs).contains_exactly([])
    env.expect.that_collection(filegroup.pkg_symlinks).contains_exactly([])

def _test_all_files_contains_only_public_headers(name):
    analysis_test(
        name = name,
        impl = _test_all_files_contains_only_public_headers_impl,
        target = "@libpcap//:all_files",
    )

def libpcap_packaging_test_suite(name):
    """Tests the package-facing contents exported by libpcap."""
    test_suite(
        name = name,
        tests = [
            _test_all_files_contains_only_public_headers,
        ],
    )
