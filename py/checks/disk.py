# stdlib
import os
import re

# 3p
try:
    import psutil
except ImportError:
    psutil = None

# project
from checks import AgentCheck
from config import _is_affirmative
from util import Platform
from utils.subprocess_output import get_subprocess_output


class Disk(AgentCheck):
    """ Collects metrics about the machine's disks. """
    # -T for filesystem info
    DF_COMMAND = ['df', '-T']
    METRIC_DISK = 'system.disk.{0}'
    METRIC_INODE = 'system.fs.inodes.{0}'

    def __init__(self, name, init_config, agentConfig, instances=None):
        if instances is not None and len(instances) > 1:
            raise Exception("Disk check only supports one configured instance.")
        AgentCheck.__init__(self, name, init_config,
                            agentConfig, instances=instances)
        # Get the configuration once for all
        self._load_conf(instances[0])

    def check(self, instance):
        """Get disk space/inode stats"""
        # Windows and Mac will always have psutil
        # (we have packaged for both of them)
        if self._psutil():
            self.collect_metrics_psutil()
        else:
            # FIXME: implement all_partitions (df -a)
            self.collect_metrics_manually()

    @classmethod
    def _psutil(cls):
        return psutil is not None

    def _load_conf(self, instance):
        self._excluded_filesystems = instance.get('excluded_filesystems', [])
        self._excluded_disks = instance.get('excluded_disks', [])
        self._tag_by_filesystem = _is_affirmative(
            instance.get('tag_by_filesystem', False))
        self._all_partitions = _is_affirmative(
            instance.get('all_partitions', False))

        # Force exclusion of CDROM (iso9660) from disk check
        self._excluded_filesystems.append('iso9660')

        # FIXME: 6.x, drop use_mount option in datadog.conf
        self._load_legacy_option(instance, 'use_mount', False,
                                 operation=_is_affirmative)
        # FIXME: 6.x, drop device_blacklist_re option in datadog.conf
        self._load_legacy_option(instance, 'excluded_disk_re', '^$',
                                 legacy_name='device_blacklist_re',
                                 operation=re.compile)

    def _load_legacy_option(self, instance, option, default,
                            legacy_name=None, operation=lambda l: l):
        value = instance.get(option, default)
        legacy_name = legacy_name or option

        if value == default and legacy_name in self.agentConfig:
            self.log.warn(
                "Using `{0}` in datadog.conf has been deprecated"
                " in favor of `{1}` in disk.yaml".format(legacy_name, option)
            )
            value = self.agentConfig.get(legacy_name) or default
        setattr(self, '_{0}'.format(option), operation(value))

    def collect_metrics_psutil(self):
        self._valid_disks = {}
        for part in psutil.disk_partitions(all=True):
            # we check all exclude conditions
            if self._exclude_disk_psutil(part):
                continue
            # Get disk metrics here to be able to exclude on total usage
            try:
                disk_usage = psutil.disk_usage(part.mountpoint)
            except Exception, e:
                self.log.debug("Unable to get disk metrics for %s: %s",
                               part.mountpoint, e)
                continue
            # Exclude disks with total disk size 0
            if disk_usage.total == 0:
                continue
            # For later, latency metrics
            self._valid_disks[part.device] = (part.fstype, part.mountpoint)
            self.log.debug('Passed: {0}'.format(part.device))

            tags = [part.fstype] if self._tag_by_filesystem else []
            device_name = part.mountpoint if self._use_mount else part.device

            # Note: psutil (0.3.0 to at least 3.1.1) calculates in_use as (used / total)
            #       The problem here is that total includes reserved space the user
            #       doesn't have access to. This causes psutil to calculate a misleadng
            #       percentage for in_use; a lower percentage than df shows.

            # Calculate in_use w/o reserved space; consistent w/ df's Use% metric.
            pmets = self._collect_part_metrics(part, disk_usage)
            used = 'system.disk.used'
            free = 'system.disk.free'
            pmets['system.disk.in_use'] = pmets[used] / (pmets[used] + pmets[free])

            # legacy check names c: vs psutil name C:\\
            if Platform.is_win32():
                device_name = device_name.strip('\\').lower()
            for metric_name, metric_value in pmets.iteritems():
                self.gauge(metric_name, metric_value,
                           tags=tags, device_name=device_name)
        # And finally, latency metrics, a legacy gift from the old Windows Check
        if Platform.is_win32():
            self.collect_latency_metrics()

    def _exclude_disk_psutil(self, part):
        # skip cd-rom drives with no disk in it; they may raise
        # ENOENT, pop-up a Windows GUI error for a non-ready
        # partition or just hang;
        # and all the other excluded disks
        return ((Platform.is_win32() and ('cdrom' in part.opts or
                                          part.fstype == '')) or
                self._exclude_disk(part.device, part.fstype))

    # We don't want all those incorrect devices
    def _exclude_disk(self, name, filesystem):
        return (((not name or name == 'none') and not self._all_partitions) or
                name in self._excluded_disks or
                self._excluded_disk_re.match(name) or
                filesystem in self._excluded_filesystems)

    def _collect_part_metrics(self, part, usage):
        metrics = {}
        for name in ['total', 'used', 'free']:
            # For legacy reasons,  the standard unit it kB
            metrics[self.METRIC_DISK.format(name)] = getattr(usage, name) / 1024.0
        # FIXME: 6.x, use percent, a lot more logical than in_use
        metrics[self.METRIC_DISK.format('in_use')] = usage.percent / 100.0
        if Platform.is_unix():
            metrics.update(self._collect_inodes_metrics(part.mountpoint))

        return metrics

    def _collect_inodes_metrics(self, mountpoint):
        metrics = {}
        inodes = os.statvfs(mountpoint)
        if inodes.f_files != 0:
            total = inodes.f_files
            free = inodes.f_ffree
            metrics[self.METRIC_INODE.format('total')] = total
            metrics[self.METRIC_INODE.format('free')] = free
            metrics[self.METRIC_INODE.format('used')] = total - free
            # FIXME: 6.x, use percent, a lot more logical than in_use
            metrics[self.METRIC_INODE.format('in_use')] = \
                (total - free) / float(total)
        return metrics

    def collect_latency_metrics(self):
        for disk_name, disk in psutil.disk_io_counters(True).iteritems():
            self.log.debug('IO Counters: {0} -> {1}'.format(disk_name, disk))
            # x100 to have it as a percentage,
            # /1000 as psutil returns the value in ms
            read_time_pct = disk.read_time * 100.0 / 1000.0
            write_time_pct = disk.write_time * 100.0 / 1000.0
            self.rate(self.METRIC_DISK.format('read_time_pct'),
                      read_time_pct, device_name=disk_name)
            self.rate(self.METRIC_DISK.format('write_time_pct'),
                      write_time_pct, device_name=disk_name)

    # no psutil, let's use df
    def collect_metrics_manually(self):
        df_out, _, _ = get_subprocess_output(self.DF_COMMAND + ['-k'], self.log)
        self.log.debug(df_out)
        for device in self._list_devices(df_out):
            self.log.debug("Passed: {0}".format(device))
            tags = [device[1]] if self._tag_by_filesystem else []
            device_name = device[-1] if self._use_mount else device[0]
            for metric_name, value in self._collect_metrics_manually(device).iteritems():
                self.gauge(metric_name, value, tags=tags,
                           device_name=device_name)

    def _collect_metrics_manually(self, device):
        result = {}

        used = float(device[3])
        free = float(device[4])

        # device is
        # ["/dev/sda1", "ext4", 524288,  171642,  352646, "33%", "/"]
        result[self.METRIC_DISK.format('total')] = float(device[2])
        result[self.METRIC_DISK.format('used')] = used
        result[self.METRIC_DISK.format('free')] = free

        # Rather than grabbing in_use, let's calculate it to be more precise
        result[self.METRIC_DISK.format('in_use')] = used / (used + free)

        result.update(self._collect_inodes_metrics(device[-1]))
        return result

    def _keep_device(self, device):
        # device is for Unix
        # [/dev/disk0s2, ext4, 244277768, 88767396, 155254372, 37%, /]
        # First, skip empty lines.
        # then filter our fake hosts like 'map -hosts'.
        #    Filesystem    Type   1024-blocks     Used Available Capacity  Mounted on
        #    /dev/disk0s2  ext4     244277768 88767396 155254372    37%    /
        #    map -hosts    tmpfs            0        0         0   100%    /net
        # and finally filter out fake devices
        return (device and len(device) > 1 and
                device[2].isdigit() and
                not self._exclude_disk(device[0], device[1]))

    def _flatten_devices(self, devices):
        # Some volumes are stored on their own line. Rejoin them here.
        previous = None
        for parts in devices:
            if len(parts) == 1:
                previous = parts[0]
            elif previous and self._is_number(parts[0]):
                # collate with previous line
                parts.insert(0, previous)
                previous = None
            else:
                previous = None
        return devices

    def _list_devices(self, df_output):
        """
        Given raw output for the df command, transform it into a normalized
        list devices. A 'device' is a list with fields corresponding to the
        output of df output on each platform.
        """
        all_devices = [l.strip().split() for l in df_output.splitlines()]

        # Skip the header row and empty lines.
        raw_devices = [l for l in all_devices[1:] if l]

        # Flatten the disks that appear in the mulitple lines.
        flattened_devices = self._flatten_devices(raw_devices)

        # Filter fake or unwanteddisks.
        return [d for d in flattened_devices if self._keep_device(d)]
