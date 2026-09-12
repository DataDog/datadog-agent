# -*- coding: utf-8 -*-
import copy
import xml.etree.cElementTree as etree
import math

from .utils import RebalanceOperationType as ROT

ParseError = etree.ParseError if hasattr(etree, 'ParseError') else SyntaxError


class GlusterCmdOutputParseError(Exception):
    pass


HEALTH_UP = "up"
HEALTH_DOWN = "down"
HEALTH_PARTIAL = "partial"
HEALTH_DEGRADED = "degraded"

STATE_CREATED = "Created"
STATE_STARTED = "Started"
STATE_STOPPED = "Stopped"

TYPE_REPLICATE = "REPLICATE"
TYPE_DISPERSE = "DISPERSE"


def _subvol_health(subvol):
    up_bricks = 0
    for brick in subvol["bricks"]:
        if brick["online"]:
            up_bricks += 1

    health = HEALTH_UP
    if len(subvol["bricks"]) != up_bricks:
        health = HEALTH_DOWN
        if subvol["type"] == TYPE_REPLICATE:
            if up_bricks >= math.ceil(subvol["replica"] / 2):
                health = HEALTH_PARTIAL

        # If down bricks are less than or equal to redudancy count
        # then Volume is UP but some bricks are down
        if subvol["type"] == TYPE_DISPERSE:
            down_bricks = (len(subvol["bricks"]) - up_bricks)
            if down_bricks <= subvol["disperse_redundancy"]:
                health = HEALTH_PARTIAL

    return health


def _update_volume_health(volumes):
    # Note: vol is edited inside loop
    for vol in volumes:
        if vol["status"] != STATE_STARTED:
            continue

        vol["health"] = HEALTH_UP
        up_subvols = 0

        # Note: subvol is edited inside loop
        for subvol in vol["subvols"]:
            subvol["health"] = _subvol_health(subvol)

            if subvol["health"] == HEALTH_DOWN:
                vol["health"] = HEALTH_DEGRADED

            if subvol["health"] == HEALTH_PARTIAL:
                if vol["health"] != HEALTH_DEGRADED:
                    vol["health"] = subvol["health"]

            if subvol["health"] != HEALTH_DOWN:
                up_subvols += 1

        if up_subvols == 0:
            vol["health"] = HEALTH_DOWN


def _update_volume_utilization(volumes):
    # Note: modifies volume inside loop
    for vol in volumes:
        vol["size_total"] = 0
        vol["size_free"] = 0
        vol["size_used"] = 0
        vol["inodes_total"] = 0
        vol["inodes_free"] = 0
        vol["inodes_used"] = 0

        # Note: modifies subvol inside loop
        for subvol in vol["subvols"]:
            effective_capacity_used = 0
            effective_capacity_total = 0
            effective_inodes_used = 0
            effective_inodes_total = 0

            for brick in subvol["bricks"]:
                if brick["type"] == "Arbiter":
                    continue

                if brick["size_used"] >= effective_capacity_used:
                    effective_capacity_used = brick["size_used"]

                ect_is_zero = effective_capacity_total == 0
                bst_lt_ect = brick["size_total"] <= effective_capacity_total
                if ect_is_zero or (bst_lt_ect and brick["size_total"] > 0):
                    effective_capacity_total = brick["size_total"]

                if brick["inodes_used"] >= effective_inodes_used:
                    effective_inodes_used = brick["inodes_used"]

                eit_is_zero = effective_inodes_total == 0
                bit_lt_eit = brick["inodes_total"] <= effective_inodes_total
                if eit_is_zero or (bit_lt_eit and brick["inodes_total"] > 0):
                    effective_inodes_total = brick["inodes_total"]

            if subvol["type"] == TYPE_DISPERSE:
                # Subvol Size = Sum of size of Data bricks
                effective_capacity_used = effective_capacity_used * (
                    subvol["disperse"] - subvol["disperse_redundancy"])
                effective_capacity_total = effective_capacity_total * (
                    subvol["disperse"] - subvol["disperse_redundancy"])
                effective_inodes_used = effective_inodes_used * (
                    subvol["disperse"] - subvol["disperse_redundancy"])
                effective_inodes_total = effective_inodes_total * (
                    subvol["disperse"] - subvol["disperse_redundancy"])

            vol["size_total"] += effective_capacity_total
            vol["size_used"] += effective_capacity_used
            vol["size_free"] = vol["size_total"] - vol["size_used"]
            vol["inodes_total"] += effective_inodes_total
            vol["inodes_used"] += effective_inodes_used
            vol["inodes_free"] = vol["inodes_total"] - vol["inodes_used"]


