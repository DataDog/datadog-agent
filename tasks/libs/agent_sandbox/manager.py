"""State and command helpers for the local Agent Sandbox MVP.

This module owns the repo-independent parts of the Stage A sandbox:
state layout, managed SSH metadata, Agent package provisioning script rendering,
and command construction. The macOS Virtualization.framework lifecycle is kept
behind a helper boundary so invoke tasks do not embed platform-specific VM code.
"""

from __future__ import annotations

import hashlib
import json
import os
import platform
import shlex
import shutil
import signal
import subprocess
import textwrap
import time
import urllib.request
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any

DEFAULT_STATE_ROOT = Path.home() / ".dd-agent-dev" / "sandbox"
DEFAULT_SANDBOX_NAME = "default"
DEFAULT_GUEST_USER = "ubuntu"
DEFAULT_AGENT_MAJOR_VERSION = "7"
DEFAULT_UBUNTU_IMAGE_URL = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-arm64.img"
DUMMY_API_KEY = "00000000000000000000000000000000"


class AgentSandboxError(RuntimeError):
    """Raised for expected user-facing sandbox failures."""


@dataclass(frozen=True)
class SandboxPaths:
    root: Path
    cache_dir: Path
    instances_dir: Path
    instance_dir: Path
    metadata_file: Path
    ssh_dir: Path
    private_key: Path
    public_key: Path
    provisioning_dir: Path
    host_install_script: Path
    cloud_init_dir: Path
    cloud_init_user_data: Path
    cloud_init_meta_data: Path
    seed_iso: Path
    vm_dir: Path
    disk_image: Path
    efi_variable_store: Path
    serial_log: Path
    apt_cache_dir: Path


@dataclass
class SandboxMetadata:
    name: str
    mode: str
    state: str
    guest_user: str
    mac_address: str | None = None
    ssh_host: str | None = None
    ssh_port: int | None = None
    agent_version: str | None = None
    agent_major_version: str = DEFAULT_AGENT_MAJOR_VERSION
    config_source: str | None = None
    fx_trace: bool = False
    vm_pid: int | None = None


