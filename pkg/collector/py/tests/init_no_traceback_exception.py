# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

import aggregator
from checks import AgentCheck

class TestCheck(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances, one, two, three): # __init__ with many arguments
        pass

    def check(self, instance):
        pass
