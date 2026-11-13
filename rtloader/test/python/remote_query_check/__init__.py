# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

import threading

from datadog_checks.base.checks import AgentCheck

reply = '{"status":"SUCCEEDED"}'
fail = False
blocking = False
received = []
cancelled = []
entered = threading.Event()
released = threading.Event()


class RemoteQueryCheck(AgentCheck):
    def remote_query(self, request):
        received.append(request)
        if fail:
            raise RuntimeError("secret SQL and credentials must not escape")
        if blocking:
            entered.set()
            released.wait(5)
        return reply

    def cancel_remote_query(self, crawl_id):
        cancelled.append(crawl_id)
        released.set()

    def cancel(self):
        released.set()

    def run(self):
        return ""
