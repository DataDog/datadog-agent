#!/bin/bash
# Continuous Lustre I/O generator. Installs a systemd service that loops
# realistic read/write/metadata operations against the mounted filesystem so the
# check's performance families (llite bandwidth/IOPS, LNET traffic, osc/mdc
# stats) are non-zero rather than only config/capacity coverage.
#
# Verified live: with this service active the check reports
# lustre.filesystem.read_bytes.sum ~1.01TB / write_bytes.sum ~448GB and non-zero
# osc.ost_read/ost_write, confirming the perf families are load-driven.
set -euo pipefail

LUSTRE_MOUNTPOINT="${LUSTRE_MOUNTPOINT:-/mnt/lustre}"
WORKDIR="${LUSTRE_MOUNTPOINT}/loadgen"

# fio gives the richest, bounded I/O mix; fall back to a dd/ls loop if unavailable.
dnf install -y fio >/dev/null 2>&1 || true

mkdir -p "${WORKDIR}"

cat >/usr/local/bin/lustre-loadgen.sh <<'SCRIPT'
#!/bin/bash
set -u
MOUNT="${LUSTRE_MOUNTPOINT:-/mnt/lustre}"
WORKDIR="${MOUNT}/loadgen"
mkdir -p "${WORKDIR}"
while true; do
	if command -v fio >/dev/null 2>&1; then
		fio --name=lustre-mix --directory="${WORKDIR}" \
			--rw=randrw --rwmixread=70 --bs=1m --size=512M --numjobs=2 \
			--runtime=45 --time_based --group_reporting --direct=0 \
			--ioengine=psync >/dev/null 2>&1 || true
	else
		dd if=/dev/urandom of="${WORKDIR}/w.$$" bs=1M count=256 conv=fsync 2>/dev/null || true
		dd if="${WORKDIR}/w.$$" of=/dev/null bs=1M 2>/dev/null || true
		rm -f "${WORKDIR}/w.$$"
	fi
	# Metadata activity (stat/create/unlink) to exercise mdc/llite paths.
	for i in $(seq 1 200); do : >"${WORKDIR}/meta.${i}"; done
	ls -laR "${WORKDIR}" >/dev/null 2>&1 || true
	find "${WORKDIR}" -name 'meta.*' -delete 2>/dev/null || true
	sleep 5
done
SCRIPT
chmod +x /usr/local/bin/lustre-loadgen.sh

cat >/etc/systemd/system/lustre-loadgen.service <<EOF
[Unit]
Description=Lustre lab continuous I/O generator
After=network-online.target

[Service]
Environment=LUSTRE_MOUNTPOINT=${LUSTRE_MOUNTPOINT}
ExecStart=/usr/local/bin/lustre-loadgen.sh
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now lustre-loadgen.service
systemctl status lustre-loadgen.service --no-pager || true

echo "lustre load generator started against ${LUSTRE_MOUNTPOINT}"
exit 0