def _parse_a_vol(volume_el):
    value = {
        'name': volume_el.find('name').text,
        'uuid': volume_el.find('id').text,
        'type': volume_el.find('typeStr').text.upper().replace('-', '_'),
        'status': volume_el.find('statusStr').text,
        'num_bricks': int(volume_el.find('brickCount').text),
        'distribute': int(volume_el.find('distCount').text),
#        'stripe': int(volume_el.find('stripeCount').text),
#        'stripe' : 1,
        'replica': int(volume_el.find('replicaCount').text),
        'disperse': int(volume_el.find('disperseCount').text),
        'disperse_redundancy': int(volume_el.find('redundancyCount').text),
        'transport': volume_el.find('transport').text,
        'snapshot_count': int(volume_el.find('snapshotCount').text),
        'bricks': [],
        'options': []
    }

    if value['transport'] == '0':
        value['transport'] = 'TCP'
    elif value['transport'] == '1':
        value['transport'] = 'RDMA'
    else:
        value['transport'] = 'TCP,RDMA'

    for brick in volume_el.findall('bricks/brick'):
        brick_type = "Brick"
        if brick.find("isArbiter").text == '1':
            brick_type = "Arbiter"

        value['bricks'].append({
            "name": brick.find("name").text,
            "uuid": brick.find("hostUuid").text,
            "type": brick_type
        })

    for opt in volume_el.findall('options/option'):
        value['options'].append({"name": opt.find('name').text,
                                 "value": opt.find('value').text})

    return value


def _get_subvol_bricks_count(replica_count, disperse_count):
    if replica_count > 1:
        return replica_count

    if disperse_count > 0:
        return disperse_count

    return 1


def _group_subvols(volumes):
    out_volumes = copy.deepcopy(volumes)
    for idx, vol in enumerate(volumes):
        # Remove Bricks information from the output
        # and include subvols
        del out_volumes[idx]["bricks"]
        out_volumes[idx]["subvols"] = []
        subvol_type = vol["type"].split("_")[-1]
        subvol_bricks_count = _get_subvol_bricks_count(vol["replica"],
                                                       vol["disperse"])

        number_of_subvols = int(len(vol["bricks"]) / subvol_bricks_count)

        for sidx in range(number_of_subvols):
            subvol = {
                "name": "%s-%s-%s" % (vol["name"], subvol_type.lower(), sidx),
                "replica": vol["replica"],
                "disperse": vol["disperse"],
                "disperse_redundancy": vol["disperse_redundancy"],
                "type": subvol_type,
                "bricks": []
            }
            for bidx in range(subvol_bricks_count):
                subvol["bricks"].append(
                    vol["bricks"][sidx * subvol_bricks_count + bidx]
                )
            out_volumes[idx]["subvols"].append(subvol)

    return out_volumes


def parse_volume_info(info, group_subvols=False):
    tree = etree.fromstring(info)
    volumes = []
    for volume_el in tree.findall('volInfo/volumes/volume'):
        try:
            volumes.append(_parse_a_vol(volume_el))
        except (ParseError, AttributeError, ValueError) as err:
            raise GlusterCmdOutputParseError(err)

    if group_subvols:
        return _group_subvols(volumes)

    return volumes

def _check_node_value(node_el, key, type, default_value):
    value = node_el.find(key)
    if value is not None:
        return type(value.text)
    return type(default_value)

