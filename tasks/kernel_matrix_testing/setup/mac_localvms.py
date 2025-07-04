import json
import os
from pathlib import Path

from invoke.context import Context

from tasks.kernel_matrix_testing.kmt_os import get_homebrew_prefix
from tasks.kernel_matrix_testing.setup.utils import check_launchctl_service, ensure_options_in_config
from tasks.libs.common.status import Status

from .requirement import Requirement, RequirementState


class MacPackages(Requirement):
    name: str = "mac-packages"

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        packages = ["aria2", "fio", "socat", "libvirt", "gnu-sed", "qemu", "libvirt", "wget"]

        missing_packages = []
        for package in packages:
            res = ctx.run(f"brew list {package}", warn=True, hide=True)
            if res is None or not res.ok:
                missing_packages.append(package)

        if len(missing_packages) == 0:
            return RequirementState(Status.OK, "All packages are installed.")

        if not fix:
            return RequirementState(Status.FAIL, f"Missing packages: {', '.join(missing_packages)}", fixable=True)

        try:
            ctx.run(f"brew install {' '.join(packages)}")
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to install packages: {e}")

        return RequirementState(Status.OK, "Packages installed.")


class MacLibvirtConfig(Requirement):
    name: str = "mac-libvirt-config"

    def check(self, ctx: Context, fix: bool):
        from tasks.kernel_matrix_testing.kmt_os import MacOS

        options = {
            "unix_sock_dir": MacOS.libvirt_system_dir,
            "unix_sock_ro_perms": "0777",
            "unix_sock_rw_perms": "0777",
            "uri_default": MacOS.libvirt_socket,
            "log_outputs": f"2:file:{MacOS.libvirt_system_dir}/libvirtd.log",
        }

        try:
            incorrect_options = ensure_options_in_config(
                ctx, MacOS.libvirt_conf, options, change=fix, write_with_sudo=False
            )
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to check libvirt config: {e}")

        if len(incorrect_options) == 0:
            return RequirementState(Status.OK, "Libvirt config is correct.")

        if fix:
            return RequirementState(Status.OK, "Libvirt config fixed.")

        return RequirementState(Status.FAIL, f"Libvirt config is incorrect. {incorrect_options}", fixable=True)


class MacVirtlogdConfig(Requirement):
    name: str = "mac-virtlogd-config"

    def check(self, ctx: Context, fix: bool):
        from tasks.kernel_matrix_testing.kmt_os import MacOS

        options = {
            "log_outputs": f"2:file:{MacOS.libvirt_system_dir}/virtlogd.log",
        }

        try:
            incorrect_options = ensure_options_in_config(
                ctx, MacOS.virtlogd_conf, options, change=fix, write_with_sudo=False
            )
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to check virtlogd config: {e}")

        if len(incorrect_options) == 0:
            return RequirementState(Status.OK, "Virtlogd config is correct.")

        if fix:
            return RequirementState(Status.OK, "Virtlogd config fixed.")

        return RequirementState(
            Status.FAIL,
            f"Virtlogd config is incorrect: options {incorrect_options} do not have expected values",
            fixable=True,
        )


class MacVirtlogdService(Requirement):
    name: str = "virtlogd-service"

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        import plistlib

        from tasks.kernel_matrix_testing.kmt_os import MacOS

        virtlogd_plist_path = Path("/Library/LaunchDaemons/org.libvirt.virtlogd.plist")
        if not virtlogd_plist_path.exists():
            if not fix:
                return RequirementState(Status.FAIL, "virtlogd plist missing.", fixable=True)

            plist_data = {
                "EnvironmentVariables": {"PATH": os.fspath(get_homebrew_prefix() / "bin")},
                "KeepAlive": True,
                "Label": "org.libvirt.virtlogd",
                "LimitLoadToSessionType": ["Aqua", "Background", "LoginWindow", "StandardIO", "System"],
                "ProgramArguments": [
                    os.fspath(get_homebrew_prefix() / "sbin/virtlogd"),
                    "-f",
                    os.fspath(MacOS.virtlogd_conf),
                ],
                "RunAtLoad": True,
            }

            try:
                # Allow writing the file without superuser permissions
                ctx.run(f"sudo touch {virtlogd_plist_path}")
                ctx.run(f"sudo chmod 666 {virtlogd_plist_path}")

                # Write the plist data to the file
                with open(virtlogd_plist_path, "wb") as f:
                    plistlib.dump(plist_data, f)

                # Set the correct permissions and load the service
                ctx.run(f"sudo chmod 644 {virtlogd_plist_path}")
                ctx.run(f"sudo launchctl load -w {virtlogd_plist_path}")
            except Exception as e:
                return RequirementState(Status.FAIL, f"Failed to create virtlogd plist: {e}")

        return check_launchctl_service(ctx, "org.libvirt.virtlogd", fix)


