# stdlib
import array
from collections import defaultdict
import fcntl
import itertools
import os
import struct

# 3rd party
import psutil

# project
from checks import AgentCheck

MIXED = "mixed"
DATA = "data"
METADATA = "metadata"
SYSTEM = "system"
SINGLE = "single"
RAID0 = "raid0"
RAID1 = "raid1"
RAID10 = "raid10"
DUP = "dup"
UNKNOWN = "unknown"

FLAGS_MAPPER = defaultdict(lambda:  (SINGLE, UNKNOWN),{
    1: (SINGLE, DATA),
    2: (SINGLE, SYSTEM),
    4: (SINGLE, METADATA),
    5: (SINGLE, MIXED),
    9: (RAID0, DATA),
    10: (RAID0, SYSTEM),
    12: (RAID0, METADATA),
    13: (RAID0, MIXED),
    17: (RAID1, DATA),
    18: (RAID1, SYSTEM),
    20: (RAID1, METADATA),
    21: (RAID1, MIXED),
    33: (DUP, DATA),
    34: (DUP, SYSTEM),
    36: (DUP, METADATA),
    37: (DUP, MIXED),
    65: (RAID10, DATA),
    66: (RAID10, SYSTEM),
    68: (RAID10, METADATA),
    69: (RAID10, MIXED),

})

BTRFS_IOC_SPACE_INFO = 0xc0109414

TWO_LONGS_STRUCT = struct.Struct("=2Q")  # 2 Longs
THREE_LONGS_STRUCT = struct.Struct("=3Q")  # 3 Longs


def sized_array(count):
    return array.array("B", itertools.repeat(0, count))


class FileDescriptor(object):

    def __init__(self, mountpoint):
        self.fd = os.open(mountpoint, os.O_DIRECTORY)

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        os.close(self.fd)

    def fileno(self):
        return self.fd

    def open(self, dir):
        return self.fd


class BTRFS(AgentCheck):

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances=instances)
        if instances is not None and len(instances) > 1:
            raise Exception("BTRFS check only supports one configured instance.")

    def get_usage(self, mountpoint):
        results = []

        with FileDescriptor(mountpoint) as fd:

            # Get the struct size needed
            # https://github.com/spotify/linux/blob/master/fs/btrfs/ioctl.h#L46-L50
            ret = sized_array(TWO_LONGS_STRUCT.size)
            fcntl.ioctl(fd, BTRFS_IOC_SPACE_INFO, ret)
            _, total_spaces = TWO_LONGS_STRUCT.unpack(ret)

            # Allocate it
            buffer_size = (TWO_LONGS_STRUCT.size
                        + total_spaces * THREE_LONGS_STRUCT.size)

            data = sized_array(buffer_size)
            TWO_LONGS_STRUCT.pack_into(data, 0, total_spaces, 0)
            fcntl.ioctl(fd, BTRFS_IOC_SPACE_INFO, data)

        _, total_spaces = TWO_LONGS_STRUCT.unpack_from(ret, 0)
        for offset in xrange(TWO_LONGS_STRUCT.size,
                             buffer_size,
                             THREE_LONGS_STRUCT.size):

            # https://github.com/spotify/linux/blob/master/fs/btrfs/ioctl.h#L40-L44
            flags, total_bytes, used_bytes = THREE_LONGS_STRUCT.unpack_from(data, offset)
            results.append((flags, total_bytes, used_bytes))

        return results

    def check(self, instance):
        btrfs_devices = {}
        excluded_devices = instance.get('excluded_devices', [])
        for p in psutil.disk_partitions():
            if (p.fstype == 'btrfs' and p.device not in btrfs_devices
                    and p.device not in excluded_devices):
                btrfs_devices[p.device] = p.mountpoint

        if len(btrfs_devices) == 0:
            raise Exception("No btrfs device found")

        for device, mountpoint in btrfs_devices.iteritems():
            for flags, total_bytes, used_bytes in self.get_usage(mountpoint):
                replication_type, usage_type = FLAGS_MAPPER[flags]
                tags = [
                    'usage_type:{0}'.format(usage_type),
                    'replication_type:{0}'.format(replication_type),
                ]

                free = total_bytes - used_bytes
                usage = float(used_bytes) / float(total_bytes)

                self.gauge('system.disk.btrfs.total', total_bytes, tags=tags, device_name=device)
                self.gauge('system.disk.btrfs.used', used_bytes, tags=tags, device_name=device)
                self.gauge('system.disk.btrfs.free', free, tags=tags, device_name=device)
                self.gauge('system.disk.btrfs.usage', usage, tags=tags, device_name=device)
