# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

# pylint: disable=E0401

"""
A lightweight Python WMI module wrapper built on top of `pywin32` and `win32com` extensions.

**Specifications**
* Based on top of the `pywin32` and `win32com` third party extensions only
* Compatible with `Raw`* and `Formatted` Performance Data classes
    * Dynamically resolve properties' counter types
    * Hold the previous/current `Raw` samples to compute/format new values*
* Fast and lightweight
    * Avoid queries overhead
    * Cache connections and qualifiers
    * Use `wbemFlagForwardOnly` flag to improve enumeration/memory performance

*\\* `Raw` data formatting relies on the avaibility of the corresponding calculator.
Please refer to `checks.lib.wmi.counter_type` for more information*

Original discussion thread: https://github.com/DataDog/dd-agent/issues/1952
Credits to @TheCloudlessSky (https://github.com/TheCloudlessSky)
"""
from stackstate_checks.checks.win.wmi.sampler import WMISampler
