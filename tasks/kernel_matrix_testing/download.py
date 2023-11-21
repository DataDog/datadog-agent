import filecmp
import os
import platform
import tempfile
from glob import glob

from .tool import Exit, debug, info, warn

try:
    import requests
except ImportError:
    requests = None

url_base = "https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/rootfs/"
rootfs_amd64 = {
    "custom-bullseye.amd64.qcow2",
    "custom-buster.amd64.qcow2",
    "jammy-server-cloudimg-amd64.qcow2",
    "focal-server-cloudimg-amd64.qcow2",
    "bionic-server-cloudimg-amd64.qcow2",
    "amzn2-kvm-2.0-amd64-4.14.qcow2",
    "amzn2-kvm-2.0-amd64-5.4.qcow2",
    "amzn2-kvm-2.0-amd64-5.10.qcow2",
    "amzn2-kvm-2.0-amd64-5.15.qcow2",
    "Fedora-Cloud-Base-35.amd64.qcow2",
    "Fedora-Cloud-Base-36.amd64.qcow2",
    "Fedora-Cloud-Base-37.amd64.qcow2",
    "Fedora-Cloud-Base-38.amd64.qcow2",
    "debian-10-generic-amd64.qcow2",
    "debian-11-generic-amd64.qcow2",
}

rootfs_arm64 = {
    "custom-bullseye.arm64.qcow2",
    "jammy-server-cloudimg-arm64.qcow2",
    "focal-server-cloudimg-arm64.qcow2",
    "bionic-server-cloudimg-arm64.qcow2",
    "amzn2-kvm-2.0-arm64-4.14.qcow2",
    "amzn2-kvm-2.0-arm64-5.4.qcow2",
    "amzn2-kvm-2.0-arm64-5.10.qcow2",
    "amzn2-kvm-2.0-arm64-5.15.qcow2",
    "Fedora-Cloud-Base-35.arm64.qcow2",
    "Fedora-Cloud-Base-36.arm64.qcow2",
    "Fedora-Cloud-Base-37.arm64.qcow2",
    "Fedora-Cloud-Base-38.arm64.qcow2",
    "debian-10-generic-arm64.qcow2",
    "debian-11-generic-arm64.qcow2",
}

archs_mapping = {
    "amd64": "x86_64",
    "x86": "x86_64",
    "x86_64": "x86_64",
    "arm64": "arm64",
    "aarch64": "arm64",
    "arm": "arm64",
    "local": "local",
}
karch_mapping = {"x86_64": "x86", "arm64": "arm64"}


def requires_update(rootfs_dir, image):
    sum_url = url_base + image + ".sum"
    r = requests.get(sum_url)
    new_sum = r.text.rstrip().split(' ')[0]
    debug(f"[debug] new_sum: {new_sum}")

    if not os.path.exists(os.path.join(rootfs_dir, f"{image}.sum")):
        return True

    with open(os.path.join(rootfs_dir, f"{image}.sum")) as f:
        original_sum = f.read().rstrip().split(' ')[0]
        debug(f"[debug] original_sum: {original_sum}")
    if new_sum != original_sum:
        return True
    return False


def download_rootfs(ctx, rootfs_dir, backup_dir, revert=False):
    to_download = list()
    if archs_mapping[platform.machine()] == "x86_64":
        file_ls = rootfs_amd64
    else:
        file_ls = rootfs_arm64

    # if file does not exist download it.
    for f in file_ls:
        path = os.path.join(rootfs_dir, f)
        if not os.path.exists(path):
            to_download.append(f)

    # download and compare hash sums
    present_files = list(set(file_ls) - set(to_download))
    for f in present_files:
        if requires_update(rootfs_dir, f):
            debug(f"[debug] updating {f} from S3.")
            to_download.append(f)

    if len(to_download) == 0:
        warn("[-] No update required for rootfs images")
        return

    # download files to be updates
    fd, path = tempfile.mkstemp()
    try:
        with os.fdopen(fd, 'w') as tmp:
            for f in to_download:
                info(f"[+] {f} needs to be downloaded")
                # remove this file
                ctx.run(f"rm -f {os.path.join(rootfs_dir, f)}")
                # download package entry
                tmp.write(url_base + f + "\n")
                tmp.write(f" dir={rootfs_dir}\n")
                tmp.write(f" out={f}\n")
                # download sum entry
                tmp.write(url_base + f"{f}.sum\n")
                tmp.write(f" dir={rootfs_dir}\n")
                tmp.write(f" out={f}.sum\n")
            tmp.write("\n")

        ctx.run(f"cat {path}")
        res = ctx.run(f"aria2c -i {path} -j {len(to_download)}")
        if not res.ok:
            if revert:
                revert_rootfs(ctx, rootfs_dir, backup_dir)
            raise Exit("Failed to download image files")
    finally:
        os.remove(path)

    # set permissions
    res = ctx.run(f"find {rootfs_dir} -name \"*qcow*\" -type f -exec chmod 0766 {{}} \\;")
    if not res.ok:
        if revert:
            revert_rootfs(ctx, rootfs_dir, backup_dir)
        raise Exit("Failed to set permissions 0766 to rootfs")


