import os

from glustercli.metrics.utils import get_local_bricks


def local_utilization(volname=None):
    """
    Collect Utilization details of local bricks

    :param volname: Volume Name
    :returns: List of utilization information
        {
            "volume": VOLUME_NAME,
            "brick_index": BRICK_INDEX_IN_VOL_INFO,
            "node_id": NODE_ID,
            "brick": BRICK_NAME,
            "block_size": ST_F_FRSIZE,
            "blocks_total": ST_F_BLOCKS,
            "blocks_free": ST_F_BFREE,
            "blocks_avail": ST_F_BAVAIL,
            "inodes_total": ST_F_FILES,
            "inodes_free": ST_F_FFREE,
            "inodes_avail": ST_F_FAVAIL
        }
    """

    local_bricks = get_local_bricks(volname)

    for brick in local_bricks:
        bpath = brick["brick"].split(":", 1)[-1]
        stat = os.statvfs(bpath)

        brick["block_size"] = stat.f_frsize
        brick["blocks_total"] = stat.f_blocks
        brick["blocks_free"] = stat.f_bfree
        brick["blocks_avail"] = stat.f_bavail
        brick["inodes_total"] = stat.f_files
        brick["inodes_free"] = stat.f_ffree
        brick["inodes_avail"] = stat.f_favail

    return local_bricks
