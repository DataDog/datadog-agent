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
import socket
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
    fakeintake_url: str | None = None
    fakeintake_pid: int | None = None
    fakeintake_log: str | None = None
    kubernetes: bool = False
    agent_image: str | None = None
    helm_values_source: str | None = None
    kubeconfig_path: str | None = None
    k3s_version: str | None = None
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
        fakeintake_url: str | None = None,
        fakeintake_pid: int | None = None,
        fakeintake_log: str | None = None,
        kubernetes: bool = False,
        agent_image: str | None = None,
        helm_values_source: str | None = None,
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
        else:
            self.write_default_datadog_config(paths.provisioning_dir / "datadog.yaml", fakeintake_url)

        metadata = SandboxMetadata(
            name=name,
            mode="kubernetes-agent" if kubernetes else "host-agent",
            state="created",
            guest_user=guest_user,
            mac_address=self.mac_address_for_name(name),
            agent_version=agent_version,
            config_source=config_source,
            fx_trace=fx_trace,
            fakeintake_url=fakeintake_url,
            fakeintake_pid=fakeintake_pid,
            fakeintake_log=fakeintake_log,
            kubernetes=kubernetes,
            agent_image=agent_image,
            helm_values_source=helm_values_source,
        )
        if kubernetes:
            self.write_kubernetes_cloud_init_seed(paths, metadata)
        else:
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
        self.copy_disk_image(base, destination)

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

    def write_default_datadog_config(self, path: Path, fakeintake_url: str | None = None) -> None:
        endpoint_config = ""
        if fakeintake_url:
            host_port = fakeintake_url.removeprefix("http://").removeprefix("https://")
            endpoint_config = f"""
dd_url: {fakeintake_url}
logs_config:
  logs_dd_url: {host_port}
  logs_no_ssl: true
apm_config:
  apm_dd_url: {fakeintake_url}
process_config:
  process_dd_url: {fakeintake_url}
"""
        path.write_text(
            f"""api_key: {DUMMY_API_KEY}
cloud_provider_metadata: []
remote_configuration:
  enabled: false
{endpoint_config}"""
        )

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
        restart_command = r'''
if ! cmp -s /var/lib/agent-sandbox/datadog.yaml /etc/datadog-agent/datadog.yaml; then
    install -m 0644 /var/lib/agent-sandbox/datadog.yaml /etc/datadog-agent/datadog.yaml
    chown dd-agent:dd-agent /etc/datadog-agent/datadog.yaml || true
    chmod 0640 /etc/datadog-agent/datadog.yaml || true
    systemctl restart datadog-agent || service datadog-agent restart
fi
'''

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

TIMELINE=/var/log/datadog/agent-sandbox-install-timeline.log
mkdir -p /var/log/datadog
mark() {{ echo "$(date +%s.%N) $1" | tee -a "$TIMELINE"; }}
mark install_start

if ! command -v curl >/dev/null 2>&1; then
    mark curl_install_start
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y curl
    mark curl_install_done
fi

{pre_install_setup}
mark preseed_config_start
mkdir -p /etc/datadog-agent
install -m 0644 /var/lib/agent-sandbox/datadog.yaml /etc/datadog-agent/datadog.yaml
mark preseed_config_done
mark installer_script_start
{exports} bash -c "$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh)" || true
mark installer_script_done
if ! command -v /opt/datadog-agent/bin/agent/agent >/dev/null 2>&1; then
    mark fallback_apt_start
    apt-get update
    if ! apt-cache policy datadog-agent | grep -q 'Candidate: .*datadog-agent'; then
        curl -fsSL https://apt.datadoghq.com/dists/stable/7/binary-arm64/Packages \
            -o /var/lib/apt/lists/apt.datadoghq.com_dists_stable_7_binary-arm64_Packages
    fi
    DEBIAN_FRONTEND=noninteractive apt-get install -y datadog-agent{package_version} datadog-signing-keys
    mark fallback_apt_done
