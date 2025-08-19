# Python module for simulating a simple python check

import time
from time import sleep

from checks import AgentCheck


class TestCheck(AgentCheck):
    def check(self, instance):
        test_inst = instance['test_instance']

        if test_inst['lazy_wait']:
            sleep(test_inst['wait_length'])

        else:
            current_time = time.time()
            while time.time() < current_time + test_inst['wait_length']:
                pass
