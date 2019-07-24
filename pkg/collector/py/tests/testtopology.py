# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

from checks import AgentCheck, TopologyInstance

class TestComponentCheck(AgentCheck):
    def get_instance_key(self, instance):
        return TopologyInstance("type", "url")

    def check(self, instance):
        self.component("myid", "mytype", { "key": "value", "intlist": [1], "emptykey": None, "nestedobject": { "nestedkey": "nestedValue" }})

class TestRelationCheck(AgentCheck):
    def get_instance_key(self, instance):
        return TopologyInstance("type", "url")

    def check(self, instance):
        self.relation("source", "target", "mytype", { "key": "value", "intlist": [1], "emptykey": None, "nestedobject": { "nestedkey": "nestedValue" }})

class TestStartSnapshotCheck(AgentCheck):
    def get_instance_key(self, instance):
        return TopologyInstance("type", "url")

    def check(self, instance):
        self.start_snapshot()

class TestStopSnapshotCheck(AgentCheck):
    def get_instance_key(self, instance):
        return TopologyInstance("type", "url")

    def check(self, instance):
        self.stop_snapshot()
