# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

name "systemd"
default_version "238"

source :url => "https://github.com/systemd/systemd/archive/v#{version}.tar.gz",
       :sha256 => "c8982a691f70ff6a4c0782bbe9e7ca5bcc5e1982e8f6232d91634e1b469a39d9"

relative_path "systemd-#{version}"

