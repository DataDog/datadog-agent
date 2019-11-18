# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

# NOTE: The files in this folder are needed for the legacy import path
# (from checks import AgentCheck) to work.
# As long as we do not properly deprecate the legacy import path, they
# should not be removed. For more details, see #4032 and #4421.

from datadog_checks.checks import AgentCheck
from datadog_checks.errors import CheckException
