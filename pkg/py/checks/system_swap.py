# 3p
import psutil

# project
from checks import AgentCheck


class SystemSwap(AgentCheck):

    def check(self, instance):
        swap_mem = psutil.swap_memory()
        self.rate('system.swap.swapped_in', swap_mem.sin)
        self.rate('system.swap.swapped_out', swap_mem.sout)
