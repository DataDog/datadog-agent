# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

from datetime import datetime
from checks import AgentCheck
import stackstate

# [BS] TODO: move this to a separate repo
class StackStateAgentCheck(AgentCheck):

    def get_instance_key(self):
        raise NotImplementedException("get_instance_key() should be overridden when topology is used")

    def component(self, id, type, data):
        # [BS] TODO: Catch invalid parameters (input sanitization) (e.g. types of parameters, of instance_key)
        if data is None:
            data = {}
        stackstate.submit_component(self, self.check_id, self.get_instance_key(), id, type, data)

    def relation(self, source, target, type, data):
        # [BS] TODO: Catch invalid parameters (input sanitization) (e.g. types of parameters, of instance_key)
        if data is None:
            data = {}
        stackstate.submit_relation(self, self.check_id, self.get_instance_key(), source, target, type, data)

    def start_snapshot(self):
        # [BS] TODO: Catch invalid parameters (input sanitization) (e.g. instance_key)
        stackstate.submit_start_snapshot(self, self.check_id, self.get_instance_key())

    def stop_snapshot(self):
        # [BS] TODO: Catch invalid parameters (input sanitization) (e.g. instance_key)
        stackstate.submit_stop_snapshot(self, self.check_id, self.get_instance_key())

class TestComponentCheck(StackStateAgentCheck):
    def get_instance_key(self):
        return { "type": "type", "url": "url" }

    def check(self, instance):
        self.component("myid", "mytype", { "key": "value", "intlist": [1], "emptykey": None, "nestedobject": { "nestedkey": "nestedValue" }})

class TestRelationCheck(StackStateAgentCheck):
    def get_instance_key(self):
        return { "type": "type", "url": "url" }

    def check(self, instance):
        self.relation("source", "target", "mytype", { "key": "value", "intlist": [1], "emptykey": None, "nestedobject": { "nestedkey": "nestedValue" }})

class TestStartSnapshotCheck(StackStateAgentCheck):
    def get_instance_key(self):
        return { "type": "type", "url": "url" }

    def check(self, instance):
        self.start_snapshot()

class TestStopSnapshotCheck(StackStateAgentCheck):
    def get_instance_key(self):
        return { "type": "type", "url": "url" }

    def check(self, instance):
        self.stop_snapshot()
