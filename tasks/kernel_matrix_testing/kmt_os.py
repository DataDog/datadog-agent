import getpass
import os
import plistlib
import sys
from pathlib import Path

from invoke.context import Context

from tasks.kernel_matrix_testing.tool import Exit
from tasks.system_probe import is_root


def get_home_linux():
    return Path("/home/kernel-version-testing")


def get_home_macos():
    return Path.expanduser(Path("~/kernel-version-testing"))


def get_homebrew_prefix():
    # Return a fixed path. Funnily enough, you need to know the homebrew prefix (or have it loaded in $PATH)
    # to be able to run brew --prefix and get the homebrew prefix. Because of that, it's just
    # simpler to return the default, expected path.
    return Path("/opt/homebrew")


def get_kmt_os():
    if sys.platform == "linux" or sys.platform == "linux2":
        return Linux
    elif sys.platform == "darwin":
        return MacOS

    raise Exit(f"unsupported platform: {sys.platform}")


class Linux:
    kmt_dir = get_home_linux()
    libvirt_group = "libvirt"
    rootfs_dir = kmt_dir / "rootfs"
    stacks_dir = kmt_dir / "stacks"
    packages_dir = kmt_dir / "kernel-packages"
    libvirt_dir = kmt_dir / "libvirt"
    shared_dir = Path("/opt/kernel-version-testing")
    libvirt_socket = "qemu:///system"

    qemu_conf = os.path.join("/", "etc", "libvirt", "qemu.conf")

    @staticmethod
    def restart_libvirtd(ctx, sudo):
        ctx.run(f"{sudo} systemctl restart libvirtd.service")

    @staticmethod
    def assert_user_in_docker_group(ctx):
        ctx.run("cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep 'docker:' | cut -d ':' -f 3)")

    @staticmethod
    def init_local(ctx):
        sudo = "sudo" if not is_root() else ""
        user = getpass.getuser()

        ctx.run(
            f"{sudo} sed --in-place 's/#security_driver = \"selinux\"/security_driver = \"none\"/' {Linux.qemu_conf}"
        )
        ctx.run(f"{sudo} sed --in-place 's/#user = \"root\"/user = \"{user}\"/' {Linux.qemu_conf}")
        ctx.run(f"{sudo} sed --in-place 's/#group = \"root\"/group = \"kvm\"/' {Linux.qemu_conf}")

        Linux.restart_libvirtd(ctx, sudo)


class MacOS:
    kmt_dir = get_home_macos()
    libvirt_group = "staff"
    rootfs_dir = kmt_dir / "rootfs"
    stacks_dir = kmt_dir / "stacks"
    packages_dir = kmt_dir / "kernel-packages"
    libvirt_dir = kmt_dir / "libvirt"
    libvirt_conf = get_homebrew_prefix() / "etc/libvirt/libvirtd.conf"
    shared_dir = Path("/opt/kernel-version-testing")
    libvirt_system_dir = get_homebrew_prefix() / "var/run/libvirt"
    libvirt_socket = f"qemu:///system?socket={libvirt_system_dir}/libvirt-sock"
    virtlogd_conf = get_homebrew_prefix() / "etc/libvirt/virtlogd.conf"

    @staticmethod
    def assert_user_in_docker_group(_):
        return True

    @staticmethod
    def init_local(ctx: Context):
        ctx.run("brew install libvirt")
        ctx.run(
            f"gsed -i -E 's%(# *)?unix_sock_dir = .*%unix_sock_dir = \"{MacOS.libvirt_system_dir}\"%' {MacOS.libvirt_conf}"
        )
        ctx.run(f"gsed -i -E 's%(# *)?unix_sock_ro_perms = .*%unix_sock_ro_perms = \"0777\"%' {MacOS.libvirt_conf}")
        ctx.run(f"gsed -i -E 's%(# *)?unix_sock_rw_perms = .*%unix_sock_rw_perms = \"0777\"%' {MacOS.libvirt_conf}")

        # Enable logging, but only if it was commented (disabled). Do not overwrite
        # custom settings
        log_output_base = "2:file:/opt/homebrew/var/log/libvirt/"
        ctx.run(
            f"gsed -i -E 's%# *log_outputs *=.*%log_outputs = \"{log_output_base}/libvirtd.log\"%' {MacOS.libvirt_conf}"
        )
        ctx.run(
            f"gsed -i -E 's%# *log_outputs *=.*%log_outputs = \"{log_output_base}/virtlogd.log\"%' {MacOS.virtlogd_conf}"
        )

        # libvirt only installs the libvirtd service, it doesn't add virtlogd. We need to create
        # it manually and start it. It's not possible to add the service through homebrew
        # because homebrew only supports one service per formula.
        virtlogd_plist_path = Path("/Library/LaunchDaemons/org.libvirt.virtlogd.plist")

        if not virtlogd_plist_path.exists():
            # Generate a plist file for virtlogd so that we can manage it wiht launchctl.
            # Values for the plist file are taken from the libvirt formula.
            plist_data = {
                "EnvironmentVariables": dict(PATH=os.fspath(get_homebrew_prefix() / "bin")),
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

            # Allow writing the file without superuser permissions
            ctx.sudo(f"touch {virtlogd_plist_path}")
            ctx.sudo(f"chmod 666 {virtlogd_plist_path}")
            with open(virtlogd_plist_path, "wb") as f:
                plistlib.dump(plist_data, f)

            # Now we can set the correct permissions and load the service
            ctx.sudo(f"chmod 644 {virtlogd_plist_path}")
            ctx.sudo(f"launchctl load -w {virtlogd_plist_path}")

        ctx.sudo("launchctl enable system/org.libvirt.virtlogd")
        ctx.sudo(
            "launchctl start system/org.libvirt.virtlogd || true"
        )  # launchctl returns an error code even if there is no error

        ctx.sudo("brew services start libvirt")

        # Enable IP forwarding for the VMs
        ctx.sudo("sysctl -w net.inet.ip.forwarding=1")

        # Enable the bootp service that manages DHCP requests
        # Add || true to the commands because they might fail if it's already loaded/started
        ctx.sudo("launchctl load -w /System/Library/LaunchDaemons/bootps.plist || true")
        ctx.sudo("launchctl start com.apple.bootpd || true")
