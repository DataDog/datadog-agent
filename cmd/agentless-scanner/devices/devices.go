// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package devices provides differents utility functions to list block devices
// and partitions, mount and unmount them.
package devices

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// reference: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
var xenDeviceName struct {
	sync.Mutex
	count int
}

var nbdDeviceName struct {
	sync.Mutex
	count   int
	nbdsMax *int
}

// NextXen returns the next available Xen block device name.
func NextXen() (string, bool) {
	xenDeviceName.Lock()
	defer xenDeviceName.Unlock()

	// loops from "xvdaa" to "xvddx"
	// we found out that xvddy and xvddz are problematic for some undocumented reason
	const xenMax = ('d'-'a'+1)*26 - 2
	count := xenDeviceName.count % xenMax
	dev := 'a' + uint8(count/26)
	rst := 'a' + uint8(count%26)
	bdPath := fmt.Sprintf("/dev/xvd%c%c", dev, rst)
	// TODO: just like for NBD devices, we should ensure that the
	// associated device is not already busy. However on ubuntu AMIs there
	// is no udev rule making the proper symlink from /dev/xvdxx device to
	// the /dev/nvmex created block device on volume attach.
	xenDeviceName.count = (count + 1) % xenMax
	return bdPath, true
}

// NextNBD returns the next available NBD block device name.
func NextNBD() (string, bool) {
	nbdDeviceName.Lock()
	defer nbdDeviceName.Unlock()

	// Init phase: counting the number of nbd devices created.
	if nbdDeviceName.nbdsMax == nil {
		bds, _ := filepath.Glob("/dev/nbd*")
		bdsCount := len(bds)
		nbdDeviceName.nbdsMax = &bdsCount
	}

	nbdsMax := *nbdDeviceName.nbdsMax
	if nbdsMax == 0 {
		log.Error("could not locate any NBD block device in /dev")
		return "", false
	}

	for i := 0; i < nbdsMax; i++ {
		count := (nbdDeviceName.count + i) % nbdsMax
		// From man 2 open: O_EXCL: ... on Linux 2.6 and later, O_EXCL can be
		// used without  O_CREAT  if pathname refers to  a block device.  If
		// the block device is in use by the system (e.g., mounted), open()
		// fails with the error EBUSY.
		bdPath := fmt.Sprintf("/dev/nbd%d", count)
		f, err := os.OpenFile(bdPath, os.O_RDONLY|os.O_EXCL, 0600)
		if err == nil {
			f.Close()
			nbdDeviceName.count = (count + 1) % nbdsMax
			return bdPath, true
		}
	}
	return "", false
}

// Partition represents a block device partition.
type Partition struct {
	DevicePath string
	FSType     string
}

// BlockDevice represents a block device.
type BlockDevice struct {
	Name        string         `json:"name"`
	Serial      string         `json:"serial"`
	Path        string         `json:"path"`
	Type        string         `json:"type"`
	FSType      string         `json:"fstype"`
	Mountpoints []string       `json:"mountpoints"`
	Children    []*BlockDevice `json:"children"`
}

// GetChildrenType returns the children block devices of the given type.
func (bd BlockDevice) GetChildrenType(t string) []BlockDevice {
	var bds []BlockDevice
	bd.recurse(func(child BlockDevice) {
		if child.Type == t {
			for _, b := range bds {
				if b.Path == child.Path {
					return
				}
			}
			bds = append(bds, child)
		}
	})
	return bds
}

func (bd BlockDevice) recurse(cb func(BlockDevice)) {
	for _, child := range bd.Children {
		child.recurse(cb)
	}
	cb(bd)
}

