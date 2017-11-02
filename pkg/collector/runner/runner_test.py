# Python module for simulating a simple python check

from checks import AgentCheck
import time;

class TestCheck(AgentCheck):
    def check(self, instance):
        # Busy wait for 100ms
        current_time = time.time()
        while (time.time() < current_time+0.1):
            pass
