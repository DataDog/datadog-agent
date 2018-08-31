# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

import logging

from checks import AgentCheck
import _util

log = logging.getLogger(__name__)


class TestSubprocessCheck(AgentCheck):
    def check(self, instance):
        """
        Do not interact with the Aggregator during
        unit tests. Doing anything is ok here.
        """
        _util.get_subprocess_output(['sleep', '10'], log)
