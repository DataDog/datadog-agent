import getpass
import sys
from datetime import datetime
from pathlib import Path

from invoke.context import Context

from tasks.kernel_matrix_testing.tool import Exit, info


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
    user_group = getpass.getuser()
    libvirt_group = "libvirt"
    rootfs_dir = kmt_dir / "rootfs"
    stacks_dir = kmt_dir / "stacks"
    packages_dir = kmt_dir / "kernel-packages"
    libvirt_dir = kmt_dir / "libvirt"
    shared_dir = Path("/opt/kernel-version-testing")
    libvirt_socket = "qemu:///system"
    ddvm_rsa = kmt_dir / "ddvm_rsa"
    config_path = kmt_dir / "config.json"

    qemu_conf = Path("/etc/libvirt/qemu.conf")

    @staticmethod
    def flare(ctx: Context, flare_folder: Path):
        ctx.run(f"dpkg -l > {flare_folder / 'packages.txt'}", warn=True)
        ctx.run(f"ip r > {flare_folder / 'ip_r.txt'}", warn=True)
        ctx.run(f"ip a > {flare_folder / 'ip_a.txt'}", warn=True)
        ctx.run(f"sudo cp /etc/libvirt/qemu.conf {flare_folder / 'qemu.conf'}", warn=True)
        ctx.run(f"sudo ls -lhR /home/kernel-version-testing/ > {flare_folder / 'kmt.files'}", warn=True)


class MacOS:
    kmt_dir = get_home_macos()
    name = "macos"
    user_group = "staff"
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
    config_path = kmt_dir / "config.json"

    @staticmethod
    def flare(ctx: Context, flare_folder: Path):
        ctx.run(f"brew list > {flare_folder / 'brew_libvirt.txt'}", warn=True)
        ctx.run(f"netstat -an > {flare_folder / 'netstat.txt'}", warn=True)
        ctx.run(f"ifconfig -a > {flare_folder / 'ifconfig.txt'}", warn=True)
        ctx.run(f"cp -v /var/db/dhcpd_leases {flare_folder / 'dhcpd_leases'}", warn=True)


def flare(ctx: Context, tmp_flare_folder: Path, dest_folder: Path, keep_uncompressed_files: bool = False):
    kmt_os = get_kmt_os()
    kmt_os.flare(ctx, tmp_flare_folder)

    ctx.run(f"git rev-parse HEAD > {tmp_flare_folder}/git_version.txt", warn=True)
    ctx.run(f"git rev-parse --abbrev-ref HEAD >> {tmp_flare_folder}/git_version.txt", warn=True)
    ctx.run(f"mkdir -p {tmp_flare_folder}/stacks")
    ctx.run(f"cp -r {kmt_os.stacks_dir} {tmp_flare_folder}/stacks", warn=True)
    ctx.run(f"sudo virsh list --all > {tmp_flare_folder}/virsh_list.txt", warn=True)
    ctx.run(f"cp {kmt_os.config_path} {tmp_flare_folder}/config.json", warn=True)

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
