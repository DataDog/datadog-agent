import getpass
import os
import platform
import plistlib
import sys
from datetime import datetime
from pathlib import Path

from invoke.context import Context

from tasks.kernel_matrix_testing.tool import Exit, info
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
    name = "linux"
    libvirt_group = "libvirt"
    rootfs_dir = kmt_dir / "rootfs"
    stacks_dir = kmt_dir / "stacks"
    packages_dir = kmt_dir / "kernel-packages"
    libvirt_dir = kmt_dir / "libvirt"
    shared_dir = Path("/opt/kernel-version-testing")
    libvirt_socket = "qemu:///system"
    ddvm_rsa = kmt_dir / "ddvm_rsa"

    qemu_conf = os.path.join("/", "etc", "libvirt", "qemu.conf")

    packages = [
        "aria2",
        "fio",
        "socat",
        "qemu-kvm",
        "libvirt-daemon-system",
        "curl",
        "debootstrap",
        "libguestfs-tools",
        "libvirt-dev",
        "python3-pip",
        "nfs-kernel-server",
        "rpcbind",
        "ssh-askpass",
        "xsltproc",
    ]

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

        with open("/etc/exports") as f:
            if "/opt/kernel-version-testing 100.0.0.0/8(ro,no_root_squash,no_subtree_check)" not in f.read():
                ctx.run(
                    f"{sudo} sh -c 'echo \"/opt/kernel-version-testing 100.0.0.0/8(ro,no_root_squash,no_subtree_check)\" >> /etc/exports'"
                )
                ctx.run(f"{sudo} exportfs -a")

        Linux.restart_libvirtd(ctx, sudo)

    @staticmethod
    def install_requirements(ctx: Context):
        ctx.run("sudo apt update")
        ctx.run(f"sudo apt install -y {' '.join(Linux.packages)}")

        if platform.machine() == "aarch64":
            ctx.run("sudo apt install -y qemu-efi-aarch64")

        ctx.run("sudo systemctl start nfs-kernel-server.service")

    @staticmethod
    def flare(ctx: Context, flare_folder: Path):
        ctx.run(f"apt-cache policy{' '.join(Linux.packages)} > {flare_folder / 'packages.txt'}", warn=True)
        ctx.run(f"ip r > {flare_folder / 'ip_r.txt'}", warn=True)
        ctx.run(f"ip a > {flare_folder / 'ip_a.txt'}", warn=True)


