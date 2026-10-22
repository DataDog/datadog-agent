"""Exercise an installed Core Agent's startup, Python, Cgo FFI, and shutdown.

Run inside the host-package or container test environment. The installed Agent
must include its matching embedded Python and integrations. No service config
is modified; all test configuration and logs live in a temporary directory.
"""

import argparse
import os
import shutil
import socket
import subprocess
import tempfile
import time
from pathlib import Path


def available_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def component_smoke(binary, root, env, address, family=socket.AF_INET):
    with (root / (Path(binary).name + ".log")).open("w+") as log:
        process = subprocess.Popen([binary, "run", "-c", str(root)], env=env, stdout=log, stderr=log)
        ready = False
        try:
            deadline = time.monotonic() + 60
            while process.poll() is None and time.monotonic() < deadline:
                with socket.socket(family) as sock:
                    sock.settimeout(0.2)
                    ready = sock.connect_ex(address) == 0
                if ready:
                    break
                time.sleep(0.5)
        finally:
            process.terminate()
            try:
                process.wait(timeout=60)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        log.seek(0)
        assert ready and process.returncode == 0, log.read()
        print(f"PASS {binary}: listener startup and graceful shutdown")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent", required=True)
    parser.add_argument("--library", required=True)
    parser.add_argument("--nosys")
    parser.add_argument("--trace-agent")
    parser.add_argument("--system-probe")
    parser.add_argument("--integration", action="append", default=[])
    args = parser.parse_args()
    agent = str(Path(args.agent).resolve())
    env = dict(os.environ)
    env.pop("MALLOC_CONF", None)
    env.pop("LD_PRELOAD", None)
    if args.nosys:
        env["LD_PRELOAD"] = str(Path(args.nosys).resolve())

    with tempfile.TemporaryDirectory(prefix="allocator-installed-") as directory:
        root = Path(directory)
        checks, confd = root / "checks", root / "conf.d"
        checks.mkdir()
        confd.mkdir()
        shutil.copy2(Path(__file__).with_name("allocator_check.py"), checks)
        library = checks / "libdatadog-agent-allocator_ffi.so"
        shutil.copy2(args.library, library)
        library.chmod(0o750)  # Match the real shared-library loader's restrictions.
        env["DD_ALLOCATOR_TEST_LIBRARY"] = str(library)
        names = ["allocator_check", "allocator_ffi", *args.integration]
        for name in names:
            config = confd / f"{name}.d"
            config.mkdir()
            (config / "conf.yaml").write_text("instances: [{}]\n")
        port, trace_port = available_port(), available_port()
        (root / "datadog.yaml").write_text(
            "api_key: '00000000000000000000000000000000'\n"
            "hostname: allocator-validation\n"
            "dd_url: http://127.0.0.1:9\n"
            "log_level: warn\n"
            f"log_file: {root / 'agent.log'}\n"
            f"cmd_port: {port}\n"
            f"confd_path: {confd}\n"
            f"additional_checksd: {checks}\n"
            "apm_config:\n  apm_dd_url: http://127.0.0.1:9\n"
            f"  receiver_port: {trace_port}\n"
            "telemetry:\n  enabled: true\n  python_memory: true\n"
            "shared_library_check:\n  enabled: true\n"
            f"  library_folder_path: {checks}\n"
        )
        (root / "system-probe.yaml").write_text(
            f"discovery:\n  enabled: true\nsystem_probe_config:\n  sysprobe_socket: {root / 'sysprobe.sock'}\n"
        )
        with (root / "startup.log").open("w+") as log:
            process = subprocess.Popen(
                [agent, "run", "-c", str(root), "--sysprobecfgpath", str(root)], env=env, stdout=log, stderr=log
            )
            try:
                deadline = time.monotonic() + 60
                while time.monotonic() < deadline:
                    assert process.poll() is None, "Agent exited during startup"
                    status = subprocess.run(
                        [agent, "status", "-c", str(root)], env=env, capture_output=True, timeout=15
                    )
                    if status.returncode == 0:
                        break
                    time.sleep(0.5)
                else:
                    raise AssertionError("Agent status never became available")
                print("PASS installed Agent startup and status")
                # These components consume the Core Agent's configuration
                # stream and IPC credentials, so keep it running for them.
                if args.trace_agent:
                    component_smoke(args.trace_agent, root, env, ("127.0.0.1", trace_port))
                if args.system_probe:
                    component_smoke(args.system_probe, root, env, str(root / "sysprobe.sock"), socket.AF_UNIX)
                for name in names:
                    result = subprocess.run(
                        [agent, "check", name, "-c", str(root)],
                        env=env,
                        text=True,
                        capture_output=True,
                        timeout=60,
                    )
                    assert result.returncode == 0, result.stdout + result.stderr
                    if name in {"allocator_check", "allocator_ffi"}:
                        metric = "allocator.binding.success" if name == "allocator_check" else "allocator.ffi.success"
                        assert metric in result.stdout, result.stdout + result.stderr
                    print(f"PASS installed Agent check {name}")
            finally:
                process.terminate()
                try:
                    process.wait(timeout=60)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                    raise AssertionError("Agent failed to stop after SIGTERM") from None
                log.seek(0)
                if process.returncode != 0:
                    raise AssertionError(log.read())
            print("PASS installed Agent graceful shutdown")


if __name__ == "__main__":
    main()
