#!/usr/bin/env bash
#
# Dell PowerFlex nested-cluster bootstrap (DEFERRED / operator-driven).
#
# This script is STAGED onto the m5.metal host by the dell-powerflex component
# but is intentionally NOT executed automatically. The steps below depend on
# live-only unknowns (PFMP appliance console mode, install_PFMP.sh prompts,
# single-node PFMP support, scli cert approvals) that must be resolved
# interactively during live exploration before any of this can be automated.
#
# Authoritative runbook:
#   .agint/labs/dell-powerflex/research/metal-nested.md
#
# Prerequisites already provided by the component (InstallVirtStack):
#   - qemu-kvm / libvirt / virt-install / libguestfs-tools installed
#   - libvirtd enabled, /dev/kvm present
#   - libvirt NAT network "pflex" (10.55.0.0/24) with PFMP reserved at
#     10.55.0.20 (MAC 52:54:00:55:00:20)
#
# Run interactively with sudo, one phase at a time. Each phase is a TODO until
# confirmed live; do not run end-to-end unattended.

set -euo pipefail

PFMP_IP="10.55.0.20"
NET="pflex"
IMG_DIR="/var/lib/libvirt/images"
# Presigned S3 URLs are generated LOCALLY (aws-vault) and passed in as env vars;
# the instance profile is not assumed to have S3 access.
: "${PFMP_OVA_URL:?set PFMP_OVA_URL to a presigned https URL for s3://dd-vmimport-custom-ami-bucket/pfmp/pfmp-k8s-155.ova}"
: "${EL9_RPMS_URL:?set EL9_RPMS_URL to a presigned https URL (or prefix) for s3://dd-vmimport-custom-ami-bucket/powerflex-rpms/el9/}"

phase_stage_ova() {
  # TODO(live): pull the OVA + el9 RPMs onto the EBS root.
  echo "[stage] downloading PFMP OVA"
  sudo curl -fL "$PFMP_OVA_URL" -o "${IMG_DIR}/pfmp-k8s-155.ova"
  # TODO(live): fetch el9 RPMs (mdm/sds/sdc/sdr/sdt/lia/activemq) to /root/pflex-rpms.
}

phase_convert_ova() {
  # TODO(live): unpack OVA, convert the appliance vmdk -> qcow2 on the EBS root.
  echo "[convert] OVA -> qcow2"
  cd "$IMG_DIR"
  sudo tar -xvf pfmp-k8s-155.ova
  # Disk filename varies; confirm live, then:
  # sudo qemu-img convert -O qcow2 pfmp-k8s-155-disk1.vmdk pfmp.qcow2
}

phase_define_pfmp() {
  # TODO(live): define + start the PFMP domain. OVF has NO ProductSection, so
  # there is zero vSphere guestinfo: network must come from NAT DHCP or a
  # NoCloud cidata drive, never guestinfo. Attach BOTH serial pty and VNC
  # consoles (console behavior unknown). NIC MAC must be 52:54:00:55:00:20 so
  # PFMP gets the reserved 10.55.0.20.
  echo "[define] PFMP domain on net ${NET}, reserved ${PFMP_IP}"
  # sudo virt-install --import --name pfmp --memory 32768 --vcpus 8 \
  #   --disk "${IMG_DIR}/pfmp.qcow2",bus=scsi --controller scsi,model=virtio-scsi \
  #   --network network=${NET},mac=52:54:00:55:00:20 \
  #   --graphics vnc --console pty,target_type=serial --os-variant sle15sp4 --noautoconsole
  # sudo virsh autostart pfmp
}

phase_first_boot_wizard() {
  # TODO(live): drive the PFMP first-boot wizard over the console:
  #   sudo virsh console pfmp
  # Set network (DHCP -> 10.55.0.20 via reservation, or static), set delladmin
  # password, then run install_PFMP.sh (>2h). Confirm single-node PFMP support.
  echo "[wizard] drive 'virsh console pfmp' manually"
}

phase_build_cluster() {
  # TODO(live): bring up 1 MDM + 3 SDS RHEL9 guests (cloud image + cloud-init),
  # install the el9 ScaleIO RPMs, then build the cluster with scli:
  #   scli --create_mdm_cluster ...
  #   scli --add_protection_domain --add_storage_pool --add_sds --add_sds_device
  # Approve certs / sync mdm_mno_certificate.pem as needed.
  echo "[cluster] build MDM/SDS via scli"
}

phase_register_system() {
  # TODO(live): register the running PowerFlex 4.x system into PFMP (import
  # existing: MDM IPs + System ID from 'drv_cfg --query_mdms' + LIA cred) so the
  # Gateway /api/* + /rest/v1/* return real data for the dell_powerflex check.
  echo "[register] import system into PFMP Gateway"
}

phase_golden_ami() {
  # TODO(live): quiesce (shut down) guests so qcow2 is consistent, then snapshot.
  #   sudo virsh shutdown pfmp; ... ; aws ec2 create-image ...
  # All qcow2 + SDS device files live on the EBS root so create-image captures
  # them (local NVMe is ephemeral and ignored). Budget root 600-800 GiB.
  echo "[golden-ami] quiesce guests + aws ec2 create-image"
}

cat <<'USAGE'
Dell PowerFlex bootstrap is operator-driven and DEFERRED to live exploration.
Resolve the live unknowns in metal-nested.md, then invoke phases individually:
  phase_stage_ova / phase_convert_ova / phase_define_pfmp /
  phase_first_boot_wizard / phase_build_cluster / phase_register_system /
  phase_golden_ami
USAGE