def _parse_a_node(node_el):
    name = (node_el.find('hostname').text + ":" + node_el.find('path').text)
    online = node_el.find('status').text == "1" or False
    if not online:
        # if the node where the brick exists isn't
        # online then no reason to continue as the
        # caller of this method will populate "default"
        # information
        return {'name': name, 'online': online}

    value = {
        'name': name,
        'uuid': node_el.find('peerid').text,
        'online': online,
        'pid': node_el.find('pid').text,
        'size_total': int(node_el.find('sizeTotal').text),
        'size_free': int(node_el.find('sizeFree').text),
        'inodes_total': _check_node_value(node_el, 'inodesTotal', int, 0),   
        'inodes_free': _check_node_value(node_el, 'inodesFree', int, 0),
        'device': node_el.find('device').text,
        'block_size': node_el.find('blockSize').text,
        'mnt_options': node_el.find('mntOptions').text,
        'fs_name': node_el.find('fsName').text,
    }
    value['size_used'] = value['size_total'] - value['size_free']
    value['inodes_used'] = value['inodes_total'] - value['inodes_free']

    # ISSUE #14 glusterfs 3.6.5 does not have 'ports' key
    # in vol status detail xml output
    if node_el.find('ports'):
        value['ports'] = {
            "tcp": node_el.find('ports').find("tcp").text,
            "rdma": node_el.find('ports').find("rdma").text
        }
    else:
        value['ports'] = {
            "tcp": node_el.find('port'),
            "rdma": None
        }

    return value


def _parse_volume_status(data):
    tree = etree.fromstring(data)
    nodes = []
    for node_el in tree.findall('volStatus/volumes/volume/node'):
        try:
            nodes.append(_parse_a_node(node_el))
        except (ParseError, AttributeError, ValueError) as err:
            raise GlusterCmdOutputParseError(err)

    return nodes


def parse_volume_status(status_data, volinfo, group_subvols=False):
    nodes_data = _parse_volume_status(status_data)
    tmp_brick_status = {}
    for node in nodes_data:
        tmp_brick_status[node["name"]] = node

    volumes = []
    for vol in volinfo:
        volumes.append(vol.copy())
        volumes[-1]["bricks"] = []

        for brick in vol["bricks"]:
            brick_status_data = tmp_brick_status.get(brick["name"], None)
            if brick_status_data is None:
                use_default = True
            else:
                # brick could be offline
                use_default = not brick_status_data.get("online", False)

            if use_default:
                # Default Status
                volumes[-1]["bricks"].append({
                    "name": brick["name"],
                    "uuid": brick["uuid"],
                    "type": brick["type"],
                    "online": False,
                    "ports": {"tcp": "N/A", "rdma": "N/A"},
                    "pid": "N/A",
                    "size_total": 0,
                    "size_free": 0,
                    "size_used": 0,
                    "inodes_total": 0,
                    "inodes_free": 0,
                    "inodes_used": 0,
                    "device": "N/A",
                    "block_size": "N/A",
                    "mnt_options": "N/A",
                    "fs_name": "N/A"
                })
            else:
                volumes[-1]["bricks"].append(brick_status_data.copy())
                volumes[-1]["bricks"][-1]["type"] = brick["type"]

    if group_subvols:
        grouped_vols = _group_subvols(volumes)
        _update_volume_utilization(grouped_vols)
        _update_volume_health(grouped_vols)
        return grouped_vols

    return volumes


def _parse_profile_info_clear(volume_el):
    clear = {
        'volname': volume_el.find('volname').text,
        'bricks': []
    }

    for brick_el in volume_el.findall('brick'):
        clear['bricks'].append({
            'brick_name': brick_el.find('brickName').text,
            'clear_stats': brick_el.find('clearStats').text
        })

    return clear


def _bytes_size(size):
    traditional = [
        (1024**5, 'PB'),
        (1024**4, 'TB'),
        (1024**3, 'GB'),
        (1024**2, 'MB'),
        (1024**1, 'KB'),
        (1024**0, 'B')
    ]

    for factor, suffix in traditional:
        if size > factor:
            break
    return str(int(size / factor)) + suffix


def _parse_profile_block_stats(b_el):
    stats = []
    for block_el in b_el.findall('block'):
        size = _bytes_size(int(block_el.find('size').text))
        stats.append({size: {'reads': int(block_el.find('reads').text),
                             'writes': int(block_el.find('writes').text)}})
    return stats


def _parse_profile_fop_stats(fop_el):
    stats = []
    for fop in fop_el.findall('fop'):
        name = fop.find('name').text
        stats.append(
            {
                name: {
                    'hits': int(fop.find('hits').text),
                    'max_latency': float(fop.find('maxLatency').text),
                    'min_latency': float(fop.find('minLatency').text),
                    'avg_latency': float(fop.find('avgLatency').text),
                }
            }
        )
    return stats


