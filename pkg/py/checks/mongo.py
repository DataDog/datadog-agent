# stdlib
import time

# 3p
import pymongo

# project
from checks import AgentCheck
from config import _is_affirmative
from util import get_hostname

DEFAULT_TIMEOUT = 30
GAUGE = AgentCheck.gauge
RATE = AgentCheck.rate


class MongoDb(AgentCheck):
    SERVICE_CHECK_NAME = 'mongodb.can_connect'
    SOURCE_TYPE_NAME = 'mongodb'

    COMMON_METRICS = {
        "asserts.msg": RATE,
        "asserts.regular": RATE,
        "asserts.rollovers": RATE,
        "asserts.user": RATE,
        "asserts.warning": RATE,
        "connections.available": GAUGE,
        "connections.current": GAUGE,
        "connections.totalCreated": GAUGE,
        "cursors.timedOut": GAUGE,
        "cursors.totalOpen": GAUGE,
        "extra_info.heap_usage_bytes": RATE,
        "extra_info.page_faults": RATE,
        "globalLock.activeClients.readers": GAUGE,
        "globalLock.activeClients.total": GAUGE,
        "globalLock.activeClients.writers": GAUGE,
        "globalLock.currentQueue.readers": GAUGE,
        "globalLock.currentQueue.total": GAUGE,
        "globalLock.currentQueue.writers": GAUGE,
        "globalLock.totalTime": GAUGE,
        "mem.bits": GAUGE,
        "mem.mapped": GAUGE,
        "mem.mappedWithJournal": GAUGE,
        "mem.resident": GAUGE,
        "mem.virtual": GAUGE,
        "metrics.document.deleted": RATE,
        "metrics.document.inserted": RATE,
        "metrics.document.returned": RATE,
        "metrics.document.updated": RATE,
        "metrics.getLastError.wtime.num": RATE,
        "metrics.getLastError.wtime.totalMillis": RATE,
        "metrics.getLastError.wtimeouts": RATE,
        "metrics.operation.fastmod": RATE,
        "metrics.operation.idhack": RATE,
        "metrics.operation.scanAndOrder": RATE,
        "metrics.queryExecutor.scanned": RATE,
        "metrics.record.moves": RATE,
        "metrics.repl.apply.batches.num": RATE,
        "metrics.repl.apply.batches.totalMillis": RATE,
        "metrics.repl.apply.ops": RATE,
        "metrics.repl.buffer.count": GAUGE,
        "metrics.repl.buffer.maxSizeBytes": GAUGE,
        "metrics.repl.buffer.sizeBytes": GAUGE,
        "metrics.repl.network.bytes": RATE,
        "metrics.repl.network.getmores.num": RATE,
        "metrics.repl.network.getmores.totalMillis": RATE,
        "metrics.repl.network.ops": RATE,
        "metrics.repl.network.readersCreated": RATE,
        "metrics.repl.oplog.insert.num": RATE,
        "metrics.repl.oplog.insert.totalMillis": RATE,
        "metrics.repl.oplog.insertBytes": RATE,
        "metrics.repl.preload.indexes.num": RATE,
        "metrics.repl.preload.indexes.totalMillis": RATE,
        "metrics.ttl.deletedDocuments": RATE,
        "metrics.ttl.passes": RATE,
        "opcounters.command": RATE,
        "opcounters.delete": RATE,
        "opcounters.getmore": RATE,
        "opcounters.insert": RATE,
        "opcounters.query": RATE,
        "opcounters.update": RATE,
        "opcountersRepl.command": RATE,
        "opcountersRepl.delete": RATE,
        "opcountersRepl.getmore": RATE,
        "opcountersRepl.insert": RATE,
        "opcountersRepl.query": RATE,
        "opcountersRepl.update": RATE,
        "replSet.health": GAUGE,
        "replSet.replicationLag": GAUGE,
        "replSet.state": GAUGE,
        "stats.avgObjSize": GAUGE,
        "stats.collections": GAUGE,
        "stats.dataSize": GAUGE,
        "stats.fileSize": GAUGE,
        "stats.indexes": GAUGE,
        "stats.indexSize": GAUGE,
        "stats.nsSizeMB": GAUGE,
        "stats.numExtents": GAUGE,
        "stats.objects": GAUGE,
        "stats.storageSize": GAUGE,
        "uptime": GAUGE,
    }

    V2_ONLY_METRICS = {
        "globalLock.lockTime": GAUGE,
        "globalLock.ratio": GAUGE,                  # < 2.2
        "indexCounters.accesses": RATE,
        "indexCounters.btree.accesses": RATE,       # < 2.4
        "indexCounters.btree.hits": RATE,           # < 2.4
        "indexCounters.btree.misses": RATE,         # < 2.4
        "indexCounters.btree.missRatio": GAUGE,     # < 2.4
        "indexCounters.hits": RATE,
        "indexCounters.misses": RATE,
        "indexCounters.missRatio": GAUGE,
        "indexCounters.resets": RATE,
    }

    TCMALLOC_METRICS = {
        "tcmalloc.generic.current_allocated_bytes": GAUGE,
        "tcmalloc.generic.heap_size": GAUGE,
        "tcmalloc.tcmalloc.aggressive_memory_decommit": GAUGE,
        "tcmalloc.tcmalloc.central_cache_free_bytes": GAUGE,
        "tcmalloc.tcmalloc.current_total_thread_cache_bytes": GAUGE,
        "tcmalloc.tcmalloc.max_total_thread_cache_bytes": GAUGE,
        "tcmalloc.tcmalloc.pageheap_free_bytes": GAUGE,
        "tcmalloc.tcmalloc.pageheap_unmapped_bytes": GAUGE,
        "tcmalloc.tcmalloc.thread_cache_free_bytes": GAUGE,
        "tcmalloc.tcmalloc.transfer_cache_free_bytes": GAUGE,
    }

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self._last_state_by_server = {}
        self.metrics_to_collect_by_instance = {}

    def get_library_versions(self):
        return {"pymongo": pymongo.version}

    def check_last_state(self, state, clean_server_name, agentConfig):
        if self._last_state_by_server.get(clean_server_name, -1) != state:
            self._last_state_by_server[clean_server_name] = state
            return self.create_event(state, clean_server_name, agentConfig)

    def create_event(self, state, clean_server_name, agentConfig):
        """Create an event with a message describing the replication
            state of a mongo node"""

        def get_state_description(state):
            if state == 0:
                return 'Starting Up'
            elif state == 1:
                return 'Primary'
            elif state == 2:
                return 'Secondary'
            elif state == 3:
                return 'Recovering'
            elif state == 4:
                return 'Fatal'
            elif state == 5:
                return 'Starting up (forking threads)'
            elif state == 6:
                return 'Unknown'
            elif state == 7:
                return 'Arbiter'
            elif state == 8:
                return 'Down'
            elif state == 9:
                return 'Rollback'

        status = get_state_description(state)
        hostname = get_hostname(agentConfig)
        msg_title = "%s is %s" % (clean_server_name, status)
        msg = "MongoDB %s just reported as %s" % (clean_server_name, status)

        self.event({
            'timestamp': int(time.time()),
            'event_type': 'Mongo',
            'api_key': agentConfig.get('api_key', ''),
            'msg_title': msg_title,
            'msg_text': msg,
            'host': hostname
        })

    @classmethod
    def _build_metric_list_to_collect(cls, collect_tcmalloc_metrics=False):
        """
        Build the metric list to collect based on the instance preferences.
        """
        metrics_to_collect = {}

        # Defaut metrics
        metrics_to_collect.update(cls.COMMON_METRICS)
        metrics_to_collect.update(cls.V2_ONLY_METRICS)

        # Optional metrics
        if collect_tcmalloc_metrics:
            metrics_to_collect.update(cls.TCMALLOC_METRICS)

        return metrics_to_collect

    def _get_metrics_to_collect(self, instance_key, **instance_preferences):
        """
        Return and cache the list of metrics to collect.
        """
        if instance_key not in self.metrics_to_collect_by_instance:
            self.metrics_to_collect_by_instance[instance_key] = \
                self._build_metric_list_to_collect(**instance_preferences)
        return self.metrics_to_collect_by_instance[instance_key]

    def _normalize(self, metric_name, submit_method):
        """
        Normalize the metric name considering its type.
        """
        if submit_method == RATE:
            return self.normalize(metric_name.lower(), 'mongodb') + "ps"

        return self.normalize(metric_name.lower(), 'mongodb')

    def check(self, instance):
        """
        Returns a dictionary that looks a lot like what's sent back by
        db.serverStatus()
        """
        if 'server' not in instance:
            raise Exception("Missing 'server' in mongo config")

        server = instance['server']

        ssl_params = {
            'ssl': instance.get('ssl', None),
            'ssl_keyfile': instance.get('ssl_keyfile', None),
            'ssl_certfile': instance.get('ssl_certfile', None),
            'ssl_cert_reqs': instance.get('ssl_cert_reqs', None),
            'ssl_ca_certs': instance.get('ssl_ca_certs', None)
        }

        for key, param in ssl_params.items():
            if param is None:
                del ssl_params[key]

        # Configuration a URL, mongodb://user:pass@server/db
        parsed = pymongo.uri_parser.parse_uri(server)
        username = parsed.get('username')
        password = parsed.get('password')
        db_name = parsed.get('database')
        clean_server_name = server.replace(password, "*" * 5) if password is not None else server

        tags = instance.get('tags', [])
        tags.append('server:%s' % clean_server_name)

        # Get the list of metrics to collect
        collect_tcmalloc_metrics = _is_affirmative(
            instance.get('collect_tcmalloc_metrics', False)
        )
        metrics_to_collect = self._get_metrics_to_collect(
            server,
            collect_tcmalloc_metrics=collect_tcmalloc_metrics,
        )

        # de-dupe tags to avoid a memory leak
        tags = list(set(tags))

        if not db_name:
            self.log.info('No MongoDB database found in URI. Defaulting to admin.')
            db_name = 'admin'

        service_check_tags = [
            "db:%s" % db_name
        ]

        nodelist = parsed.get('nodelist')
        if nodelist:
            host = nodelist[0][0]
            port = nodelist[0][1]
            service_check_tags = service_check_tags + [
                "host:%s" % host,
                "port:%s" % port
            ]

        do_auth = True
        if username is None or password is None:
            self.log.debug("Mongo: cannot extract username and password from config %s" % server)
            do_auth = False

        timeout = float(instance.get('timeout', DEFAULT_TIMEOUT)) * 1000
        try:
            cli = pymongo.mongo_client.MongoClient(
                server,
                socketTimeoutMS=timeout,
                read_preference=pymongo.ReadPreference.PRIMARY_PREFERRED,
                **ssl_params)
            # some commands can only go against the admin DB
            admindb = cli['admin']
            db = cli[db_name]
        except Exception:
            self.service_check(
                self.SERVICE_CHECK_NAME,
                AgentCheck.CRITICAL,
                tags=service_check_tags)
            raise

        if do_auth and not db.authenticate(username, password):
            message = "Mongo: cannot connect with config %s" % server
            self.service_check(
                self.SERVICE_CHECK_NAME,
                AgentCheck.CRITICAL,
                tags=service_check_tags,
                message=message)
            raise Exception(message)

        self.service_check(
            self.SERVICE_CHECK_NAME,
            AgentCheck.OK,
            tags=service_check_tags)

        status = db["$cmd"].find_one({"serverStatus": 1, "tcmalloc": collect_tcmalloc_metrics})
        if status['ok'] == 0:
            raise Exception(status['errmsg'].__str__())

        status['stats'] = db.command('dbstats')
        dbstats = {}
        dbstats[db_name] = {'stats': status['stats']}

        # Handle replica data, if any
        # See
        # http://www.mongodb.org/display/DOCS/Replica+Set+Commands#ReplicaSetCommands-replSetGetStatus  # noqa
        try:
            data = {}
            dbnames = []

            replSet = admindb.command('replSetGetStatus')
            if replSet:
                primary = None
                current = None

                # need a new connection to deal with replica sets
                setname = replSet.get('set')
                cli = pymongo.mongo_client.MongoClient(
                    server,
                    socketTimeoutMS=timeout,
                    replicaset=setname,
                    read_preference=pymongo.ReadPreference.NEAREST,
                    **ssl_params)
                db = cli[db_name]

                if do_auth and not db.authenticate(username, password):
                    message = ("Mongo: cannot connect with config %s" % server)
                    self.service_check(
                        self.SERVICE_CHECK_NAME,
                        AgentCheck.CRITICAL,
                        tags=service_check_tags,
                        message=message)
                    raise Exception(message)

                # find nodes: master and current node (ourself)
                for member in replSet.get('members'):
                    if member.get('self'):
                        current = member
                    if int(member.get('state')) == 1:
                        primary = member

                # If we have both we can compute a lag time
                if current is not None and primary is not None:
                    lag = primary['optimeDate'] - current['optimeDate']
                    # Python 2.7 has this built in, python < 2.7 don't...
                    if hasattr(lag, 'total_seconds'):
                        data['replicationLag'] = lag.total_seconds()
                    else:
                        data['replicationLag'] = (
                            lag.microseconds +
                            (lag.seconds + lag.days * 24 * 3600) * 10**6
                        ) / 10.0**6

                if current is not None:
                    data['health'] = current['health']

                data['state'] = replSet['myState']
                self.check_last_state(
                    data['state'],
                    clean_server_name,
                    self.agentConfig)
                status['replSet'] = data

        except Exception as e:
            if "OperationFailure" in repr(e) and "replSetGetStatus" in str(e):
                pass
            else:
                raise e

        # If these keys exist, remove them for now as they cannot be serialized
        try:
            status['backgroundFlushing'].pop('last_finished')
        except KeyError:
            pass
        try:
            status.pop('localTime')
        except KeyError:
            pass

        dbnames = cli.database_names()
        for db_n in dbnames:
            db_aux = cli[db_n]
            dbstats[db_n] = {'stats': db_aux.command('dbstats')}

        # Go through the metrics and save the values
        for metric_name, submit_method in metrics_to_collect.iteritems():
            # each metric is of the form: x.y.z with z optional
            # and can be found at status[x][y][z]
            value = status

            if metric_name.startswith('stats'):
                continue
            else:
                try:
                    for c in metric_name.split("."):
                        value = value[c]
                except KeyError:
                    continue

            # value is now status[x][y][z]
            if not isinstance(value, (int, long, float)):
                raise TypeError(
                    u"{0} value is a {1}, it should be an int, a float or a long instead."
                    .format(metric_name, type(value)))

            # Submit the metric
            metric_name = self._normalize(metric_name, submit_method)
            submit_method(self, metric_name, value, tags=tags)

        for st, value in dbstats.iteritems():
            for metric_name, submit_method in metrics_to_collect.iteritems():
                if not metric_name.startswith('stats.'):
                    continue

                try:
                    val = value['stats'][metric_name.split('.')[1]]
                except KeyError:
                    continue

                # value is now status[x][y][z]
                if not isinstance(val, (int, long, float)):
                    raise TypeError(
                        u"{0} value is a {1}, it should be an int, a float or a long instead."
                        .format(metric_name, type(val))
                    )

                # Submit the metric
                metric_name = self._normalize(metric_name, submit_method)
                metrics_tags = tags + ['cluster:db:%s' % st]
                submit_method(self, metric_name, val, tags=metrics_tags)
