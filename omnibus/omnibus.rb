# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

#
# This file is used to configure the datadog-agent project. It contains
# some minimal configuration examples for working with Omnibus. For a full list
# of configurable options, please see the documentation for +omnibus/config.rb+.
#

# Windows architecture defaults
# ------------------------------
windows_arch :x86_64
# Don't append a timestamp to the package version
append_timestamp false
