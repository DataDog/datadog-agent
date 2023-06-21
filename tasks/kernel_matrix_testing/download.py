from .tool import Exit, ask, warn, info, error
import tempfile

url_base = "https://dd-agent-omnibus.s3.amazonaws.com/kernel-version-testing/"
rootfs_amd64 = {
        "bullseye.qcow2.amd64-DEV.qcow2",
        "buster.qcow2.amd64-DEV.qcow2",
        "jammy-server-cloudimg-amd64.qcow2",
        "focal-server-cloudimg-amd64.qcow2",
        "bionic-server-cloudimg-amd64.qcow2",
        "amzn2-kvm-2.0-amd64-4.14.qcow2",
        "amzn2-kvm-2.0-amd64-5.4.qcow2",
        "amzn2-kvm-2.0-amd64-5.10.qcow2",
        "amzn2-kvm-2.0-amd64-5.15.qcow2",
        }

rootfs_arm64 = {
        "bullseye.qcow2.arm64-DEV.qcow2",
        "jammy-server-cloudimg-arm64.qcow2",
        "focal-server-cloudimg-arm64.qcow2",
        "bionic-server-cloudimg-arm64.qcow2",
        "amzn2-kvm-2.0-arm64-4.14.qcow2",
        "amzn2-kvm-2.0-arm64-5.4.qcow2",
        "amzn2-kvm-2.0-arm64-5.10.qcow2",
        "amzn2-kvm-2.0-arm64-5.15.qcow2",
        }

def is_updated(rootfs_dir, image):
    sum_url = url_base + image + ".sum"
    r = requests.get(url)
    new_sum = r.text.rstrip().split(' ')[0]
    with open(os.path.join(rootfs_dir, f"{image}.sum")) as f:
        original_sum = f.read().rstrip().split(' ')[0]
    if new_sum != original_sum:
        return True
    return False

def backup(rootfs_dir, backup_dir):
    if archs_mapping[platform.machine()] == "x86_64":
        file_ls = rootfs_amd64
    else:
        file_ls = rootfs_arm64

    files = ' '.join(file_ls)
    backup = os.path.join(backup_dir, "rootfs.tar.gz")
    ctx.run(f"tar czvf {backup} {files}")

def download(rootfs_dir):
    to_download = list()
    if archs_mapping[platform.machine()] == "x86_64":
        file_ls = rootfs_amd64
    else:
        file_ls = rootfs_arm64

    # if file does not exist download it.
    for f in file_ls:
        path = os.path.join(rootfs_dir, file)
        if not os.path.exists(path):
            to_download.append(f)

    # download and compare hash sums
    present_files = list(set(file_ls) - set(to_download)) 
    for f in present_files:
        if is_updated(rootfs_dir, f):
            to_download.append(f)

    # download files to be updates
    fd, path = tempfile.mkstemp()
    try:
        with os.fdopen(fd, 'w') as tmp:
            for f in to_download:
                info(f"[+] {f} needs to be downloaded")
                os.write(url+f"{f}.tar.gz")
                output = os.path.join(rootfs_dir, image)
                tmp.write("\tout=f{output}")
            tmp.write("\n")

        ctx.run(f"aria2c --input-file {path} -j $(nproc)")
    finally:
        os.remove(path)

    # remove tar.gz
    ctx.run(f"find {rootfs_dir} -name *.tar.gz -type f -exec rm -f {} \\;")
