import filecmp
import json
import os
import platform
import tempfile
from glob import glob

from .tool import Exit, debug, info, warn

try:
    import requests
except ImportError:
    requests = None

platforms_file = "test/new-e2e/system-probe/config/platforms.json"
vmconfig_file = "test/new-e2e/system-probe/config/vmconfig.json"

arch_mapping = {
    "amd64": "x86_64",
    "x86": "x86_64",
    "x86_64": "x86_64",
    "arm64": "arm64",
    "arm": "arm64",
    "aarch64": "arm64",
}


def requires_update(url_base, rootfs_dir, image):
    sum_url = os.path.join(url_base, "master", image + ".sum")
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
    with open(platforms_file) as f:
        platforms = json.load(f)

    with open(vmconfig_file) as f:
        vmconfig_template = json.load(f)

    url_base = platforms["url_base"]

    arch = arch_mapping[platform.machine()]
    to_download = list()
    file_ls = list()
    for tag in platforms[arch]:
        path = os.path.basename(platforms[arch][tag])
        if path.endswith(".xz"):
            path = path[: -len(".xz")]

        file_ls.append(os.path.basename(path))

    # if file does not exist download it.
    for f in file_ls:
        path = os.path.join(rootfs_dir, f)
        if not os.path.exists(path):
            to_download.append(f)

    disks_to_download = list()
    for vmset in vmconfig_template["vmsets"]:
        if vmset["arch"] != arch:
            continue

        for disk in vmset["disks"]:
            d = os.path.basename(disk["source"])
            if not os.path.exists(os.path.join(rootfs_dir, d)):
                disks_to_download.append(d)

    # download and compare hash sums
    present_files = list(set(file_ls) - set(to_download)) + disks_to_download
    for f in present_files:
        if requires_update(url_base, rootfs_dir, f):
            debug(f"[debug] updating {f} from S3.")
            ctx.run(f"rm -f {f}")
            ctx.run(f"rm -f {f}.sum")
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
                xz = ".xz" if f not in disks_to_download else ""
                filename = f"{f}{xz}"
                sum_file = f"{f}.sum"
                # remove this file and sum
                ctx.run(f"rm -f {os.path.join(rootfs_dir, filename)}")
                ctx.run(f"rm -f {os.path.join(rootfs_dir, sum_file)}")
                # download package entry
                tmp.write(os.path.join(url_base, "master", filename) + "\n")
                tmp.write(f" dir={rootfs_dir}\n")
                tmp.write(f" out={filename}\n")
                # download sum entry
                tmp.write(os.path.join(url_base, "master", f"{sum_file}") + "\n")
                tmp.write(f" dir={rootfs_dir}\n")
                tmp.write(f" out={sum_file}\n")
            tmp.write("\n")
        ctx.run(f"cat {path}")
        res = ctx.run(f"aria2c -i {path} -j {len(to_download)}")
        if not res.ok:
            if revert:
                revert_rootfs(ctx, rootfs_dir, backup_dir)
            raise Exit("Failed to download image files")
    finally:
        os.remove(path)

    # extract files
    res = ctx.run(f"find {rootfs_dir} -name \"*qcow2.xz\" -type f -exec xz -d {{}} \\;")
    if not res.ok:
        if revert:
            revert_rootfs(ctx, rootfs_dir, backup_dir)
        raise Exit("Failed to extract qcow2 files")

    # set permissions
    res = ctx.run(f"find {rootfs_dir} -name \"*qcow*\" -type f -exec chmod 0766 {{}} \\;")
    if not res.ok:
        if revert:
            revert_rootfs(ctx, rootfs_dir, backup_dir)
        raise Exit("Failed to set permissions 0766 to rootfs")


def revert_kernel_packages(ctx, kernel_packages_dir, backup_dir):
    arch = arch_mapping[platform.machine()]
    kernel_packages_tar = f"kernel-packages-{arch}.tar"
    ctx.run(f"rm -rf {kernel_packages_dir}/*")
    ctx.run(f"mv {backup_dir}/{kernel_packages_tar} {kernel_packages_dir}")
    ctx.run(f"tar xvf {kernel_packages_dir}/{kernel_packages_tar} | xargs -i tar xzf {{}}")


def download_kernel_packages(ctx, kernel_packages_dir, kernel_headers_dir, backup_dir, revert=False):
    arch = arch_mapping[platform.machine()]
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
    arch = arch_mapping[platform.machine()]
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
        karch = arch_mapping[platform.machine()]
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
