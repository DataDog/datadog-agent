import os
import tempfile
import time
from multiprocessing import Process

from datadog_checks.base import AgentCheck  # type: ignore


def write_to_file(file_path, data, write_count=5, delay=1):
    """
    Writes the specified data to the given file multiple times with delays,
    printing process information to demonstrate concurrent execution.
    """
    start_time = time.time()
    print(f"[Process {os.getpid()}] Starting to write to {file_path} at {time.strftime('%X')}")
    try:
        with open(file_path, 'a') as f:
            for i in range(write_count):
                message = (
                    f"[Process {os.getpid()}] Finished writing to {file_path} at {time.strftime('%X')} at line {i + 1}"
                )
                f.write(message + "\n")
                f.flush()
                time.sleep(delay)
        elapsed = time.time() - start_time
        print(
            f"[Process {os.getpid()}] Finished writing to {file_path} at {time.strftime('%X')} (Elapsed: {elapsed:.2f} seconds)"
        )
    except Exception as e:
        print(f"[Process {os.getpid()}] Error writing to {file_path}: {str(e)}")


class MultiFileCheck(AgentCheck):
    def check(self, instance):
        """
        Spawns 3 processes that concurrently write to 3 different files.
        The files are created in the system's temporary directory.
        """
        temp_dir = os.path.join(tempfile.gettempdir(), "multi_file_check")
        os.makedirs(temp_dir, exist_ok=True)

        file_paths = [
            os.path.join(temp_dir, "file1.txt"),
            os.path.join(temp_dir, "file2.txt"),
            os.path.join(temp_dir, "file3.txt"),
        ]

        processes = []
        for idx, file_path in enumerate(file_paths, start=1):
            data = f"Datadog Check Data for File {idx}"
            p = Process(target=write_to_file, args=(file_path, data))
            processes.append(p)
            p.start()

        for p in processes:
            p.join()

        self.log.info("MultiFileCheck completed writing to all files.")
