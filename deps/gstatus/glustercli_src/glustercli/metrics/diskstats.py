import os
import subprocess

from glustercli.metrics.utils import get_local_bricks


DEFAULT_DISKSTAT = {
    "major_number": 0,
    "minor_number": 0,
    "reads_completed": 0,
    "reads_merged": 0,
    "sectors_read": 0,
    "time_spent_reading": 0,
    "writes_completed": 0,
    "writes_merged": 0,
    "sectors_written": 0,
    "time_spent_writing": 0,
    "ios_currently_in_progress": 0,
    "time_spent_doing_ios": 0,
    "weighted_time_spent_doing_ios": 0
}


def local_diskstats(volname=None):
    """
    Collect Diskstats info of local bricks

    :param volname: Volume Name
    :returns: List of diskstats information
        {
            "volume": VOLUME_NAME,
            "brick_index": BRICK_INDEX_IN_VOL_INFO,
            "node_id": NODE_ID,
            "brick": BRICK_NAME,
            "fs": BRICK_FILESYSTEM,
            "device": BRICK_DEVICE,
            "major_number": MAJOR_NUMBER,
            "minor_number": MINOR_NUMBER,
            "reads_completed": READS_COMPLETED,
            "reads_merged": READS_MERGED,
            "sectors_read": SECTORS_READ,
            "time_spent_reading": TIME_SPENT_READING,
            "writes_completed": WRITES_COMPLETED,
            "writes_merged": WRITES_MERGED,
            "sectors_written": SECTORS_WRITTEN,
            "time_spent_writing": TIME_SPENT_WRITING,
            "ios_currently_in_progress": IOS_CURRENTLY_IN_PROGRESS,
            "time_spent_doing_ios": TIME_SPENT_DOING_IOS,
            "weighted_time_spent_doing_ios": WEIGHTED_TIME_SPENT_DOING_IOS
        }
    """
    local_bricks = get_local_bricks(volname)
    cmd = ["df", "--output=source"]

    diskstat_data_raw = ""
    with open("/proc/diskstats") as stat_file:
        diskstat_data_raw = stat_file.read()

    # /proc/diskstats fields
    #  1 - major number
    #  2 - minor mumber
    #  3 - device name
    #  4 - reads completed successfully
    #  5 - reads merged
    #  6 - sectors read
    #  7 - time spent reading (ms)
    #  8 - writes completed
    #  9 - writes merged
    # 10 - sectors written
    # 11 - time spent writing (ms)
    # 12 - I/Os currently in progress
    # 13 - time spent doing I/Os (ms)
    # 14 - weighted time spent doing I/Os (ms)
    diskstat_data = {}
    for row in diskstat_data_raw.strip().split("\n"):
        row = row.split()
        if not row:
            continue

        diskstat_data[row[2]] = {
            "major_number": row[0],
            "minor_number": row[1],
            "reads_completed": row[3],
            "reads_merged": row[4],
            "sectors_read": row[5],
            "time_spent_reading": row[6],
            "writes_completed": row[7],
            "writes_merged": row[8],
            "sectors_written": row[9],
            "time_spent_writing": row[10],
            "ios_currently_in_progress": row[11],
            "time_spent_doing_ios": row[12],
            "weighted_time_spent_doing_ios": row[13]
        }

    for brick in local_bricks:
        bpath = brick["brick"].split(":", 1)[-1]
        proc = subprocess.Popen(cmd + [bpath],
                                stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE,
                                universal_newlines=True)
        out, _ = proc.communicate()

        # `df` command error
        if proc.returncode != 0:
            brick["fs"] = "unknown"
            brick["device"] = "unknown"
            brick.update(DEFAULT_DISKSTAT)
            continue

        df_data = out.strip()
        df_data = df_data.split("\n")[-1].strip()  # First line is header
        brick["fs"] = df_data
        if os.path.islink(df_data):
            brick["device"] = os.readlink(df_data).split("/")[-1]
        else:
            brick["device"] = df_data.split("/")[-1]

        brick.update(diskstat_data.get(brick["device"], DEFAULT_DISKSTAT))

    return local_bricks