def _parse_profile_bricks(brick_el):
    cumulative_block_stats = []
    cumulative_fop_stats = []
    cumulative_total_read_bytes = 0
    cumulative_total_write_bytes = 0
    cumulative_total_duration = 0

    interval_block_stats = []
    interval_fop_stats = []
    interval_total_read_bytes = 0
    interval_total_write_bytes = 0
    interval_total_duration = 0

    brick_name = brick_el.find('brickName').text

    if brick_el.find('cumulativeStats') is not None:
        cumulative_block_stats = _parse_profile_block_stats(
            brick_el.find('cumulativeStats/blockStats'))
        cumulative_fop_stats = _parse_profile_fop_stats(
            brick_el.find('cumulativeStats/fopStats'))
        cumulative_total_read_bytes = int(
            brick_el.find('cumulativeStats').find('totalRead').text)
        cumulative_total_write_bytes = int(
            brick_el.find('cumulativeStats').find('totalWrite').text)
        cumulative_total_duration = int(
            brick_el.find('cumulativeStats').find('duration').text)

    if brick_el.find('intervalStats') is not None:
        interval_block_stats = _parse_profile_block_stats(
            brick_el.find('intervalStats/blockStats'))
        interval_fop_stats = _parse_profile_fop_stats(
            brick_el.find('intervalStats/fopStats'))
        interval_total_read_bytes = int(
            brick_el.find('intervalStats').find('totalRead').text)
        interval_total_write_bytes = int(
            brick_el.find('intervalStats').find('totalWrite').text)
        interval_total_duration = int(
            brick_el.find('intervalStats').find('duration').text)

    profile_brick = {
        'brick_name': brick_name,
        'cumulative_block_stats': cumulative_block_stats,
        'cumulative_fop_stats': cumulative_fop_stats,
        'cumulative_total_read_bytes': cumulative_total_read_bytes,
        'cumulative_total_write_bytes': cumulative_total_write_bytes,
        'cumulative_total_duration': cumulative_total_duration,
        'interval_block_stats': interval_block_stats,
        'interval_fop_stats': interval_fop_stats,
        'interval_total_read_bytes': interval_total_read_bytes,
        'interval_total_write_bytes': interval_total_write_bytes,
        'interval_total_duration': interval_total_duration,
    }

    return profile_brick


def _parse_profile_info(volume_el):
    profile = {
        'volname': volume_el.find('volname').text,
        'bricks': []
    }

    for brick_el in volume_el.findall('brick'):
        profile['bricks'].append(_parse_profile_bricks(brick_el))

    return profile


def parse_volume_profile_info(info, opt):
    xml = etree.fromstring(info)
    profiles = []
    for prof_el in xml.findall('volProfile'):
        try:
            if opt == "clear":
                profiles.append(_parse_profile_info_clear(prof_el))
            else:
                profiles.append(_parse_profile_info(prof_el))

        except (ParseError, AttributeError, ValueError) as err:
            raise GlusterCmdOutputParseError(err)

    return profiles


def parse_volume_options(data):
    raise NotImplementedError("Volume Options")