class AgentSandboxManager:
    """Manage local Agent Sandbox state and command wrappers."""

    def __init__(self, state_root: Path | str = DEFAULT_STATE_ROOT, helper_path: Path | str | None = None):
        self.state_root = Path(state_root).expanduser()
        self.helper_path = Path(helper_path).expanduser() if helper_path else None

    def paths(self, name: str = DEFAULT_SANDBOX_NAME) -> SandboxPaths:
        root = self.state_root
        cache_dir = root / "cache"
        instances_dir = root / "instances"
        instance_dir = instances_dir / name
        ssh_dir = instance_dir / "ssh"
        provisioning_dir = instance_dir / "provisioning"
        vm_dir = instance_dir / "vm"
        return SandboxPaths(
            root=root,
            cache_dir=cache_dir,
            instances_dir=instances_dir,
            instance_dir=instance_dir,
            metadata_file=instance_dir / "metadata.json",
            ssh_dir=ssh_dir,
            private_key=ssh_dir / "id_ed25519",
            public_key=ssh_dir / "id_ed25519.pub",
            provisioning_dir=provisioning_dir,
            host_install_script=provisioning_dir / "install-host-agent.sh",
            cloud_init_dir=provisioning_dir / "cidata",
            cloud_init_user_data=provisioning_dir / "cidata" / "user-data",
            cloud_init_meta_data=provisioning_dir / "cidata" / "meta-data",
            seed_iso=provisioning_dir / "cidata.iso",
            vm_dir=vm_dir,
            disk_image=vm_dir / "ubuntu.raw",
            efi_variable_store=vm_dir / "efi-variable-store",
            serial_log=vm_dir / "serial.log",
            apt_cache_dir=cache_dir / "apt-archives",
        )

    def ensure_layout(self, name: str = DEFAULT_SANDBOX_NAME) -> SandboxPaths:
        paths = self.paths(name)
        for directory in (
            paths.cache_dir,
            paths.instances_dir,
            paths.instance_dir,
            paths.ssh_dir,
            paths.provisioning_dir,
            paths.cloud_init_dir,
            paths.vm_dir,
            paths.apt_cache_dir,
        ):
            directory.mkdir(parents=True, exist_ok=True)
        return paths

    def assert_supported_host(self) -> None:
        if platform.system() != "Darwin":
            raise AgentSandboxError("Agent Sandbox Stage A requires macOS with Apple Virtualization.framework")
        if platform.machine() != "arm64":
            raise AgentSandboxError("Agent Sandbox Stage A MVP supports Apple Silicon macOS hosts only")
        if not Path("/System/Library/Frameworks/Virtualization.framework").exists():
            raise AgentSandboxError("Apple Virtualization.framework is not available on this host")

    def prepare_host_sandbox(
        self,
        name: str = DEFAULT_SANDBOX_NAME,
        agent_version: str | None = None,
        config: Path | str | None = None,
        ubuntu_image: Path | str | None = None,
        guest_user: str = DEFAULT_GUEST_USER,
        fx_trace: bool = False,
    ) -> SandboxMetadata:
        """Create local state, credentials and provisioning inputs for a host sandbox."""
        self.assert_supported_host()
        paths = self.ensure_layout(name)
        if paths.metadata_file.exists():
            raise AgentSandboxError(f"sandbox {name!r} already exists at {paths.instance_dir}")

        self.ensure_ssh_key(paths)
        config_source = str(Path(config).expanduser()) if config else None
        if config_source and not Path(config_source).exists():
            raise AgentSandboxError(f"config override does not exist: {config_source}")
        if config_source:
            shutil.copyfile(config_source, paths.provisioning_dir / "datadog.yaml")

        metadata = SandboxMetadata(
            name=name,
            mode="host-agent",
            state="created",
            guest_user=guest_user,
            mac_address=self.mac_address_for_name(name),
            agent_version=agent_version,
            config_source=config_source,
            fx_trace=fx_trace,
        )
        self.write_host_install_script(paths.host_install_script, metadata)
        self.write_cloud_init_seed(paths, metadata)
        if ubuntu_image:
            self.prepare_disk_image(Path(ubuntu_image).expanduser(), paths.disk_image)
        else:
            self.clone_cached_ubuntu_base(paths.disk_image)
        self.write_metadata(paths.metadata_file, metadata)
        return metadata

    def prepare_base_builder(self, name: str = "base-builder") -> SandboxMetadata:
        self.assert_supported_host()
        paths = self.ensure_layout(name)
        if paths.metadata_file.exists():
            raise AgentSandboxError(f"sandbox {name!r} already exists at {paths.instance_dir}")
        self.ensure_ssh_key(paths)
        metadata = SandboxMetadata(
            name=name,
            mode="base-builder",
            state="created",
            guest_user=DEFAULT_GUEST_USER,
            mac_address=self.mac_address_for_name(name),
        )
        self.write_base_builder_cloud_init_seed(paths, metadata)
        self.clone_raw_ubuntu_base(paths.disk_image)
        self.write_metadata(paths.metadata_file, metadata)
        return metadata

    def write_base_builder_cloud_init_seed(self, paths: SandboxPaths, metadata: SandboxMetadata) -> None:
        public_key = paths.public_key.read_text().strip() if paths.public_key.exists() else ""
        if not public_key:
            raise AgentSandboxError("managed SSH public key is missing")
        user_data = f"""#cloud-config
users:
  - default
  - name: {metadata.guest_user}
    groups: [adm, sudo]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - {public_key}
package_update: true
packages:
  - ca-certificates
  - curl
  - gnupg
  - openssh-server
runcmd:
  - systemctl enable --now ssh || systemctl enable --now sshd
"""
        paths.cloud_init_user_data.write_text(user_data)
        paths.cloud_init_meta_data.write_text(
            f"instance-id: agent-sandbox-{metadata.name}\nlocal-hostname: agent-sandbox-{metadata.name}\n"
        )
        self.create_seed_iso(paths)

    def finalize_prepared_base(self, name: str = "base-builder") -> Path:
        metadata = self.read_metadata(name)
        paths = self.paths(name)
        cleanup_script = (
            "cloud-init clean --logs; "
            "rm -f /etc/ssh/ssh_host_*; "
            "truncate -s 0 /etc/machine-id; "
            "rm -f /var/lib/dbus/machine-id"
        )
        subprocess.run(
            self.ssh_command(name, ["sudo", "bash", "-lc", shlex.quote(cleanup_script)]),
            check=True,
        )
        if metadata.vm_pid:
            self.stop(name)
        prepared = self.prepared_base_path()
        prepared.parent.mkdir(parents=True, exist_ok=True)
        if prepared.exists():
            prepared.unlink()
        shutil.copyfile(paths.disk_image, prepared)
        return prepared

    def clone_raw_ubuntu_base(self, destination: Path) -> None:
        base = self.paths().cache_dir / "ubuntu-noble-arm64.raw"
        if not base.exists():
            source = self.ensure_cached_ubuntu_image()
            tmp = base.with_suffix(".raw.tmp")
            self.prepare_disk_image(source, tmp)
            tmp.replace(base)
        destination.parent.mkdir(parents=True, exist_ok=True)
        if destination.exists():
            destination.unlink()
        if platform.system() == "Darwin":
            subprocess.run(["cp", "-c", str(base), str(destination)], check=True)
        else:
            shutil.copyfile(base, destination)

    def mac_address_for_name(self, name: str) -> str:
        digest = hashlib.sha256(name.encode("utf-8")).digest()
        return f"02:dd:{digest[0]:02x}:{digest[1]:02x}:{digest[2]:02x}:{digest[3]:02x}"

    def ensure_ssh_key(self, paths: SandboxPaths) -> None:
        if paths.private_key.exists() and paths.public_key.exists():
            return
        if not shutil.which("ssh-keygen"):
            raise AgentSandboxError("ssh-keygen is required to create managed sandbox SSH credentials")
        subprocess.run(
            [
                "ssh-keygen",
                "-t",
                "ed25519",
                "-N",
                "",
                "-C",
                "agent-sandbox",
                "-f",
                str(paths.private_key),
            ],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    def agent_minor_version(self, version: str, major: str) -> str:
        prefix = f"{major}."
        if version.startswith(prefix):
            return version[len(prefix) :]
        return version

    def write_host_install_script(self, path: Path, metadata: SandboxMetadata) -> None:
        env = {
            "DD_API_KEY": DUMMY_API_KEY,
            "DD_AGENT_MAJOR_VERSION": metadata.agent_major_version,
        }
        if metadata.agent_version:
            env["DD_AGENT_MINOR_VERSION"] = self.agent_minor_version(
                metadata.agent_version, metadata.agent_major_version
            )

        exports = " ".join(f"{key}={shlex.quote(value)}" for key, value in env.items())
        config_copy = ""
        restart_needed = False
        if metadata.config_source:
            config_copy = "\ninstall -m 0644 /var/lib/agent-sandbox/datadog.yaml /etc/datadog-agent/datadog.yaml\n"
            restart_needed = True
        pre_install_setup = ""
        if metadata.fx_trace:
            pre_install_setup = r'''
mkdir -p /etc/systemd/system/datadog-agent.service.d /var/lib/agent-sandbox /var/log/datadog
cat > /var/lib/agent-sandbox/fx-trace-intake.py <<'PY'
#!/usr/bin/env python3
import http.server
import pathlib
import time

OUT = pathlib.Path('/var/log/datadog/fx-trace-spans.jsonl')
OUT.parent.mkdir(parents=True, exist_ok=True)

class Handler(http.server.BaseHTTPRequestHandler):
    def do_PUT(self):
        length = int(self.headers.get('content-length', '0'))
        body = self.rfile.read(length)
        with OUT.open('ab') as f:
            f.write(str(time.time_ns()).encode())
            f.write(b' ')
            f.write(body.replace(b'\n', b''))
            f.write(b'\n')
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'OK')

    def log_message(self, fmt, *args):
        return

http.server.ThreadingHTTPServer(('127.0.0.1', 8126), Handler).serve_forever()
PY
chmod 0755 /var/lib/agent-sandbox/fx-trace-intake.py
cat > /etc/systemd/system/agent-sandbox-fx-trace-intake.service <<'EOF'
[Unit]
Description=Agent Sandbox FX trace intake
Before=datadog-agent.service

[Service]
ExecStart=/usr/bin/python3 /var/lib/agent-sandbox/fx-trace-intake.py
Restart=always

[Install]
WantedBy=multi-user.target
EOF
cat > /etc/systemd/system/datadog-agent.service.d/agent-sandbox-fx-trace.conf <<'EOF'
[Service]
Environment=TRACE_FX=1
Environment=DD_FX_TRACING_ENABLED=true
EOF
systemctl daemon-reload
systemctl enable --now agent-sandbox-fx-trace-intake.service
'''
        restart_command = ""
        if restart_needed:
            restart_command = "\nsystemctl restart datadog-agent || service datadog-agent restart\n"

        package_version = ""
        if metadata.agent_version:
            full_version = (
                metadata.agent_version
                if metadata.agent_version.startswith(f"{metadata.agent_major_version}.")
                else f"{metadata.agent_major_version}.{metadata.agent_version}"
            )
            package_version = f"=1:{full_version}-1"

        script = f"""#!/usr/bin/env bash
set -euo pipefail

if ! command -v curl >/dev/null 2>&1; then
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y curl
fi

{pre_install_setup}
{exports} bash -c "$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh)" || true
if ! command -v /opt/datadog-agent/bin/agent/agent >/dev/null 2>&1; then
    apt-get update
    if ! apt-cache policy datadog-agent | grep -q 'Candidate: .*datadog-agent'; then
        curl -fsSL https://apt.datadoghq.com/dists/stable/7/binary-arm64/Packages \
            -o /var/lib/apt/lists/apt.datadoghq.com_dists_stable_7_binary-arm64_Packages
    fi
    DEBIAN_FRONTEND=noninteractive apt-get install -y datadog-agent{package_version} datadog-signing-keys
fi
{config_copy}
chown dd-agent:dd-agent /etc/datadog-agent/datadog.yaml || true
chmod 0640 /etc/datadog-agent/datadog.yaml || true
{restart_command}/opt/datadog-agent/bin/agent/agent version
"""
        path.write_text(script)
        path.chmod(0o755)

    def write_cloud_init_seed(self, paths: SandboxPaths, metadata: SandboxMetadata) -> None:
        public_key = paths.public_key.read_text().strip() if paths.public_key.exists() else ""
        if not public_key:
            raise AgentSandboxError("managed SSH public key is missing")

        config_write_file = ""
        if metadata.config_source:
            config_content = (paths.provisioning_dir / "datadog.yaml").read_text()
            config_write_file = f"""
  - path: /var/lib/agent-sandbox/datadog.yaml
    permissions: '0644'
    owner: root:root
    content: |
{textwrap.indent(config_content.rstrip(), '      ')}
"""

        install_script = paths.host_install_script.read_text()
        user_data = f"""#cloud-config
bootcmd:
  - mkdir -p /mnt/agent-sandbox-apt-cache /var/cache/apt/archives /mnt/agent-sandbox-apt-cache/partial
  - mount -t virtiofs agent_sandbox_apt_cache /mnt/agent-sandbox-apt-cache || true
  - mountpoint -q /mnt/agent-sandbox-apt-cache && mount --bind /mnt/agent-sandbox-apt-cache /var/cache/apt/archives || true
users:
  - default
  - name: {metadata.guest_user}
    groups: [adm, sudo]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - {public_key}
package_update: false
write_files:
  - path: /var/lib/agent-sandbox/install-host-agent.sh
    permissions: '0755'
    owner: root:root
    content: |
{textwrap.indent(install_script.rstrip(), '      ')}
{config_write_file}runcmd:
  - systemctl enable --now ssh || systemctl enable --now sshd
  - bash /var/lib/agent-sandbox/install-host-agent.sh
"""
        paths.cloud_init_user_data.write_text(user_data)
        paths.cloud_init_meta_data.write_text(
            f"instance-id: agent-sandbox-{metadata.name}\nlocal-hostname: agent-sandbox-{metadata.name}\n"
        )
        self.create_seed_iso(paths)

    def create_seed_iso(self, paths: SandboxPaths) -> None:
        if not shutil.which("hdiutil"):
            raise AgentSandboxError("hdiutil is required to create the cloud-init seed ISO")
        subprocess.run(
            [
                "hdiutil",
                "makehybrid",
                "-o",
                str(paths.seed_iso),
                "-iso",
                "-joliet",
                "-default-volume-name",
                "cidata",
                str(paths.cloud_init_dir),
            ],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    def ensure_cached_ubuntu_image(self) -> Path:
        image = self.paths().cache_dir / "ubuntu-noble-arm64.img"
        image.parent.mkdir(parents=True, exist_ok=True)
        if image.exists():
            return image
        print(f"Downloading Ubuntu cloud image to {image}")
        urllib.request.urlretrieve(DEFAULT_UBUNTU_IMAGE_URL, image)
        return image

    def prepared_base_path(self) -> Path:
        return self.paths().cache_dir / "ubuntu-noble-arm64-prepared.raw"

    def ensure_cached_ubuntu_base(self) -> Path:
        prepared = self.prepared_base_path()
        if prepared.exists():
            return prepared
        base = self.paths().cache_dir / "ubuntu-noble-arm64.raw"
        if base.exists():
            return base
        source = self.ensure_cached_ubuntu_image()
        tmp = base.with_suffix(".raw.tmp")
        self.prepare_disk_image(source, tmp)
        tmp.replace(base)
        return base

    def clone_cached_ubuntu_base(self, destination: Path) -> None:
        base = self.ensure_cached_ubuntu_base()
        destination.parent.mkdir(parents=True, exist_ok=True)
        if destination.exists():
            destination.unlink()
        if platform.system() == "Darwin":
            subprocess.run(["cp", "-c", str(base), str(destination)], check=True)
        else:
            shutil.copyfile(base, destination)

    def prepare_disk_image(self, source: Path, destination: Path) -> None:
        if not source.exists():
            raise AgentSandboxError(f"Ubuntu image does not exist: {source}")
        destination.parent.mkdir(parents=True, exist_ok=True)
        if not shutil.which("qemu-img"):
            raise AgentSandboxError("qemu-img is required to prepare Ubuntu cloud images for Apple Virtualization")

        image_format = self.qemu_image_format(source)
        if image_format == "raw":
            shutil.copyfile(source, destination)
        else:
            subprocess.run(["qemu-img", "convert", "-O", "raw", str(source), str(destination)], check=True)
        subprocess.run(["qemu-img", "resize", "-f", "raw", str(destination), "+10G"], check=True)

    def qemu_image_format(self, source: Path) -> str:
        result = subprocess.run(
            ["qemu-img", "info", "--output=json", str(source)],
            check=True,
            text=True,
            capture_output=True,
        )
        data = json.loads(result.stdout)
        return data.get("format", "raw")

    def read_metadata(self, name: str = DEFAULT_SANDBOX_NAME) -> SandboxMetadata:
        path = self.paths(name).metadata_file
        if not path.exists():
            raise AgentSandboxError(f"sandbox {name!r} does not exist at {path.parent}")
        data = json.loads(path.read_text())
        return SandboxMetadata(**data)

    def write_metadata(self, path: Path, metadata: SandboxMetadata) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(asdict(metadata), indent=2, sort_keys=True) + "\n")

    def update_connection(self, name: str, ssh_host: str, ssh_port: int, state: str = "running") -> SandboxMetadata:
        paths = self.paths(name)
        metadata = self.read_metadata(name)
        metadata.ssh_host = ssh_host
        metadata.ssh_port = ssh_port
        metadata.state = state
        self.write_metadata(paths.metadata_file, metadata)
        return metadata

    def status(self, name: str = DEFAULT_SANDBOX_NAME) -> dict[str, Any]:
        metadata = self.read_metadata(name)
        paths = self.paths(name)
        return {
            **asdict(metadata),
            "state_root": str(self.state_root),
            "instance_dir": str(paths.instance_dir),
            "ssh_private_key": str(paths.private_key),
            "host_install_script": str(paths.host_install_script),
            "seed_iso": str(paths.seed_iso),
            "disk_image": str(paths.disk_image),
            "serial_log": str(paths.serial_log),
            "apt_cache_dir": str(paths.apt_cache_dir),
        }

    def ssh_command(self, name: str = DEFAULT_SANDBOX_NAME, extra_args: list[str] | None = None) -> list[str]:
        metadata = self.read_metadata(name)
        paths = self.paths(name)
        if not metadata.ssh_host or not metadata.ssh_port:
            raise AgentSandboxError(
                f"sandbox {name!r} has no SSH endpoint yet; start the VM helper or update connection metadata"
            )
        return [
            "ssh",
            "-i",
            str(paths.private_key),
            "-p",
            str(metadata.ssh_port),
            "-o",
            "IdentitiesOnly=yes",
            "-o",
            "StrictHostKeyChecking=accept-new",
            f"{metadata.guest_user}@{metadata.ssh_host}",
            *(extra_args or []),
        ]

    def agent_command(self, name: str, agent_args: str) -> list[str]:
        return self.ssh_command(name, ["sudo", "/opt/datadog-agent/bin/agent/agent", *shlex.split(agent_args)])

    def wait_agent_ready(self, name: str = DEFAULT_SANDBOX_NAME, timeout_seconds: int = 240) -> None:
        deadline = time.time() + timeout_seconds
        last_output = ""
        while time.time() < deadline:
            result = subprocess.run(
                self.agent_command(name, "status"),
                check=False,
                text=True,
                capture_output=True,
            )
            if result.returncode == 0:
                return
            last_output = (result.stdout + result.stderr).strip()
            time.sleep(5)
        raise AgentSandboxError(f"timed out waiting for Agent command port; last output: {last_output}")

    def logs_command(self, name: str, lines: int = 200) -> list[str]:
        return self.ssh_command(name, ["sudo", "journalctl", "-u", "datadog-agent", "-n", str(lines), "--no-pager"])

    def destroy(self, name: str = DEFAULT_SANDBOX_NAME) -> Path:
        paths = self.paths(name)
        metadata = self.read_metadata(name) if paths.metadata_file.exists() else None
        if metadata and metadata.vm_pid:
            self.stop(name)
        if not paths.instance_dir.exists():
            raise AgentSandboxError(f"sandbox {name!r} does not exist")
        shutil.rmtree(paths.instance_dir)
        return paths.instance_dir

    def start_background(self, name: str = DEFAULT_SANDBOX_NAME) -> SandboxMetadata:
        paths = self.paths(name)
        metadata = self.read_metadata(name)
        if metadata.vm_pid and self.pid_is_running(metadata.vm_pid):
            raise AgentSandboxError(f"sandbox {name!r} is already running with pid {metadata.vm_pid}")
        log = paths.vm_dir / "helper.log"
        with log.open("ab") as output:
            process = subprocess.Popen(
                self.helper_command(name, "start"),
                stdout=output,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )
        metadata.vm_pid = process.pid
        metadata.state = "running"
        self.write_metadata(paths.metadata_file, metadata)
        return metadata

    def stop(self, name: str = DEFAULT_SANDBOX_NAME) -> SandboxMetadata:
        metadata = self.read_metadata(name)
        if metadata.vm_pid and self.pid_is_running(metadata.vm_pid):
            os_pid = metadata.vm_pid
            try:
                os.kill(os_pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            for _ in range(30):
                if not self.pid_is_running(os_pid):
                    break
                time.sleep(0.2)
            if self.pid_is_running(os_pid):
                os.kill(os_pid, signal.SIGKILL)
        metadata.vm_pid = None
        metadata.state = "stopped"
        self.write_metadata(self.paths(name).metadata_file, metadata)
        return metadata

    def pid_is_running(self, pid: int) -> bool:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            return False
        except PermissionError:
            return True
        return True

    def discover_ssh_endpoint(self, name: str = DEFAULT_SANDBOX_NAME, timeout_seconds: int = 180) -> SandboxMetadata:
        metadata = self.read_metadata(name)
        if not metadata.mac_address:
            raise AgentSandboxError(f"sandbox {name!r} has no managed MAC address")
        deadline = time.time() + timeout_seconds
        while time.time() < deadline:
            host = self.ip_for_mac(metadata.mac_address)
            if host and self.tcp_port_open(host, 22, timeout_seconds=2):
                return self.update_connection(name, host, 22, state="running")
            time.sleep(2)
        raise AgentSandboxError(f"timed out waiting for SSH endpoint for MAC {metadata.mac_address}")

    def ip_for_mac(self, mac_address: str) -> str | None:
        result = subprocess.run(["arp", "-an"], check=False, text=True, capture_output=True)
        needles = {mac_address.lower(), self.compact_mac(mac_address)}
        for line in result.stdout.splitlines():
            lower = line.lower()
            if not any(needle in lower for needle in needles):
                continue
            start = line.find("(")
            end = line.find(")", start + 1)
            if start != -1 and end != -1:
                return line[start + 1 : end]
        return None

    def compact_mac(self, value: str) -> str:
        parts = value.lower().replace("-", ":").split(":")
        if len(parts) != 6:
            return value.lower()
        return ":".join(part.lstrip("0") or "0" for part in parts)

    def tcp_port_open(self, host: str, port: int, timeout_seconds: int = 2) -> bool:
        import socket

        try:
            with socket.create_connection((host, port), timeout=timeout_seconds):
                return True
        except OSError:
            return False

    def helper_command(self, name: str, command: str) -> list[str]:
        paths = self.paths(name)
        metadata = self.read_metadata(name)
        if not metadata.mac_address:
            raise AgentSandboxError(f"sandbox {name!r} has no managed MAC address")
        return [
            str(self.require_helper()),
            command,
            "--disk",
            str(paths.disk_image),
            "--seed",
            str(paths.seed_iso),
            "--efi",
            str(paths.efi_variable_store),
            "--serial",
            str(paths.serial_log),
            "--apt-cache",
            str(paths.apt_cache_dir),
            "--mac",
            metadata.mac_address,
        ]

    def require_helper(self) -> Path:
        helper = self.helper_path or self.state_root / "bin" / "agent-sandbox-vz"
        if not helper.exists():
            raise AgentSandboxError(
                "Agent Sandbox VM helper is not built yet. The Python/invoke wrapper can prepare state and SSH "
                f"metadata, but VM lifecycle requires {helper}."
            )
        return helper
