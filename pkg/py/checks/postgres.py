"""PostgreSQL check

Collects database-wide metrics and optionally per-relation metrics, custom metrics.
"""
# stdlib
import socket

# 3rd party
import pg8000 as pg
from pg8000 import InterfaceError, ProgrammingError

# project
from checks import AgentCheck, CheckException
from config import _is_affirmative

MAX_CUSTOM_RESULTS = 100


class ShouldRestartException(Exception):
    pass


class PostgreSql(AgentCheck):
    """Collects per-database, and optionally per-relation metrics, custom metrics
    """
    SOURCE_TYPE_NAME = 'postgresql'
    RATE = AgentCheck.rate
    GAUGE = AgentCheck.gauge
    MONOTONIC = AgentCheck.monotonic_count
    SERVICE_CHECK_NAME = 'postgres.can_connect'

    # turning columns into tags
    DB_METRICS = {
        'descriptors': [
            ('datname', 'db')
        ],
        'metrics': {},
        'query': """
SELECT datname,
       %s
  FROM pg_stat_database
 WHERE datname not ilike 'template%%'
   AND datname not ilike 'postgres'
   AND datname not ilike 'rdsadmin'
""",
        'relation': False,
    }

    COMMON_METRICS = {
        'numbackends'       : ('postgresql.connections', GAUGE),
        'xact_commit'       : ('postgresql.commits', RATE),
        'xact_rollback'     : ('postgresql.rollbacks', RATE),
        'blks_read'         : ('postgresql.disk_read', RATE),
        'blks_hit'          : ('postgresql.buffer_hit', RATE),
        'tup_returned'      : ('postgresql.rows_returned', RATE),
        'tup_fetched'       : ('postgresql.rows_fetched', RATE),
        'tup_inserted'      : ('postgresql.rows_inserted', RATE),
        'tup_updated'       : ('postgresql.rows_updated', RATE),
        'tup_deleted'       : ('postgresql.rows_deleted', RATE),
        'pg_database_size(datname) as pg_database_size' : ('postgresql.database_size', GAUGE),
    }

    NEWER_92_METRICS = {
        'deadlocks'         : ('postgresql.deadlocks', RATE),
        'temp_bytes'        : ('postgresql.temp_bytes', RATE),
        'temp_files'        : ('postgresql.temp_files', RATE),
    }

    BGW_METRICS = {
        'descriptors': [],
        'metrics': {},
        'query': "select %s FROM pg_stat_bgwriter",
        'relation': False,
    }

    COMMON_BGW_METRICS = {
        'checkpoints_timed'    : ('postgresql.bgwriter.checkpoints_timed', MONOTONIC),
        'checkpoints_req'      : ('postgresql.bgwriter.checkpoints_requested', MONOTONIC),
        'buffers_checkpoint'   : ('postgresql.bgwriter.buffers_checkpoint', MONOTONIC),
        'buffers_clean'        : ('postgresql.bgwriter.buffers_clean', MONOTONIC),
        'maxwritten_clean'     : ('postgresql.bgwriter.maxwritten_clean', MONOTONIC),
        'buffers_backend'      : ('postgresql.bgwriter.buffers_backend', MONOTONIC),
        'buffers_alloc'        : ('postgresql.bgwriter.buffers_alloc', MONOTONIC),
    }

    NEWER_91_BGW_METRICS = {
        'buffers_backend_fsync': ('postgresql.bgwriter.buffers_backend_fsync', MONOTONIC),
    }

    NEWER_92_BGW_METRICS = {
        'checkpoint_write_time': ('postgresql.bgwriter.write_time', MONOTONIC),
        'checkpoint_sync_time' : ('postgresql.bgwriter.sync_time', MONOTONIC),
    }

    LOCK_METRICS = {
        'descriptors': [
            ('mode', 'lock_mode'),
            ('relname', 'table'),
        ],
        'metrics': {
            'lock_count'       : ('postgresql.locks', GAUGE),
        },
        'query': """
SELECT mode,
       pc.relname,
       count(*) AS %s
  FROM pg_locks l
  JOIN pg_class pc ON (l.relation = pc.oid)
 WHERE l.mode IS NOT NULL
   AND pc.relname NOT LIKE 'pg_%%'
 GROUP BY pc.relname, mode""",
        'relation': False,
    }


    REL_METRICS = {
        'descriptors': [
            ('relname', 'table'),
            ('schemaname', 'schema'),
        ],
        'metrics': {
            'seq_scan'          : ('postgresql.seq_scans', RATE),
            'seq_tup_read'      : ('postgresql.seq_rows_read', RATE),
            'idx_scan'          : ('postgresql.index_scans', RATE),
            'idx_tup_fetch'     : ('postgresql.index_rows_fetched', RATE),
            'n_tup_ins'         : ('postgresql.rows_inserted', RATE),
            'n_tup_upd'         : ('postgresql.rows_updated', RATE),
            'n_tup_del'         : ('postgresql.rows_deleted', RATE),
            'n_tup_hot_upd'     : ('postgresql.rows_hot_updated', RATE),
            'n_live_tup'        : ('postgresql.live_rows', GAUGE),
            'n_dead_tup'        : ('postgresql.dead_rows', GAUGE),
        },
        'query': """
SELECT relname,schemaname,%s
  FROM pg_stat_user_tables
 WHERE relname = ANY(array[%s])""",
        'relation': True,
    }

    IDX_METRICS = {
        'descriptors': [
            ('relname', 'table'),
            ('schemaname', 'schema'),
            ('indexrelname', 'index')
        ],
        'metrics': {
            'idx_scan'          : ('postgresql.index_scans', RATE),
            'idx_tup_read'      : ('postgresql.index_rows_read', RATE),
            'idx_tup_fetch'     : ('postgresql.index_rows_fetched', RATE),
        },
        'query': """
SELECT relname,
       schemaname,
       indexrelname,
       %s
  FROM pg_stat_user_indexes
 WHERE relname = ANY(array[%s])""",
        'relation': True,
    }

    SIZE_METRICS = {
        'descriptors': [
            ('relname', 'table'),
        ],
        'metrics': {
            'pg_table_size(C.oid) as table_size'  : ('postgresql.table_size', GAUGE),
            'pg_indexes_size(C.oid) as index_size' : ('postgresql.index_size', GAUGE),
            'pg_total_relation_size(C.oid) as total_size' : ('postgresql.total_size', GAUGE),
        },
        'relation': True,
        'query': """
SELECT
  relname,
  %s
FROM pg_class C
LEFT JOIN pg_namespace N ON (N.oid = C.relnamespace)
WHERE nspname NOT IN ('pg_catalog', 'information_schema') AND
  nspname !~ '^pg_toast' AND
  relkind IN ('r') AND
  relname = ANY(array[%s])"""
    }

    COUNT_METRICS = {
        'descriptors': [
            ('schemaname', 'schema')
        ],
        'metrics': {
            'pg_stat_user_tables': ('postgresql.table.count', GAUGE),
        },
        'relation': False,
        'query': """
SELECT schemaname, count(*)
FROM %s
GROUP BY schemaname
        """
    }

    REPLICATION_METRICS_9_1 = {
        'CASE WHEN pg_last_xlog_receive_location() = pg_last_xlog_replay_location() THEN 0 ELSE GREATEST (0, EXTRACT (EPOCH FROM now() - pg_last_xact_replay_timestamp())) END': ('postgresql.replication_delay', GAUGE),
    }

    REPLICATION_METRICS_9_2 = {
        'abs(pg_xlog_location_diff(pg_last_xlog_receive_location(), pg_last_xlog_replay_location())) AS replication_delay_bytes': ('postgres.replication_delay_bytes', GAUGE)
    }

    REPLICATION_METRICS = {
        'descriptors': [],
        'metrics': {},
        'relation': False,
        'query': """
SELECT %s
 WHERE (SELECT pg_is_in_recovery())"""
    }

    CONNECTION_METRICS = {
        'descriptors': [],
        'metrics': {
            'MAX(setting) AS max_connections': ('postgresql.max_connections', GAUGE),
            'SUM(numbackends)/MAX(setting) AS pct_connections': ('postgresql.percent_usage_connections', GAUGE),
        },
        'relation': False,
        'query': """
WITH max_con AS (SELECT setting::float FROM pg_settings WHERE name = 'max_connections')
SELECT %s
  FROM pg_stat_database, max_con
"""
    }

    STATIO_METRICS = {
        'descriptors': [
            ('relname', 'table'),
            ('schemaname', 'schema')
        ],
        'metrics': {
            'heap_blks_read'  : ('postgresql.heap_blocks_read', RATE),
            'heap_blks_hit'   : ('postgresql.heap_blocks_hit', RATE),
            'idx_blks_read'   : ('postgresql.index_blocks_read', RATE),
            'idx_blks_hit'    : ('postgresql.index_blocks_hit', RATE),
            'toast_blks_read' : ('postgresql.toast_blocks_read', RATE),
            'toast_blks_hit'  : ('postgresql.toast_blocks_hit', RATE),
            'tidx_blks_read'  : ('postgresql.toast_index_blocks_read', RATE),
            'tidx_blks_hit'   : ('postgresql.toast_index_blocks_hit', RATE),
        },
        'query': """
SELECT relname,
       schemaname,
       %s
  FROM pg_statio_user_tables
 WHERE relname = ANY(array[%s])""",
        'relation': True,
    }

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.dbs = {}
        self.versions = {}
        self.instance_metrics = {}
        self.bgw_metrics = {}
        self.db_instance_metrics = []
        self.db_bgw_metrics = []
        self.replication_metrics = {}
        self.custom_metrics = {}

    def _get_version(self, key, db):
        if key not in self.versions:
            cursor = db.cursor()
            cursor.execute('SHOW SERVER_VERSION;')
            result = cursor.fetchone()
            try:
                version = map(int, result[0].split('.'))
            except Exception:
                version = result[0]
            self.versions[key] = version

        self.service_metadata('version', self.versions[key])
        return self.versions[key]

    def _is_above(self, key, db, version_to_compare):
        version = self._get_version(key, db)
        if type(version) == list:
            return version >= version_to_compare

        return False

    def _is_9_1_or_above(self, key, db):
        return self._is_above(key, db, [9,1,0])

    def _is_9_2_or_above(self, key, db):
        return self._is_above(key, db, [9,2,0])

    def _get_instance_metrics(self, key, db):
        """Use either COMMON_METRICS or COMMON_METRICS + NEWER_92_METRICS
        depending on the postgres version.
        Uses a dictionnary to save the result for each instance
        """
        # Extended 9.2+ metrics if needed
        metrics = self.instance_metrics.get(key)

        if metrics is None:
            # Hack to make sure that if we have multiple instances that connect to
            # the same host, port, we don't collect metrics twice
            # as it will result in https://github.com/DataDog/dd-agent/issues/1211
            sub_key = key[:2]
            if sub_key in self.db_instance_metrics:
                self.instance_metrics[key] = None
                self.log.debug("Not collecting instance metrics for key: {0} as"
                    " they are already collected by another instance".format(key))
                return None

            self.db_instance_metrics.append(sub_key)

            if self._is_9_2_or_above(key, db):
                self.instance_metrics[key] = dict(self.COMMON_METRICS, **self.NEWER_92_METRICS)
            else:
                self.instance_metrics[key] = dict(self.COMMON_METRICS)
            metrics = self.instance_metrics.get(key)
        return metrics

    def _get_bgw_metrics(self, key, db):
        """Use either COMMON_BGW_METRICS or COMMON_BGW_METRICS + NEWER_92_BGW_METRICS
        depending on the postgres version.
        Uses a dictionnary to save the result for each instance
        """
        # Extended 9.2+ metrics if needed
        metrics = self.bgw_metrics.get(key)

        if metrics is None:
            # Hack to make sure that if we have multiple instances that connect to
            # the same host, port, we don't collect metrics twice
            # as it will result in https://github.com/DataDog/dd-agent/issues/1211
            sub_key = key[:2]
            if sub_key in self.db_bgw_metrics:
                self.bgw_metrics[key] = None
                self.log.debug("Not collecting bgw metrics for key: {0} as"
                    " they are already collected by another instance".format(key))
                return None

            self.db_bgw_metrics.append(sub_key)

            self.bgw_metrics[key] = dict(self.COMMON_BGW_METRICS)
            if self._is_9_1_or_above(key, db):
                self.bgw_metrics[key].update(self.NEWER_91_BGW_METRICS)
            if self._is_9_2_or_above(key, db):
                self.bgw_metrics[key].update(self.NEWER_92_BGW_METRICS)
            metrics = self.bgw_metrics.get(key)
        return metrics

    def _get_replication_metrics(self, key, db):
        """ Use either REPLICATION_METRICS_9_1 or REPLICATION_METRICS_9_1 + REPLICATION_METRICS_9_2
        depending on the postgres version.
        Uses a dictionnary to save the result for each instance
        """
        metrics = self.replication_metrics.get(key)
        if self._is_9_1_or_above(key, db) and metrics is None:
            self.replication_metrics[key] = dict(self.REPLICATION_METRICS_9_1)
            if self._is_9_2_or_above(key, db):
                self.replication_metrics[key].update(self.REPLICATION_METRICS_9_2)
            metrics = self.replication_metrics.get(key)
        return metrics

    def _build_relations_config(self, yamlconfig):
        """Builds a dictionary from relations configuration while maintaining compatibility
        """
        config = {}
        for element in yamlconfig:
            try:
                if isinstance(element, str):
                    config[element] = {'relation_name': element, 'schemas': []}
                elif isinstance(element, dict):
                    name = element['relation_name']
                    config[name] = {}
                    config[name]['schemas'] = element['schemas']
                    config[name]['relation_name'] = name
                else:
                    self.log.warn('Unhandled relations config type: %s' % str(element))
            except KeyError:
                self.log.warn('Failed to parse config element=%s, check syntax' % str(element))
        return config

    def _collect_stats(self, key, db, instance_tags, relations, custom_metrics):
        """Query pg_stat_* for various metrics
        If relations is not an empty list, gather per-relation metrics
        on top of that.
        If custom_metrics is not an empty list, gather custom metrics defined in postgres.yaml
        """

        metric_scope = [
            self.CONNECTION_METRICS,
            self.LOCK_METRICS,
            self.COUNT_METRICS,
        ]

        # These are added only once per PG server, thus the test
        db_instance_metrics = self._get_instance_metrics(key, db)
        bgw_instance_metrics = self._get_bgw_metrics(key, db)

        if db_instance_metrics is not None:
            # FIXME: constants shouldn't be modified
            self.DB_METRICS['metrics'] = db_instance_metrics
            metric_scope.append(self.DB_METRICS)

        if bgw_instance_metrics is not None:
            # FIXME: constants shouldn't be modified
            self.BGW_METRICS['metrics'] = bgw_instance_metrics
            metric_scope.append(self.BGW_METRICS)

        # Do we need relation-specific metrics?
        if relations:
            metric_scope += [
                self.REL_METRICS,
                self.IDX_METRICS,
                self.SIZE_METRICS,
                self.STATIO_METRICS
            ]
            relations_config = self._build_relations_config(relations)

        replication_metrics = self._get_replication_metrics(key, db)
        if replication_metrics is not None:
            # FIXME: constants shouldn't be modified
            self.REPLICATION_METRICS['metrics'] = replication_metrics
            metric_scope.append(self.REPLICATION_METRICS)

        full_metric_scope = list(metric_scope) + custom_metrics
        try:
            cursor = db.cursor()

            for scope in full_metric_scope:
                if scope == self.REPLICATION_METRICS or not self._is_above(key, db, [9,0,0]):
                    log_func = self.log.debug
                else:
                    log_func = self.log.warning

                # build query
                cols = scope['metrics'].keys()  # list of metrics to query, in some order
                # we must remember that order to parse results

                try:
                    # if this is a relation-specific query, we need to list all relations last
                    if scope['relation'] and len(relations) > 0:
                        relnames = ', '.join("'{0}'".format(w) for w in relations_config.iterkeys())
                        query = scope['query'] % (", ".join(cols), "%s")  # Keep the last %s intact
                        self.log.debug("Running query: %s with relations: %s" % (query, relnames))
                        cursor.execute(query % (relnames))
                    else:
                        query = scope['query'] % (", ".join(cols))
                        self.log.debug("Running query: %s" % query)
                        cursor.execute(query.replace(r'%', r'%%'))

                    results = cursor.fetchall()
                except ProgrammingError, e:
                    log_func("Not all metrics may be available: %s" % str(e))
                    continue

                if not results:
                    continue

                if scope in custom_metrics and len(results) > MAX_CUSTOM_RESULTS:
                    self.warning(
                        "Query: {0} returned more than {1} results ({2}). Truncating"
                        .format(query, MAX_CUSTOM_RESULTS, len(results))
                    )
                    results = results[:MAX_CUSTOM_RESULTS]

                # FIXME this cramps my style
                if scope == self.DB_METRICS:
                    self.gauge("postgresql.db.count", len(results),
                        tags=[t for t in instance_tags if not t.startswith("db:")])

                desc = scope['descriptors']

                # parse & submit results
                # A row should look like this
                # (descriptor, descriptor, ..., value, value, value, value, ...)
                # with descriptor a PG relation or index name, which we use to create the tags
                for row in results:
                    # Check that all columns will be processed
                    assert len(row) == len(cols) + len(desc)

                    # build a map of descriptors and their values
                    desc_map = dict(zip([x[1] for x in desc], row[0:len(desc)]))
                    if 'schema' in desc_map:
                        try:
                            relname = desc_map['table']
                            config_schemas = relations_config[relname]['schemas']
                            if config_schemas and desc_map['schema'] not in config_schemas:
                                continue
                        except KeyError:
                            pass

                    # Build tags
                    # descriptors are: (pg_name, dd_tag_name): value
                    # Special-case the "db" tag, which overrides the one that is passed as instance_tag
                    # The reason is that pg_stat_database returns all databases regardless of the
                    # connection.
                    if not scope['relation']:
                        tags = [t for t in instance_tags if not t.startswith("db:")]
                    else:
                        tags = [t for t in instance_tags]

                    tags += [("%s:%s" % (k,v)) for (k,v) in desc_map.iteritems()]

                    # [(metric-map, value), (metric-map, value), ...]
                    # metric-map is: (dd_name, "rate"|"gauge")
                    # shift the results since the first columns will be the "descriptors"
                    values = zip([scope['metrics'][c] for c in cols], row[len(desc):])

                    # To submit simply call the function for each value v
                    # v[0] == (metric_name, submit_function)
                    # v[1] == the actual value
                    # tags are
                    for v in values:
                        v[0][1](self, v[0][0], v[1], tags=tags)

            cursor.close()
        except InterfaceError, e:
            self.log.error("Connection error: %s" % str(e))
            raise ShouldRestartException
        except socket.error, e:
            self.log.error("Connection error: %s" % str(e))
            raise ShouldRestartException

    def _get_service_check_tags(self, host, port, dbname):
        service_check_tags = [
            "host:%s" % host,
            "port:%s" % port,
            "db:%s" % dbname
        ]
        return service_check_tags

    def get_connection(self, key, host, port, user, password, dbname, ssl, use_cached=True):
        "Get and memoize connections to instances"
        if key in self.dbs and use_cached:
            return self.dbs[key]

        elif host != "" and user != "":
            try:
                if host == 'localhost' and password == '':
                    # Use ident method
                    connection = pg.connect("user=%s dbname=%s" % (user, dbname))
                elif port != '':
                    connection = pg.connect(host=host, port=port, user=user,
                        password=password, database=dbname, ssl=ssl)
                else:
                    connection = pg.connect(host=host, user=user, password=password,
                        database=dbname, ssl=ssl)
            except Exception as e:
                message = u'Error establishing postgres connection: %s' % (str(e))
                service_check_tags = self._get_service_check_tags(host, port, dbname)
                self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                    tags=service_check_tags, message=message)
                raise
        else:
            if not host:
                raise CheckException("Please specify a Postgres host to connect to.")
            elif not user:
                raise CheckException("Please specify a user to connect to Postgres as.")

        self.dbs[key] = connection
        return connection

    def _get_custom_metrics(self, custom_metrics, key):
        # Pre-processed cached custom_metrics
        if key in self.custom_metrics:
            return self.custom_metrics[key]

        # Otherwise pre-process custom metrics and verify definition
        required_parameters = ("descriptors", "metrics", "query", "relation")

        for m in custom_metrics:
            for param in required_parameters:
                if param not in m:
                    raise CheckException("Missing {0} parameter in custom metric".format(param))

            self.log.debug("Metric: {0}".format(m))

            for ref, (_, mtype) in m['metrics'].iteritems():
                cap_mtype = mtype.upper()
                if cap_mtype not in ('RATE', 'GAUGE', 'MONOTONIC'):
                    raise CheckException("Collector method {0} is not known."
                        " Known methods are RATE, GAUGE, MONOTONIC".format(cap_mtype))

                m['metrics'][ref][1] = getattr(PostgreSql, cap_mtype)
                self.log.debug("Method: %s" % (str(mtype)))

        self.custom_metrics[key] = custom_metrics
        return custom_metrics

    def check(self, instance):
        host = instance.get('host', '')
        port = instance.get('port', '')
        user = instance.get('username', '')
        password = instance.get('password', '')
        tags = instance.get('tags', [])
        dbname = instance.get('dbname', None)
        relations = instance.get('relations', [])
        ssl = _is_affirmative(instance.get('ssl', False))

        if relations and not dbname:
            self.warning('"dbname" parameter must be set when using the "relations" parameter.')

        if dbname is None:
            dbname = 'postgres'

        key = (host, port, dbname)

        custom_metrics = self._get_custom_metrics(instance.get('custom_metrics', []), key)

        # Clean up tags in case there was a None entry in the instance
        # e.g. if the yaml contains tags: but no actual tags
        if tags is None:
            tags = []
        else:
            tags = list(set(tags))

        # preset tags to the database name
        tags.extend(["db:%s" % dbname])

        self.log.debug("Custom metrics: %s" % custom_metrics)

        # preset tags to the database name
        db = None

        # Collect metrics
        try:
            # Check version
            db = self.get_connection(key, host, port, user, password, dbname, ssl)
            version = self._get_version(key, db)
            self.log.debug("Running check against version %s" % version)
            self._collect_stats(key, db, tags, relations, custom_metrics)
        except ShouldRestartException:
            self.log.info("Resetting the connection")
            db = self.get_connection(key, host, port, user, password, dbname, ssl, use_cached=False)
            self._collect_stats(key, db, tags, relations, custom_metrics)

        if db is not None:
            service_check_tags = self._get_service_check_tags(host, port, dbname)
            message = u'Established connection to postgres://%s:%s/%s' % (host, port, dbname)
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
                tags=service_check_tags, message=message)
            try:
                # commit to close the current query transaction
                db.commit()
            except Exception, e:
                self.log.warning("Unable to commit: {0}".format(e))
