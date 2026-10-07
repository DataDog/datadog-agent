import os
import pathlib
import sys
import time

state_dir = pathlib.Path(sys.argv[1])
state_dir.mkdir(parents=True, exist_ok=True)
(state_dir / "parent.pid").write_text(str(os.getpid()), encoding="utf-8")
while not (state_dir / "trigger").exists():
    time.sleep(0.1)
child_pid = os.fork()
if child_pid == 0:
    os._exit(0)
(state_dir / "child.pid").write_text(str(child_pid), encoding="utf-8")
while True:
    time.sleep(1)