// List returns the list of block devices.
func List(ctx context.Context, deviceName ...string) ([]BlockDevice, error) {
	var blockDevices struct {
		BlockDevices []BlockDevice `json:"blockdevices"`
	}
	_, _ = exec.Command("udevadm", "settle", "--timeout=1").CombinedOutput()
	lsblkArgs := []string{"--json", "--bytes", "--output", "NAME,SERIAL,PATH,TYPE,FSTYPE,MOUNTPOINTS"}
	lsblkArgs = append(lsblkArgs, deviceName...)
	lsblkJSON, err := exec.CommandContext(ctx, "lsblk", lsblkArgs...).Output()
	if err != nil {
		var errx *exec.ExitError
		if errors.As(err, &errx) && errx.ExitCode() == 32 { // none of specified devices found
			return nil, nil
		}
		if !errors.Is(err, context.Canceled) {
			log.Warnf("lsblk exited with error: %v", err)
		}
		return nil, fmt.Errorf("lsblk exited with error: %w", err)
	}
	if err := json.Unmarshal(lsblkJSON, &blockDevices); err != nil {
		return nil, fmt.Errorf("lsblk output parsing error: %w", err)
	}
	// lsblk can return [null] as mountpoints list. We need to clean this up.
	for _, bd := range blockDevices.BlockDevices {
		for _, child := range bd.Children {
			mountpoints := child.Mountpoints
			child.Mountpoints = make([]string, 0, len(mountpoints))
			for _, mp := range mountpoints {
				if mp != "" {
					child.Mountpoints = append(child.Mountpoints, mp)
				}
			}
		}
	}
	return blockDevices.BlockDevices, nil
}