class MacLibvirtService(Requirement):
    name: str = "mac-libvirt-service"

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        service_name = "libvirt"
        res = ctx.run(f"sudo brew services info {service_name} --json", warn=True)
        if res is None or not res.ok:
            return RequirementState(Status.FAIL, f"Failed to check libvirt service: {res}")

        service_info = json.loads(res.stdout)
        if not service_info.get("running", False):
            if not fix:
                return RequirementState(Status.FAIL, "Libvirt service is not running.", fixable=True)

            ctx.run(f"sudo brew services start {service_name}")

        return RequirementState(Status.OK, "Libvirt service is running.")


class EnableServices(Requirement):
    name: str = "enable-services"

    def check(self, ctx: Context, fix: bool):
        # Always enable/start if fix is requested
        if not fix:
            return [RequirementState(Status.OK, "Service enable/start not required.")]
        ctx.run("sudo launchctl enable system/org.libvirt.virtlogd")
        ctx.run("sudo launchctl start system/org.libvirt.virtlogd || true")
        ctx.run("sudo brew services start libvirt")
        ctx.run("sudo sysctl -w net.inet.ip.forwarding=1")
        ctx.run("sudo launchctl load -w /System/Library/LaunchDaemons/bootps.plist || true")
        ctx.run("sudo launchctl start com.apple.bootpd || true")
        return [RequirementState(Status.OK, "Services enabled/started.", fixable=True)]


class BootPService(Requirement):
    name: str = "bootp-service"

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        return check_launchctl_service(
            ctx, "com.apple.bootpd", fix, service_install_file="/System/Library/LaunchDaemons/bootps.plist"
        )


class MacNFSService(Requirement):
    name: str = "mac-nfs-service"

    def check(self, ctx: Context, fix: bool) -> list[RequirementState]:
        res = ctx.run("sudo nfsd status", warn=True)
        if res is None or not res.ok:
            return [RequirementState(Status.FAIL, f"Failed to check NFS service: {res}")]

        states: list[RequirementState] = []
        if "nfsd service is enabled" in res.stdout:
            states.append(RequirementState(Status.OK, "NFS service is enabled."))
        else:
            if not fix:
                states.append(RequirementState(Status.FAIL, "NFS service is not enabled.", fixable=True))
            else:
                ctx.run("sudo nfsd enable")
                states.append(RequirementState(Status.OK, "NFS service enabled."))

        if "nfsd is running" in res.stdout:
            states.append(RequirementState(Status.OK, "NFS service is running."))
        else:
            if not fix:
                states.append(RequirementState(Status.FAIL, "NFS service is not running.", fixable=True))
            else:
                ctx.run("sudo nfsd start")
                states.append(RequirementState(Status.OK, "NFS service updated."))

        return states


class MacNFSExport(Requirement):
    name: str = "mac-nfs-export"
    dependencies: list[type[Requirement]] = [MacNFSService]

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        from tasks.kernel_matrix_testing.kmt_os import MacOS

        exports_file = Path("/etc/exports")
        shared_dir = MacOS.shared_dir
        export_line = f"{shared_dir} -network 192.168.0.0 -mask 255.255.0.0"
        if exports_file.exists() and export_line in exports_file.read_text():
            return RequirementState(Status.OK, "NFS export already present.")
        if not fix:
            return RequirementState(Status.FAIL, "NFS export missing.", fixable=True)

        try:
            ctx.run(f"echo '{export_line}' | sudo tee -a {exports_file}")
            ctx.run("sudo nfsd update")
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to add NFS export: {e}")

        return RequirementState(Status.OK, "NFS export added.", fixable=True)


class MacIPForwarding(Requirement):
    name: str = "mac-ip-forwarding"

    def check(self, ctx: Context, fix: bool) -> RequirementState:
        res = ctx.run("sudo sysctl net.inet.ip.forwarding", warn=True)
        if res is None or not res.ok:
            return RequirementState(Status.FAIL, "Failed to check IP forwarding")

        if res.stdout.strip().split()[-1] == "1":
            return RequirementState(Status.OK, "IP forwarding is already enabled.")

        if not fix:
            return RequirementState(Status.FAIL, "IP forwarding is not enabled.", fixable=True)

        try:
            ctx.run("sudo sysctl -w net.inet.ip.forwarding=1")
        except Exception as e:
            return RequirementState(Status.FAIL, f"Failed to enable IP forwarding: {e}")

        return RequirementState(Status.OK, "IP forwarding enabled.")
