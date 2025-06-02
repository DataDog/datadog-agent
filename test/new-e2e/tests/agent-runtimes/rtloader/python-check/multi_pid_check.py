import os
from multiprocessing import Process, Queue

from datadog_checks.base import AgentCheck  # type: ignore


def collect_pid(queue: Queue):
    """
    Collects the current process ID and puts it in a multiprocessing queue.
    """
    pid = os.getpid()
    queue.put(pid)


class MultiPIDCheck(AgentCheck):
    def check(self, instance):
        """
        Spawns 3 processes that queue their PIDs to the main process that will emit them as metrics.
        """
        processes = []
        pid_queue = Queue()

        for idx in range(3):  # noqa: B007
            p = Process(target=collect_pid, args=(pid_queue,))
            processes.append(p)
            p.start()

        for p in processes:
            p.join()

        # Collect and emit PID metrics
        for idx in range(3):
            try:
                pid = pid_queue.get_nowait()
                self.gauge("multi_pid_check.process.pid", pid, tags=[f"process:index_{idx+1}"])
            except Exception as e:
                self.log.error(f"Failed to get PID from queue: {str(e)}")  # noqa: G004

        self.log.info("MultiPIDCheck completed emitting all process PIDs.")
