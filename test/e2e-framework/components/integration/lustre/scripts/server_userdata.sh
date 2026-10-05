#!/bin/bash
# Lustre 2.15.6 server (combined MGS+MDS+OSS, ldiskfs) bootstrap for EL8.10.
#
# REBOOT REQUIRED: the Whamcloud ldiskfs OSD kmods are built for the PATCHED
# kernel 4.18.0-553.27.1.el8_lustre (the .ko live under that kernel's extra/
# dir; weak-updates for the running z-stream kernel is empty), so the server
# MUST boot that kernel before mkfs.lustre can create ldiskfs targets.
#
# This runs as EC2 cloud-init user-data on FIRST boot (stock kernel), BEFORE the
# framework's first SSH. It installs the patched kernel + a boot-time systemd
# oneshot (lustre-setup.service) that does the real format/mount, then reboots
# into the patched kernel. cloud-init's `status --wait` (the framework host
# readiness) blocks across this reboot, so no Pulumi remote command is ever
# severed mid-run. The client self-waits for the server via lctl ping + retry
# mount, so the server needs no server-side Pulumi command at all.
set -euo pipefail

LUSTRE_FSNAME="${LUSTRE_FSNAME:-lustrefs}"
LUSTRE_LNET_IFACE="${LUSTRE_LNET_IFACE:-eth0}"
LUSTRE_VERSION="2.15.6"
LUSTRE_PATCHED_KERNEL="4.18.0-553.27.1.el8_lustre.x86_64"
WC_BASE="https://downloads.whamcloud.com/public"

diag() {
	echo "===== lustre server phase-1 FAILED, diagnostics =====" >&2
	uname -r >&2 || true
	cat /etc/redhat-release 2>/dev/null >&2 || true
	rpm -qa 'kernel*' 'lustre*' 'kmod*' 'e2fsprogs*' 2>/dev/null >&2 || true
	dnf -q repolist >&2 2>&1 || true
	dmesg | tail -n 40 >&2 || true
}
trap diag ERR

# ----- Repos: whamcloud lustre server + e2fsprogs-wc -----
dnf install -y wget || true
cat >/etc/yum.repos.d/lustre.repo <<EOF
[lustre-server]
name=lustre-server
baseurl=${WC_BASE}/lustre/lustre-${LUSTRE_VERSION}/el8.10/server/
enabled=1
gpgcheck=0

[e2fsprogs-wc]
name=e2fsprogs-wc
baseurl=${WC_BASE}/e2fsprogs/latest/el8/
enabled=1
gpgcheck=0
EOF

# ----- Patched kernel + ldiskfs OSD kmods + tools -----
dnf install -y --nogpgcheck \
	"kernel-${LUSTRE_PATCHED_KERNEL%.x86_64}" "kernel-devel-${LUSTRE_PATCHED_KERNEL%.x86_64}" \
	kmod-lustre kmod-lustre-osd-ldiskfs lustre lustre-osd-ldiskfs-mount e2fsprogs

# Never let a dnf update float the kernel away from the patched build the kmods
# were compiled against.
dnf install -y 'dnf-command(versionlock)' || dnf install -y python3-dnf-plugin-versionlock || true
dnf versionlock add 'kernel*' 'kmod-lustre*' 'lustre*' || true

# Boot the patched kernel by default.
grubby --set-default "/boot/vmlinuz-${LUSTRE_PATCHED_KERNEL}" || true

# ----- Persist config for the boot-time setup service -----
install -d -m 0755 /var/lib/lustre-lab
cat >/etc/default/lustre-lab <<EOF
LUSTRE_FSNAME="${LUSTRE_FSNAME}"
LUSTRE_LNET_IFACE="${LUSTRE_LNET_IFACE}"
LUSTRE_PATCHED_KERNEL="${LUSTRE_PATCHED_KERNEL}"
EOF

# ----- Real setup, owned by a boot-time oneshot (handles the reboot itself) -----
cat >/usr/local/sbin/lustre-setup.sh <<'SETUP'
#!/bin/bash
# Boot-time Lustre server setup. Runs on every boot until the ready marker
# exists. On the wrong kernel it re-points grub and reboots; on the patched
# kernel it loads modules, configures LNet, formats + mounts the targets.
set -uo pipefail
exec >>/var/lib/lustre-lab/setup.log 2>&1
echo "===== lustre-setup $(date -u) on kernel $(uname -r) ====="

# shellcheck disable=SC1091
[ -f /etc/default/lustre-lab ] && . /etc/default/lustre-lab
LUSTRE_FSNAME="${LUSTRE_FSNAME:-lustrefs}"
LUSTRE_LNET_IFACE="${LUSTRE_LNET_IFACE:-eth0}"
LUSTRE_PATCHED_KERNEL="${LUSTRE_PATCHED_KERNEL:-4.18.0-553.27.1.el8_lustre.x86_64}"

if [ -f /var/lib/lustre-lab/ready ]; then
	echo "already ready; nothing to do"
	exit 0
fi