fi
ln -sf /opt/datadog-agent/bin/agent/agent /usr/local/bin/agent
mark config_reconcile_start
{restart_command}mark config_reconcile_done
mark agent_version_start
/opt/datadog-agent/bin/agent/agent version
mark agent_version_done
"""
        path.write_text(script)
        path.chmod(0o755)

    def write_kubernetes_cloud_init_seed(self, paths: SandboxPaths, metadata: SandboxMetadata) -> None:
        public_key = paths.public_key.read_text().strip() if paths.public_key.exists() else ""
        if not public_key:
            raise AgentSandboxError("managed SSH public key is missing")
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
runcmd:
  - systemctl enable --now ssh || systemctl enable --now sshd
"""
        paths.cloud_init_user_data.write_text(user_data)
        paths.cloud_init_meta_data.write_text(
            f"instance-id: agent-sandbox-{metadata.name}\nlocal-hostname: agent-sandbox-{metadata.name}\n"
        )
        self.create_seed_iso(paths)

    def write_cloud_init_seed(self, paths: SandboxPaths, metadata: SandboxMetadata) -> None:
        public_key = paths.public_key.read_text().strip() if paths.public_key.exists() else ""
        if not public_key:
            raise AgentSandboxError("managed SSH public key is missing")

        config_write_file = ""
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
        if paths.seed_iso.exists():
            paths.seed_iso.unlink()
        result = subprocess.run(
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
            check=False,
            text=True,
            capture_output=True,
        )
        if result.returncode != 0:
            detail = (result.stderr or result.stdout or "unknown error").strip()
            raise AgentSandboxError(f"failed to create cloud-init seed ISO: {detail}")

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
        self.copy_disk_image(base, destination)

    def copy_disk_image(self, source: Path, destination: Path) -> None:
        """Copy a disk image, using APFS clonefile when the host supports it."""
        destination.parent.mkdir(parents=True, exist_ok=True)
        if destination.exists():
            destination.unlink()
        if platform.system() == "Darwin":
            cp = Path("/bin/cp")
            if cp.exists():
                result = subprocess.run([str(cp), "-c", str(source), str(destination)], check=False)
                if result.returncode == 0:
                    return
                if destination.exists():
                    destination.unlink()
        shutil.copyfile(source, destination)

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

    def fakeintake_binary(self) -> Path:
        for candidate in (
            Path("bazel-bin/test/fakeintake/cmd/server/server_/server"),
            Path("bazel-bin/test/fakeintake/cmd/server/server"),
            Path("test/fakeintake/build/fakeintake"),
        ):
            if candidate.exists():
                return candidate
        return Path("bazel-bin/test/fakeintake/cmd/server/server_/server")

    def find_free_port(self) -> int:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.bind(("0.0.0.0", 0))
            return int(sock.getsockname()[1])

    def host_bridge_ip(self) -> str:
        result = subprocess.run(["ifconfig", "bridge100"], check=False, text=True, capture_output=True)
        for line in result.stdout.splitlines():
            line = line.strip()
            if line.startswith("inet "):
                return line.split()[1]
        return "192.168.64.1"

    def start_fakeintake_process(self, name: str = DEFAULT_SANDBOX_NAME) -> tuple[str, int, str]:
        binary = self.fakeintake_binary()
        if not binary.exists():
            raise AgentSandboxError("fakeintake is not built; run dda inv fakeintake.build")
        paths = self.ensure_layout(name)
        port = self.find_free_port()
        log_path = paths.instance_dir / "fakeintake.log"
        log = log_path.open("ab")
        process = subprocess.Popen(
            [str(binary), f"-port={port}", "-retention-period=30m"],
            stdout=log,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        log.close()
        for _ in range(50):
            if not self.pid_is_running(process.pid):
                break
            if self.tcp_port_open("127.0.0.1", port, timeout_seconds=1):
                return f"http://{self.host_bridge_ip()}:{port}", process.pid, str(log_path)
            time.sleep(0.1)
        raise AgentSandboxError(f"fakeintake did not become ready; see {log_path}")

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
        command = [
            "ssh",
            "-i",
            str(paths.private_key),
            "-p",
            str(metadata.ssh_port),
        ]
        if ":" in metadata.ssh_host:
            command.append("-6")
        command.extend(
            [
                "-o",
                "IdentitiesOnly=yes",
                "-o",
                f"UserKnownHostsFile={paths.ssh_dir / 'known_hosts'}",
                "-o",
                "StrictHostKeyChecking=accept-new",
                "-o",
                "ConnectTimeout=10",
                "-o",
                "ServerAliveInterval=5",
                "-o",
                "ServerAliveCountMax=3",
                f"{metadata.guest_user}@{metadata.ssh_host}",
                *(extra_args or []),
            ]
        )
        return command

    def shell_command(self, name: str, command: str) -> list[str]:
        return self.ssh_command(name, ["bash", "-lc", shlex.quote(command)])

    def agent_command(self, name: str, agent_args: str) -> list[str]:
        return self.ssh_command(name, ["sudo", "/opt/datadog-agent/bin/agent/agent", *shlex.split(agent_args)])

    def kubectl_command(self, name: str, kubectl_args: str) -> list[str]:
        return self.ssh_command(name, ["sudo", "k3s", "kubectl", *shlex.split(kubectl_args)])

    def provision_kubernetes(
        self, name: str, agent_image: str, helm_values: Path | str | None = None
    ) -> SandboxMetadata:
        metadata = self.read_metadata(name)
        if not metadata.ssh_host:
            raise AgentSandboxError("sandbox must have SSH before Kubernetes provisioning")
        fakeintake_url = metadata.fakeintake_url or f"http://{self.host_bridge_ip()}:80"
        values_path = self.paths(name).provisioning_dir / "datadog-values.yaml"
        self.write_datadog_helm_values(values_path, agent_image, fakeintake_url, helm_values)
        self.copy_to_guest(name, values_path, "/var/lib/agent-sandbox/datadog-values.yaml")
        install_script = self.kubernetes_install_script(name, agent_image)
        subprocess.run(self.shell_command(name, install_script), check=True)
        metadata.agent_image = agent_image
        metadata.helm_values_source = str(Path(helm_values).expanduser()) if helm_values else None
        metadata.kubernetes = True
        metadata.mode = "kubernetes-agent"
        metadata.k3s_version = self.guest_output(name, "k3s --version | head -1 || true")
        metadata.kubeconfig_path = str(self.export_kubeconfig(name))
        self.write_metadata(self.paths(name).metadata_file, metadata)
        return metadata

    def copy_to_guest(self, name: str, source: Path, destination: str) -> None:
        metadata = self.read_metadata(name)
        paths = self.paths(name)
        if not metadata.ssh_host or not metadata.ssh_port:
            raise AgentSandboxError(f"sandbox {name!r} has no SSH endpoint")
        command = [
            "scp",
            "-i",
            str(paths.private_key),
            "-P",
            str(metadata.ssh_port),
        ]
        if ":" in metadata.ssh_host:
            command.append("-6")
        command.extend(
            [
                "-o",
                "IdentitiesOnly=yes",
                "-o",
                f"UserKnownHostsFile={paths.ssh_dir / 'known_hosts'}",
                "-o",
                "StrictHostKeyChecking=accept-new",
                str(source),
                f"{metadata.guest_user}@{metadata.ssh_host}:/tmp/{Path(destination).name}",
            ]
        )
        subprocess.run(command, check=True)
        subprocess.run(
            self.shell_command(
                name,
                f"sudo mkdir -p {shlex.quote(str(Path(destination).parent))} && sudo mv /tmp/{shlex.quote(Path(destination).name)} {shlex.quote(destination)}",
            ),
            check=True,
        )

    def guest_output(self, name: str, command: str) -> str:
        result = subprocess.run(self.shell_command(name, command), check=True, text=True, capture_output=True)
        return result.stdout.strip()

    def write_datadog_helm_values(
        self, path: Path, agent_image: str, fakeintake_url: str, helm_values: Path | str | None = None
    ) -> None:
        repository, tag = self.split_image(agent_image)
        host_port = fakeintake_url.removeprefix("http://").removeprefix("https://")
        content = f"""datadog:
  apiKey: a0000000000000000000000000000001
  site: datadoghq.com
  dd_url: {fakeintake_url}
  kubelet:
    tlsVerify: false
  logs:
    enabled: false
  apm:
    enabled: false
  processAgent:
    processCollection: true
  operator:
    enabled: false
agents:
  image:
    repository: {repository}
    tag: {tag}
  containers:
    agent:
      env:
        - name: DD_REMOTE_CONFIGURATION_ENABLED
          value: "false"
        - name: DD_CLOUD_PROVIDER_METADATA
          value: "[]"
        - name: DD_PROCESS_CONFIG_PROCESS_DD_URL
          value: {fakeintake_url}
        - name: DD_LOGS_CONFIG_LOGS_DD_URL
          value: {host_port}
        - name: DD_LOGS_CONFIG_LOGS_NO_SSL
          value: "true"
"""
        if helm_values:
            content += "\n# User-provided values appended below. Later keys override earlier keys.\n"
            content += Path(helm_values).expanduser().read_text()
        path.write_text(content)

    def split_image(self, image: str) -> tuple[str, str]:
        if ":" not in image.rsplit("/", 1)[-1]:
            return image, "latest"
        repository, tag = image.rsplit(":", 1)
        return repository, tag

    def kubernetes_install_script(self, name: str, agent_image: str) -> str:
        metadata = self.read_metadata(name)
        tls_san = metadata.ssh_host or "127.0.0.1"
        return f"""
set -euo pipefail
TIMELINE=/var/log/datadog/agent-sandbox-kubernetes-timeline.log
sudo mkdir -p /var/log/datadog /var/lib/agent-sandbox
mark() {{ echo "$(date +%s.%N) $1" | sudo tee -a "$TIMELINE"; }}
mark k3s_install_start
if ! command -v k3s >/dev/null 2>&1; then
  curl -4 --connect-timeout 10 --max-time 120 -sfL https://get.k3s.io | INSTALL_K3S_EXEC='server --disable=traefik --disable=servicelb --disable=metrics-server --write-kubeconfig-mode=0644 --tls-san {tls_san}' sh -
fi
mark k3s_install_done
mark node_ready_start
for i in $(seq 1 180); do
  if sudo k3s kubectl wait node --all --for=condition=Ready --timeout=2s >/dev/null 2>&1; then break; fi
  sleep 1
  if [ "$i" = "180" ]; then sudo k3s kubectl get nodes -o wide; exit 1; fi
done
mark node_ready_done
mark helm_install_start
if ! command -v helm >/dev/null 2>&1; then
  curl -4 --connect-timeout 10 --max-time 120 -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
fi
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
helm repo add datadog https://helm.datadoghq.com >/dev/null 2>&1 || true
helm repo update datadog >/dev/null
sudo k3s kubectl create namespace datadog --dry-run=client -o yaml | sudo k3s kubectl apply -f -
helm upgrade --install datadog-agent datadog/datadog \
  --namespace datadog \
  --values /var/lib/agent-sandbox/datadog-values.yaml \
  --wait --timeout 5m
mark helm_install_done
mark agent_ready_start
sudo k3s kubectl -n datadog rollout status daemonset/datadog-agent --timeout=5m
mark agent_ready_done
"""

    def export_kubeconfig(self, name: str) -> Path:
        metadata = self.read_metadata(name)
        paths = self.paths(name)
        result = subprocess.run(
            self.ssh_command(name, ["sudo", "cat", "/etc/rancher/k3s/k3s.yaml"]),
            check=True,
            text=True,
            capture_output=True,
        )
        server = f"https://{metadata.ssh_host}:6443"
        content = result.stdout.replace("https://127.0.0.1:6443", server)
        kubeconfig = paths.instance_dir / "kubeconfig"
        kubeconfig.write_text(content)
        return kubeconfig

    def wait_agent_ready(self, name: str = DEFAULT_SANDBOX_NAME, timeout_seconds: int = 240) -> None:
        deadline = time.time() + timeout_seconds
        last_output = ""
        while time.time() < deadline:
            try:
                result = subprocess.run(
                    self.agent_command(name, "status"),
                    check=False,
                    text=True,
                    capture_output=True,
                    timeout=20,
                )
            except subprocess.TimeoutExpired as e:
                last_output = f"agent status timed out after {e.timeout}s"
                time.sleep(2)
                continue
            if result.returncode == 0:
                return
            last_output = (result.stdout + result.stderr).strip()
            time.sleep(2)
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
        if metadata.fakeintake_pid and self.pid_is_running(metadata.fakeintake_pid):
            try:
                os.kill(metadata.fakeintake_pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            for _ in range(30):
                if not self.pid_is_running(metadata.fakeintake_pid):
                    break
                time.sleep(0.1)
            if metadata.fakeintake_pid and self.pid_is_running(metadata.fakeintake_pid):
                os.kill(metadata.fakeintake_pid, signal.SIGKILL)
        metadata.fakeintake_pid = None
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

    def discover_ssh_endpoint(self, name: str = DEFAULT_SANDBOX_NAME, timeout_seconds: int = 120) -> SandboxMetadata:
        metadata = self.read_metadata(name)
        if not metadata.mac_address:
            raise AgentSandboxError(f"sandbox {name!r} has no managed MAC address")
        deadline = time.time() + timeout_seconds
        while time.time() < deadline:
            host = self.ip_for_mac(metadata.mac_address) or self.ipv6_link_local_for_mac(metadata.mac_address)
            if host and self.tcp_port_open(host, 22, timeout_seconds=2):
                return self.update_connection(name, host, 22, state="running")
            time.sleep(2)
        host = self.ip_for_mac(metadata.mac_address)
        serial_tail = ""
        serial_log = self.paths(name).serial_log
        if serial_log.exists():
            serial_tail = "\nserial tail:\n" + "\n".join(serial_log.read_text(errors="replace").splitlines()[-20:])
        detail = f"; last ARP host for MAC: {host}" if host else ""
        raise AgentSandboxError(
            f"timed out waiting for SSH endpoint for MAC {metadata.mac_address}{detail}{serial_tail}"
        )

    def ipv6_link_local_for_mac(self, mac_address: str) -> str | None:
        parts = mac_address.lower().replace("-", ":").split(":")
        if len(parts) != 6:
            return None
        try:
            octets = [int(part, 16) for part in parts]
        except ValueError:
            return None
        octets[0] ^= 0x02
        groups = (
            (octets[0] << 8) | octets[1],
            (octets[2] << 8) | 0xFF,
            0xFE00 | octets[3],
            (octets[4] << 8) | octets[5],
        )
        return f"fe80::{groups[0]:x}:{groups[1]:x}:{groups[2]:x}:{groups[3]:x}%bridge100"

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