class MacOS:
    kmt_dir = get_home_macos()
    name = "macos"
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
    ddvm_rsa = kmt_dir / "ddvm_rsa"

    packages = ["aria2", "fio", "socat", "libvirt", "gnu-sed", "qemu", "libvirt", "wget"]

    @staticmethod
    def assert_user_in_docker_group(_):
        return True

    @staticmethod
    def init_local(ctx: Context):
        # Configure libvirt sockets
        ctx.run(
            f"gsed -i -E 's%(# *)?unix_sock_dir = .*%unix_sock_dir = \"{MacOS.libvirt_system_dir}\"%' {MacOS.libvirt_conf}"
        )
        ctx.run(f"gsed -i -E 's%(# *)?unix_sock_ro_perms = .*%unix_sock_ro_perms = \"0777\"%' {MacOS.libvirt_conf}")
        ctx.run(f"gsed -i -E 's%(# *)?unix_sock_rw_perms = .*%unix_sock_rw_perms = \"0777\"%' {MacOS.libvirt_conf}")

        # Configure default socket URI for libvirt
        ctx.run(
            f"gsed -i -E 's%(# *)?uri_default = .*%uri_default = \"{MacOS.libvirt_socket}\"%' {get_homebrew_prefix() / 'etc/libvirt/libvirt.conf'}"
        )

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

            # Allow writing the file without superuser permissions
            ctx.run(f"sudo touch {virtlogd_plist_path}")
            ctx.run(f"sudo chmod 666 {virtlogd_plist_path}")
            with open(virtlogd_plist_path, "wb") as f:
                plistlib.dump(plist_data, f)

            # Now we can set the correct permissions and load the service
            ctx.run(f"sudo chmod 644 {virtlogd_plist_path}")
            ctx.run(f"sudo launchctl load -w {virtlogd_plist_path}")

        ctx.run("sudo launchctl enable system/org.libvirt.virtlogd")
        ctx.run(
            "sudo launchctl start system/org.libvirt.virtlogd || true"
        )  # launchctl returns an error code even if there is no error

        ctx.run("sudo brew services start libvirt")

        # Enable IP forwarding for the VMs
        ctx.run("sudo sysctl -w net.inet.ip.forwarding=1")

        # Enable the bootp service that manages DHCP requests
        # Add || true to the commands because they might fail if it's already loaded/started
        ctx.run("sudo launchctl load -w /System/Library/LaunchDaemons/bootps.plist || true")
        ctx.run("sudo launchctl start com.apple.bootpd || true")

        # Configure sharing of the kmt directory
        ctx.run(f"sudo mkdir -p {MacOS.shared_dir}")

        exports_file = Path("/etc/exports")

        if not exports_file.exists() or os.fspath(MacOS.shared_dir) not in exports_file.read_text():
            ctx.run(
                f"echo '/opt/kernel-version-testing -network 192.168.0.0 -mask 255.255.0.0' | sudo tee -a {exports_file}"
            )

        ctx.run("sudo nfsd enable || true")
        ctx.run("sudo nfsd update")

    @staticmethod
    def install_requirements(ctx: Context):
        ctx.run("brew install " + " ".join(MacOS.packages))

    @staticmethod
    def flare(ctx: Context, flare_folder: Path):
        ctx.run(f"brew list {' '.join(MacOS.packages)} > {flare_folder / 'brew_libvirt.txt'}", warn=True)
        ctx.run(f"netstat -an > {flare_folder / 'netstat.txt'}", warn=True)
        ctx.run(f"ifconfig -a > {flare_folder / 'ifconfig.txt'}", warn=True)


def flare(ctx: Context, tmp_flare_folder: Path, dest_folder: Path, keep_uncompressed_files: bool = False):
    kmt_os = get_kmt_os()
    kmt_os.flare(ctx, tmp_flare_folder)

    ctx.run(f"git rev-parse HEAD > {tmp_flare_folder}/git_version.txt", warn=True)
    ctx.run(f"git rev-parse --abbrev-ref HEAD >> {tmp_flare_folder}/git_version.txt", warn=True)
    ctx.run(f"mkdir -p {tmp_flare_folder}/stacks")
    ctx.run(f"cp -r {kmt_os.stacks_dir} {tmp_flare_folder}/stacks", warn=True)
    ctx.run(f"sudo virsh list --all > {tmp_flare_folder}/virsh_list.txt", warn=True)

    virsh_config_folder = tmp_flare_folder / 'vm-configs'
    ctx.run(f"mkdir -p {virsh_config_folder}", warn=True)
    ctx.run(
        f"sudo virsh list --all | grep ddvm | awk '{{ print $2 }}' | xargs -I % -S 1024 /bin/bash -c \"sudo virsh dumpxml % > {virsh_config_folder}/%.xml\"",
        warn=True,
    )
    ctx.run(f"ps aux | grep -i appgate | grep -v grep > {tmp_flare_folder}/ps_appgate.txt", warn=True)
    ctx.run(f"sudo chmod a+rw {tmp_flare_folder}/*", warn=True)

    ctx.run(f"mkdir -p {dest_folder}")
    now_ts = datetime.now().strftime("%Y%m%d-%H%M%S")
    flare_fname = f"kmt_flare_{now_ts}.tar.gz"
    flare_path = dest_folder / flare_fname
    ctx.run(f"tar -C {tmp_flare_folder} -czf {flare_path} .")

    info(f"[+] Flare saved to {flare_path}")

    if keep_uncompressed_files:
        flare_dest_folder = dest_folder / f"kmt_flare_{now_ts}"
        ctx.run(f"mkdir -p {flare_dest_folder}")
        ctx.run(f"cp -r {tmp_flare_folder}/* {flare_dest_folder}")
        info(f"[+] Flare uncompressed contents saved to {flare_dest_folder}")