def parse_georep_status(data, volinfo):
    """
    Merge Geo-rep status and Volume Info to get Offline Status
    and to sort the status in the same order as of Volume Info
    """
    session_keys = set()
    gstatus = {}

    try:
        tree = etree.fromstring(data)
        # Get All Sessions
        for volume_el in tree.findall("geoRep/volume"):
            sessions_el = volume_el.find("sessions")
            # Master Volume name if multiple Volumes
            mvol = volume_el.find("name").text

            # For each session, collect the details
            for session in sessions_el.findall("session"):
                session_slave = "{0}:{1}".format(mvol, session.find(
                    "session_slave").text)
                session_keys.add(session_slave)
                gstatus[session_slave] = {}

                for pair in session.findall('pair'):
                    master_brick = "{0}:{1}".format(
                        pair.find("master_node").text,
                        pair.find("master_brick").text
                    )

                    gstatus[session_slave][master_brick] = {
                        "mastervol": mvol,
                        "slavevol": pair.find("slave").text.split("::")[-1],
                        "master_node": pair.find("master_node").text,
                        "master_brick": pair.find("master_brick").text,
                        "slave_user": pair.find("slave_user").text,
                        "slave": pair.find("slave").text,
                        "slave_node": pair.find("slave_node").text,
                        "status": pair.find("status").text,
                        "crawl_status": pair.find("crawl_status").text,
                        "entry": pair.find("entry").text,
                        "data": pair.find("data").text,
                        "meta": pair.find("meta").text,
                        "failures": pair.find("failures").text,
                        "checkpoint_completed": pair.find(
                            "checkpoint_completed").text,
                        "master_node_uuid": pair.find("master_node_uuid").text,
                        "last_synced": pair.find("last_synced").text,
                        "checkpoint_time": pair.find("checkpoint_time").text,
                        "checkpoint_completion_time":
                        pair.find("checkpoint_completion_time").text
                    }
    except (ParseError, AttributeError, ValueError) as err:
        raise GlusterCmdOutputParseError(err)

    # Get List of Bricks for each Volume
    all_bricks = {}
    for vol in volinfo:
        all_bricks[vol["name"]] = vol["bricks"]

    # For Each session Get Bricks info for the Volume and Populate
    # Geo-rep status for that Brick
    out = []
    for session in session_keys:
        mvol, _, slave = session.split(":", 2)
        slave = slave.replace("ssh://", "")
        master_bricks = all_bricks[mvol]
        out.append([])
        for brick in master_bricks:
            bname = brick["name"]
            if gstatus.get(session) and gstatus[session].get(bname, None):
                out[-1].append(gstatus[session][bname])
            else:
                # Offline Status
                node, brick_path = bname.split(":")
                if "@" not in slave:
                    slave_user = "root"
                else:
                    slave_user, _ = slave.split("@")

                out[-1].append({
                    "mastervol": mvol,
                    "slavevol": slave.split("::")[-1],
                    "master_node": node,
                    "master_brick": brick_path,
                    "slave_user": slave_user,
                    "slave": slave,
                    "slave_node": "N/A",
                    "status": "Offline",
                    "crawl_status": "N/A",
                    "entry": "N/A",
                    "data": "N/A",
                    "meta": "N/A",
                    "failures": "N/A",
                    "checkpoint_completed": "N/A",
                    "master_node_uuid": brick["uuid"],
                    "last_synced": "N/A",
                    "checkpoint_time": "N/A",
                    "checkpoint_completion_time": "N/A"
                })
    return out


def parse_bitrot_scrub_status(data):
    raise NotImplementedError("Bitrot Scrub Status")


def parse_rebalance_status(data):
    status = etree.fromstring(data).find('volRebalance')
    result = {}
    try:
        result['task_id'] = status.find('task-id').text
        result['op_type'] = ROT(int(status.find('op').text)).name
        result['node_count'] = int(status.find('nodeCount').text)

        # individual node section
        result['nodes'] = []
        for i in status:
            if i.tag != 'node':
                continue
            else:
                result['nodes'].append({
                    'node_name': i.find('nodeName').text,
                    'id': i.find('id').text,
                    'files': i.find('files').text,
                    'size': i.find('size').text,
                    'lookups': i.find('lookups').text,
                    'failures': i.find('failures').text,
                    'skipped': i.find('skipped').text,
                    'status': i.find('statusStr').text,
                    'runtime': i.find('runtime').text,
                })

        # aggregate section
        result['aggregate'] = {
            'files': status.find('aggregate/files').text,
            'size': status.find('aggregate/size').text,
            'lookups': status.find('aggregate/lookups').text,
            'failures': status.find('aggregate/failures').text,
            'skipped': status.find('aggregate/skipped').text,
            'status': status.find('aggregate/statusStr').text,
            'runtime': status.find('aggregate/runtime').text,
        }
    except (ParseError, AttributeError, ValueError) as err:
        raise GlusterCmdOutputParseError(err)

    return result


def parse_quota_list_paths(quotainfo):
    volquota = etree.fromstring(quotainfo)
    quota_list = []

    for limit in volquota.findall('volQuota/limit'):
        quota_list.append({
            'path': limit.find('path').text,
            'hard_limit': limit.find('hard_limit').text,
            'soft_limit_percent': limit.find('soft_limit_percent').text,
            'used_space': limit.find('used_space').text,
            'avail_space': limit.find('avail_space').text,
            'sl_exceeded': limit.find('sl_exceeded').text,
            'hl_exceeded': limit.find('hl_exceeded').text
        })
    return quota_list