# Wrong kernel: re-point grub and reboot into the patched kernel. Sleep first so
# the phase-1 SSH command that started us has time to return exit 0 cleanly.
if [ "$(uname -r)" != "${LUSTRE_PATCHED_KERNEL}" ]; then
	echo "running $(uname -r), need ${LUSTRE_PATCHED_KERNEL}; rebooting"
	grubby --set-default "/boot/vmlinuz-${LUSTRE_PATCHED_KERNEL}" || true
	sleep 5
	systemctl reboot
	exit 0
fi

set -e
trap 'echo "lustre-setup FAILED on $(uname -r)"; uname -r; rpm -qa "kernel*" "lustre*" "kmod*"; lsmod | grep -E "lustre|ldiskfs|lnet"; dmesg | tail -n 80' ERR

cat >/etc/modprobe.d/lnet.conf <<CONF
options lnet networks=tcp0(${LUSTRE_LNET_IFACE})
CONF
modprobe lnet
modprobe lustre
# ldiskfs OSD module (kmod exposes ldiskfs or osd_ldiskfs depending on build).
if ! modprobe ldiskfs 2>/dev/null && ! modprobe osd_ldiskfs 2>/dev/null; then
	echo "===== ldiskfs OSD unavailable for kernel $(uname -r) ====="
	rpm -ql kmod-lustre-osd-ldiskfs 2>/dev/null | grep -E '\.ko' || true
	find /lib/modules -name 'osd_ldiskfs.ko*' -o -name 'ldiskfs.ko*' 2>/dev/null || true
	ls -la "/lib/modules/$(uname -r)/weak-updates/" 2>/dev/null || true
	modinfo osd_ldiskfs 2>&1 | head -5 || true
	modinfo ldiskfs 2>&1 | head -5 || true
	exit 1
fi
lnetctl lnet configure || true
lnetctl net add --net tcp0 --if "${LUSTRE_LNET_IFACE}" || true

server_ip="$(ip -4 -o addr show dev "${LUSTRE_LNET_IFACE}" | awk '{print $4}' | cut -d/ -f1 | head -n1)"
echo "server ip on ${LUSTRE_LNET_IFACE}: ${server_ip}"

# Loop-backed ldiskfs targets. mkfs.lustre cannot size a plain file, so attach
# explicit loop block devices with losetup and mkfs/mount the block device.
mkdir -p /lustre
[ -f /lustre/mgtmdt.img ] || truncate -s 8G /lustre/mgtmdt.img
[ -f /lustre/ost0.img ] || truncate -s 20G /lustre/ost0.img
modprobe loop || true
attach_loop() {
	local img="$1" dev
	dev="$(losetup -j "$img" -O NAME --noheadings 2>/dev/null | awk 'NF{print $1; exit}')"
	[ -n "$dev" ] || dev="$(losetup -f --show "$img")"
	echo "$dev"
}
mgt_loop="$(attach_loop /lustre/mgtmdt.img)"
ost_loop="$(attach_loop /lustre/ost0.img)"
echo "mgt loop: ${mgt_loop}, ost loop: ${ost_loop}"

mkfs.lustre --fsname="${LUSTRE_FSNAME}" --mgs --mdt --index=0 --reformat "$mgt_loop"
mkfs.lustre --fsname="${LUSTRE_FSNAME}" --ost --index=0 \
	--mgsnode="${server_ip}@tcp" --reformat "$ost_loop"

mkdir -p /mnt/mgtmdt /mnt/ost0
mountpoint -q /mnt/mgtmdt || mount -t lustre "$mgt_loop" /mnt/mgtmdt
mountpoint -q /mnt/ost0 || mount -t lustre "$ost_loop" /mnt/ost0

lctl dl
touch /var/lib/lustre-lab/ready
echo "lustre server ready: fsname=${LUSTRE_FSNAME}"
SETUP
chmod +x /usr/local/sbin/lustre-setup.sh

# ----- Oneshot unit: runs on every boot until ready exists -----
cat >/etc/systemd/system/lustre-setup.service <<EOF
[Unit]
Description=Lustre lab server setup (boot-time, reboot-aware)
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/sbin/lustre-setup.sh

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable lustre-setup.service

# Reboot into the patched kernel — but ONLY after cloud-init reports "done", so
# the framework's `cloud-init status --wait` host-readiness returns 0 first and is
# not severed mid-run (rebooting during cloud-init kills that SSH command). A
# detached waiter polls cloud-init status, then reboots. On the next boot
# lustre-setup.service formats + mounts and writes the ready marker. The server
# has no Pulumi remote command, so this reboot severs nothing.
echo "lustre server user-data complete; scheduling patched-kernel reboot after cloud-init done"
# shellcheck disable=SC2016  # $ expressions are intentionally evaluated by the detached child shell, not now
nohup bash -c '
	for _ in $(seq 1 180); do
		cloud-init status 2>/dev/null | grep -q "status: done" && break
		sleep 5
	done
	sleep 5
	systemctl reboot
' >/var/lib/lustre-lab/reboot-waiter.log 2>&1 &
disown || true