// Poll waits for the given block device to appear. Providing a serial number
// will filter the devices by serial number.
func Poll(ctx context.Context, scan *types.ScanTask, device string, serialNumber *string) (*BlockDevice, error) {
	log.Debugf("%s: polling partitions from device %q (sn=%v)", scan, device, serialNumber) // The attached device name may not be the one we expect. We update it.
	for i := 0; i < 120; i++ {
		if !sleepCtx(ctx, 500*time.Millisecond) {
			return nil, ctx.Err()
		}
		blockDevices, err := List(ctx)
		if err != nil {
			continue
		}
		for _, bd := range blockDevices {
			if serialNumber != nil && bd.Serial == *serialNumber {
				return &bd, nil
			} else if bd.Path == device {
				return &bd, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find the block device %s for", device)
}

// ListPartitions returns the list of partitions from the given block device.
func ListPartitions(ctx context.Context, scan *types.ScanTask, deviceName string) ([]Partition, error) {
	log.Debugf("%s: listing partitions from device %q", scan, deviceName)

	var partitions []Partition
	for i := 0; i < 5; i++ {
		blockDevices, err := List(ctx, deviceName)
		if err != nil {
			continue
		}
		if len(blockDevices) != 1 {
			continue
		}
		for _, part := range blockDevices[0].Children {
			if part.FSType == "btrfs" || part.FSType == "ext2" || part.FSType == "ext3" || part.FSType == "ext4" || part.FSType == "xfs" {
				partitions = append(partitions, Partition{
					DevicePath: part.Path,
					FSType:     part.FSType,
				})
			}
		}
		if len(partitions) > 0 {
			break
		}
		if !sleepCtx(ctx, 100*time.Millisecond) {
			return nil, ctx.Err()
		}
	}
	if len(partitions) == 0 {
		return nil, fmt.Errorf("could not find any btrfs, ext2, ext3, ext4 or xfs partition in %s", deviceName)
	}

	log.Debugf("%s: found %d compatible partitions for device %q", scan, len(partitions), deviceName)
	return partitions, nil
}

// Mount mounts the given partitions.
func Mount(ctx context.Context, scan *types.ScanTask, partitions []Partition) ([]string, error) {
	var mountPoints []string
	for _, mp := range partitions {
		mountPoint := scan.Path(types.EBSMountPrefix + path.Base(mp.DevicePath))
		if err := os.MkdirAll(mountPoint, 0700); err != nil {
			return nil, fmt.Errorf("could not create mountPoint directory %q: %w", mountPoint, err)
		}

		fsOptions := "ro,noauto,nodev,noexec,nosuid," // these are generic options supported for all filesystems
		switch mp.FSType {
		case "btrfs":
			// TODO: we could implement support for multiple BTRFS subvolumes in the future.
			fsOptions += "subvol=/root"
		case "ext2":
			// nothing
		case "ext3", "ext4":
			// noload means we do not try to load the journal
			fsOptions += "noload"
		case "xfs":
			// norecovery means we do not try to recover the FS
			fsOptions += "norecovery,nouuid"
		default:
			panic(fmt.Errorf("unsupported filesystem type %s", mp.FSType))
		}

		if mp.FSType == "btrfs" {
			// Replace fsid of btrfs partition with randomly generated UUID.
			log.Debugf("%s: execing btrfstune -f -u %s", scan, mp.DevicePath)
			_, err := exec.CommandContext(ctx, "btrfstune", "-f", "-u", mp.DevicePath).CombinedOutput()
			if err != nil {
				return nil, err
			}

			// Clear the tree log, to prevent "failed to read log tree" warning, which leads to "open_ctree failed" error.
			log.Debugf("%s: execing btrfs rescue zero-log %s", scan, mp.DevicePath)
			_, err = exec.CommandContext(ctx, "btrfs", "rescue", "zero-log", mp.DevicePath).CombinedOutput()
			if err != nil {
				return nil, err
			}
		}

		mountCmd := []string{"-o", fsOptions, "-t", mp.FSType, "--source", mp.DevicePath, "--target", mountPoint}
		log.Debugf("%s: execing mount %s", scan, mountCmd)

		var mountOutput []byte
		var errm error
		for i := 0; i < 50; i++ {
			// using context.Background() here as we do not want to sigkill
			// the "mount" command during work.
			mountOutput, errm = exec.CommandContext(context.Background(), "mount", mountCmd...).CombinedOutput()
			if errm == nil {
				break
			}
			if !sleepCtx(ctx, 200*time.Millisecond) {
				errm = ctx.Err()
				break
			}
		}
		if errm != nil {
			return nil, fmt.Errorf("could not mount into target=%q device=%q output=%q: %w", mountPoint, mp.DevicePath, string(mountOutput), errm)
		}
		mountPoints = append(mountPoints, mountPoint)
	}
	return mountPoints, nil
}

// Umount unmounts the given mount point.
func Umount(ctx context.Context, maybeScan *types.ScanTask, mountPoint string) {
	log.Debugf("%s: un-mounting %q", maybeScan, mountPoint)
	var umountOutput []byte
	var erru error
	const maxTries = 10
	for tryCount := 0; tryCount < maxTries; tryCount++ {
		if _, err := os.Stat(mountPoint); os.IsNotExist(err) {
			return
		}
		umountCmd := exec.CommandContext(ctx, "umount", mountPoint)
		if umountOutput, erru = umountCmd.CombinedOutput(); erru != nil {
			// Check for "not mounted" errors that we ignore
			const MntExFail = 32 // MNT_EX_FAIL
			if exiterr, ok := erru.(*exec.ExitError); ok && exiterr.ExitCode() == MntExFail && bytes.Contains(umountOutput, []byte("not mounted")) {
				return
			}
			waitDuration := 3 * time.Second
			log.Infof("%s: could not umount %s; retrying after %s (%d/%d): %s: %s", maybeScan, mountPoint, waitDuration, tryCount+1, maxTries, erru, string(umountOutput))
			if !sleepCtx(ctx, waitDuration) {
				return
			}
			continue
		}
		if err := os.Remove(mountPoint); err != nil {
			log.Warnf("could not remove mount point %q: %v", mountPoint, err)
		}
		return
	}
	log.Errorf("could not umount %s: %s: %s", mountPoint, erru, string(umountOutput))
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}
