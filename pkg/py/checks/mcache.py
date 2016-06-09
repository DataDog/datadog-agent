# 3rd party
import memcache

# project
from checks import AgentCheck

# Ref: http://code.sixapart.com/svn/memcached/trunk/server/doc/protocol.txt
# Name              Type     Meaning
# ----------------------------------
# pid               32u      Process id of this server process
# uptime            32u      Number of seconds this server has been running
# time              32u      current UNIX time according to the server
# version           string   Version string of this server
# pointer_size      32       Default size of pointers on the host OS
#                            (generally 32 or 64)
# rusage_user       32u:32u  Accumulated user time for this process
#                            (seconds:microseconds)
# rusage_system     32u:32u  Accumulated system time for this process
#                            (seconds:microseconds)
# curr_items        32u      Current number of items stored by the server
# total_items       32u      Total number of items stored by this server
#                            ever since it started
# bytes             64u      Current number of bytes used by this server
#                            to store items
# curr_connections  32u      Number of open connections
# total_connections 32u      Total number of connections opened since
#                            the server started running
# connection_structures 32u  Number of connection structures allocated
#                            by the server
# cmd_get           64u      Cumulative number of retrieval requests
# cmd_set           64u      Cumulative number of storage requests
# get_hits          64u      Number of keys that have been requested and
#                            found present
# get_misses        64u      Number of items that have been requested
#                            and not found
# delete_misses     64u      Number of deletions reqs for missing keys
# delete_hits       64u      Number of deletion reqs resulting in
#                            an item being removed.
# evictions         64u      Number of valid items removed from cache
#                            to free memory for new items
# bytes_read        64u      Total number of bytes read by this server
#                            from network
# bytes_written     64u      Total number of bytes sent by this server to
#                            network
# limit_maxbytes    32u      Number of bytes this server is allowed to
#                            use for storage.
# threads           32u      Number of worker threads requested.
#                            (see doc/threads.txt)
# listen_disabled_num 32u    How many times the server has reached maxconns
#                            (see https://code.google.com/p/memcached/wiki/Timeouts)
#     >>> mc.get_stats()
# [('127.0.0.1:11211 (1)', {'pid': '2301', 'total_items': '2',
# 'uptime': '80', 'listen_disabled_num': '0', 'version': '1.2.8',
# 'limit_maxbytes': '67108864', 'rusage_user': '0.002532',
# 'bytes_read': '51', 'accepting_conns': '1', 'rusage_system':
# '0.007445', 'cmd_get': '0', 'curr_connections': '4', 'threads': '2',
# 'total_connections': '5', 'cmd_set': '2', 'curr_items': '0',
# 'get_misses': '0', 'cmd_flush': '0', 'evictions': '0', 'bytes': '0',
# 'connection_structures': '5', 'bytes_written': '25', 'time':
# '1306364220', 'pointer_size': '64', 'get_hits': '0'})]

# For Membase it gets worse
# http://www.couchbase.org/wiki/display/membase/Membase+Statistics
# https://github.com/membase/ep-engine/blob/master/docs/stats.org


class Memcache(AgentCheck):

    SOURCE_TYPE_NAME = 'memcached'

    DEFAULT_PORT = 11211

    GAUGES = [
        "total_items",
        "curr_items",
        "limit_maxbytes",
        "uptime",
        "bytes",
        "curr_connections",
        "connection_structures",
        "threads",
        "pointer_size"
    ]

    RATES = [
        "rusage_user",
        "rusage_system",
        "cmd_get",
        "cmd_set",
        "cmd_flush",
        "get_hits",
        "get_misses",
        "delete_misses",
        "delete_hits",
        "evictions",
        "bytes_read",
        "bytes_written",
        "cas_misses",
        "cas_hits",
        "cas_badval",
        "total_connections",
        "listen_disabled_num"
    ]

    SERVICE_CHECK = 'memcache.can_connect'

    def get_library_versions(self):
        return {"memcache": memcache.__version__}

    def _get_metrics(self, server, port, tags):
        mc = None  # client
        service_check_tags = ["host:%s" % server, "port:%s" % port]
        try:
            self.log.debug("Connecting to %s:%s tags:%s", server, port, tags)
            mc = memcache.Client(["%s:%s" % (server, port)])
            raw_stats = mc.get_stats()

            assert len(raw_stats) == 1 and len(raw_stats[0]) == 2,\
                "Malformed response: %s" % raw_stats


            # Access the dict
            stats = raw_stats[0][1]
            for metric in stats:
                # Check if metric is a gauge or rate
                if metric in self.GAUGES:
                    our_metric = self.normalize(metric.lower(), 'memcache')
                    self.gauge(our_metric, float(stats[metric]), tags=tags)

                # Tweak the name if it's a rate so that we don't use the exact
                # same metric name as the memcache documentation
                if metric in self.RATES:
                    our_metric = self.normalize(
                        "{0}_rate".format(metric.lower()), 'memcache')
                    self.rate(our_metric, float(stats[metric]), tags=tags)

            # calculate some metrics based on other metrics.
            # stats should be present, but wrap in try/except
            # and log an exception just in case.
            try:
                self.gauge(
                    "memcache.get_hit_percent",
                    100.0 * float(stats["get_hits"]) / float(stats["cmd_get"]),
                    tags=tags,
                )
            except ZeroDivisionError:
                pass

            try:
                self.gauge(
                    "memcache.fill_percent",
                    100.0 * float(stats["bytes"]) / float(stats["limit_maxbytes"]),
                    tags=tags,
                )
            except ZeroDivisionError:
                pass

            try:
                self.gauge(
                    "memcache.avg_item_size",
                    float(stats["bytes"]) / float(stats["curr_items"]),
                    tags=tags,
                )
            except ZeroDivisionError:
                pass

            uptime = stats.get("uptime", 0)
            self.service_check(
                self.SERVICE_CHECK, AgentCheck.OK,
                tags=service_check_tags,
                message="Server has been up for %s seconds" % uptime)
        except AssertionError:
            self.service_check(
                self.SERVICE_CHECK, AgentCheck.CRITICAL,
                tags=service_check_tags,
                message="Unable to fetch stats from server")
            raise Exception(
                "Unable to retrieve stats from memcache instance: {0}:{1}."
                "Please check your configuration".format(server, port))

        if mc is not None:
            mc.disconnect_all()
            self.log.debug("Disconnected from memcached")
        del mc

    def check(self, instance):
        socket = instance.get('socket')
        server = instance.get('url')
        if not server and not socket:
            raise Exception('Either "url" or "socket" must be configured')

        if socket:
            server = 'unix'
            port = socket
        else:
            port = int(instance.get('port', self.DEFAULT_PORT))
        custom_tags = instance.get('tags') or []

        tags = ["url:{0}:{1}".format(server, port)] + custom_tags

        self._get_metrics(server, port, tags)
