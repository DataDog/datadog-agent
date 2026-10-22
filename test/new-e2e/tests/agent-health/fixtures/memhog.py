import os
import time


def working_set():
    usage = int(open("/sys/fs/cgroup/memory.current").read())
    inactive = 0
    for line in open("/sys/fs/cgroup/memory.stat"):
        key, value = line.split()
        if key == "inactive_file":
            inactive = int(value)
    return usage - inactive


limit = int(open("/sys/fs/cgroup/memory.max").read())
chunks = []
while working_set() < 0.93 * limit:
    chunks.append(b"x" * (8 << 20))
with open("/tmp/memhog.pid", "w") as f:
    f.write(str(os.getpid()))
while True:
    time.sleep(3600)
