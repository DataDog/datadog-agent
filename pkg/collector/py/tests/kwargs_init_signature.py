# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

from checks import AgentCheck
from common import assert_init_config_init, assert_agent_config_init, assert_instance_init


class TestCheck(AgentCheck):
    def __init__(self, *args, **kwargs):
        super(TestCheck, self).__init__(*args, **kwargs)

        assert_init_config_init(self)
        assert_agent_config_init(self, True)
        assert_instance_init(self)

    def check(self, instance):
        pass