def parse_quota_list_objects(data):
    raise NotImplementedError("Quota List Objects")


def parse_georep_config(data):
    raise NotImplementedError("Georep Config")


def parse_remove_brick_status(status):
    tree = etree.fromstring(status)

    result = {'nodes': [], 'aggregate': _parse_remove_aggregate(
        tree.find('volRemoveBrick/aggregate'))}

    for node_el in tree.findall('volRemoveBrick/node'):
        result['nodes'].append(_parse_remove_node(node_el))

    return result


def _parse_remove_node(node_el):
    value = {
        'name': node_el.find('nodeName').text,
        'id': node_el.find('id').text,
        'files': node_el.find('files').text,
        'size': node_el.find('size').text,
        'lookups': node_el.find('lookups').text,
        'failures': node_el.find('failures').text,
        'skipped': node_el.find('skipped').text,
        'status_code': node_el.find('status').text,
        'status': node_el.find('statusStr').text,
        'runtime': node_el.find('runtime').text
    }

    return value


def _parse_remove_aggregate(aggregate_el):
    value = {
        'files': aggregate_el.find('files').text,
        'size': aggregate_el.find('size').text,
        'lookups': aggregate_el.find('lookups').text,
        'failures': aggregate_el.find('failures').text,
        'skipped': aggregate_el.find('skipped').text,
        'status_code': aggregate_el.find('status').text,
        'status': aggregate_el.find('statusStr').text,
        'runtime': aggregate_el.find('runtime').text
    }

    return value


def parse_tier_detach(data):
    raise NotImplementedError("Tier detach Status")


def parse_tier_status(data):
    raise NotImplementedError("Tier Status")


def parse_volume_list(data):
    xml = etree.fromstring(data)
    volumes = []
    for volume_el in xml.findall('volList/volume'):
        volumes.append(volume_el.text)
    return volumes


def parse_heal_info(data):
    xml = etree.fromstring(data)
    healinfo = []
    for brick_el in xml.findall('healInfo/bricks/brick'):
        healinfo.append({
            'name': brick_el.find('name').text,
            'status': brick_el.find('status').text,
            'host_uuid': brick_el.attrib['hostUuid'],
            'nr_entries': brick_el.find('numberOfEntries').text
        })
    return healinfo


def parse_heal_statistics(data):
    raise NotImplementedError("Heal Statistics")


def parse_snapshot_status(data):
    raise NotImplementedError("Snapshot Status")


def parse_snapshot_info(data):
    xml = etree.fromstring(data)
    snapinfo = []
    for snap_el in xml.findall('snapInfo/snapshots/snapshot'):
        snapdata = {
            'name': snap_el.find('name').text,
            'create_time': snap_el.find('createTime').text,
            'uuid': snap_el.find('uuid').text,
            'status': snap_el.find('snapVolume/status').text,
        }
        if snap_el.find('snapVolume/originVolume'):
            snapdata['origin_volume'] = \
                snap_el.find('snapVolume/originVolume/name').text
        snapinfo.append(snapdata)
    return snapinfo


def parse_snapshot_list(data):
    xml = etree.fromstring(data)
    snapshots = []
    for snap_el in xml.findall('snapList/snapshot'):
        snapshots.append(snap_el.text)
    return snapshots


def _parse_a_peer(peer):
    value = {
        'uuid': peer.find('uuid').text,
        'hostname': peer.find('hostname').text,
        'connected': peer.find('connected').text
    }

    if value['connected'] == '0':
        value['connected'] = "Disconnected"
    elif value['connected'] == '1':
        value['connected'] = "Connected"

    return value


def parse_peer_status(data):
    tree = etree.fromstring(data)
    peers = []
    for peer_el in tree.findall('peerStatus/peer'):
        try:
            peers.append(_parse_a_peer(peer_el))
        except (ParseError, AttributeError, ValueError) as err:
            raise GlusterCmdOutputParseError(err)

    return peers


def parse_pool_list(data):
    tree = etree.fromstring(data)
    pools = []
    for peer_el in tree.findall('peerStatus/peer'):
        try:
            pools.append(_parse_a_peer(peer_el))
        except (ParseError, AttributeError, ValueError) as err:
            raise GlusterCmdOutputParseError(err)

    return pools