def revert_kernel_packages(ctx, kernel_packages_dir, backup_dir):
    arch = archs_mapping[platform.machine()]
    kernel_packages_tar = f"kernel-packages-{arch}.tar"
    ctx.run(f"rm -rf {kernel_packages_dir}/*")
    ctx.run(f"mv {backup_dir}/{kernel_packages_tar} {kernel_packages_dir}")
    ctx.run(f"tar xvf {kernel_packages_dir}/{kernel_packages_tar} | xargs -i tar xzf {{}}")


def download_kernel_packages(ctx, kernel_packages_dir, kernel_headers_dir, backup_dir, revert=False):
    arch = archs_mapping[platform.machine()]
    kernel_packages_sum = f"kernel-packages-{arch}.sum"
    kernel_packages_tar = f"kernel-packages-{arch}.tar"

    # download kernel packages
    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_tar} -O {kernel_packages_dir}/{kernel_packages_tar}",
        warn=True,
    )
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx, kernel_packages_dir, backup_dir)
        raise Exit("Failed to download kernel pacakges")

    res = ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_sum} -O {kernel_packages_dir}/{kernel_packages_sum}",
        warn=True,
    )
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx, kernel_packages_dir, backup_dir)
        raise Exit("Failed to download kernel pacakges checksum")

    # extract pacakges
    res = ctx.run(f"cd {kernel_packages_dir} && tar xvf {kernel_packages_tar} | xargs -i tar xzf {{}}")
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx, kernel_packages_dir, backup_dir)
        raise Exit("Failed to extract kernel packages")

    # set permissions
    packages = glob(f"{kernel_packages_dir}/kernel-v*")
    for pkg in packages:
        if not os.path.isdir(pkg):
            continue
        # set package dir as rwx for all
        os.chmod(pkg, 0o766)
        files = glob(f"{pkg}/*")
        for f in files:
            if not os.path.isdir(f):
                # set all files to rw for all
                os.chmod(f, 0o666)

    # copy headers
    res = ctx.run(
        f"find {kernel_packages_dir} -name 'linux-image-*' -type f | xargs -i cp {{}} {kernel_headers_dir} && find {kernel_packages_dir} -name 'linux-headers-*' -type f | xargs -i cp {{}} {kernel_headers_dir}"
    )
    if not res.ok:
        if revert:
            revert_kernel_packages(ctx, kernel_packages_dir, backup_dir)
        raise Exit(f"failed to copy kernel headers to shared dir {kernel_headers_dir}")


def update_kernel_packages(ctx, kernel_packages_dir, kernel_headers_dir, backup_dir, no_backup):
    arch = archs_mapping[platform.machine()]
    kernel_packages_sum = f"kernel-packages-{arch}.sum"
    kernel_packages_tar = f"kernel-packages-{arch}.tar"

    ctx.run(
        f"wget -q https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/{kernel_packages_sum} -O /tmp/{kernel_packages_sum}"
    )

    current_sum_file = f"{kernel_packages_dir}/{kernel_packages_sum}"
    if os.path.exists(current_sum_file) and filecmp.cmp(current_sum_file, f"/tmp/{kernel_packages_sum}"):
        warn("[-] No update required for custom kernel packages")
        return

    # backup kernel-packges
    if not no_backup:
        karch = karch_mapping[archs_mapping[platform.machine()]]
        ctx.run(
            f"find {kernel_packages_dir} -name \"kernel-*.{karch}.pkg.tar.gz\" -type f | rev | cut -d '/' -f 1  | rev > /tmp/package.ls"
        )
        ctx.run(f"cd {kernel_packages_dir} && tar -cf {kernel_packages_tar} -T /tmp/package.ls")
        ctx.run(f"cp {kernel_packages_dir}/{kernel_packages_tar} {backup_dir}")
        info("[+] Backed up current kernel packages")

    # clean kernel packages directory
    ctx.run(f"rm -rf {kernel_packages_dir}/*")

    download_kernel_packages(ctx, kernel_packages_dir, kernel_headers_dir, backup_dir, revert=True)

    info("[+] Kernel packages successfully updated")


def revert_rootfs(ctx, rootfs_dir, backup_dir):
    ctx.run(f"rm -f {rootfs_dir}/*")
    ctx.run(f"find {backup_dir} -name *qcow2 -type f -exec mv {{}} {rootfs_dir}/ \\;")


def update_rootfs(ctx, rootfs_dir, backup_dir, no_backup):
    # backup rootfs
    if not no_backup:
        ctx.run(f"find {rootfs_dir} -name *qcow2 -type f -exec cp {{}} {backup_dir}/ \\;")
        info("[+] Backed up rootfs")

    download_rootfs(ctx, rootfs_dir, backup_dir, revert=True)

    info("[+] Root filesystem and bootables images updated")
