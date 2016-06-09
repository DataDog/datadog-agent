# stdlib
import os
import sys
import re
import traceback

# 3p
import pymysql

# project
from checks import AgentCheck
from utils.platform import Platform
from utils.subprocess_output import get_subprocess_output

GAUGE = "gauge"
RATE = "rate"

STATUS_VARS = {
    'Connections': ('mysql.net.connections', RATE),
    'Max_used_connections': ('mysql.net.max_connections', GAUGE),
    'Open_files': ('mysql.performance.open_files', GAUGE),
    'Table_locks_waited': ('mysql.performance.table_locks_waited', GAUGE),
    'Threads_connected': ('mysql.performance.threads_connected', GAUGE),
    'Threads_running': ('mysql.performance.threads_running', GAUGE),
    'Innodb_data_reads': ('mysql.innodb.data_reads', RATE),
    'Innodb_data_writes': ('mysql.innodb.data_writes', RATE),
    'Innodb_os_log_fsyncs': ('mysql.innodb.os_log_fsyncs', RATE),
    'Slow_queries': ('mysql.performance.slow_queries', RATE),
    'Questions': ('mysql.performance.questions', RATE),
    'Queries': ('mysql.performance.queries', RATE),
    'Com_select': ('mysql.performance.com_select', RATE),
    'Com_insert': ('mysql.performance.com_insert', RATE),
    'Com_update': ('mysql.performance.com_update', RATE),
    'Com_delete': ('mysql.performance.com_delete', RATE),
    'Com_insert_select': ('mysql.performance.com_insert_select', RATE),
    'Com_update_multi': ('mysql.performance.com_update_multi', RATE),
    'Com_delete_multi': ('mysql.performance.com_delete_multi', RATE),
    'Com_replace_select': ('mysql.performance.com_replace_select', RATE),
    'Qcache_hits': ('mysql.performance.qcache_hits', RATE),
    'Innodb_mutex_spin_waits': ('mysql.innodb.mutex_spin_waits', RATE),
    'Innodb_mutex_spin_rounds': ('mysql.innodb.mutex_spin_rounds', RATE),
    'Innodb_mutex_os_waits': ('mysql.innodb.mutex_os_waits', RATE),
    'Created_tmp_tables': ('mysql.performance.created_tmp_tables', RATE),
    'Created_tmp_disk_tables': ('mysql.performance.created_tmp_disk_tables', RATE),
    'Created_tmp_files': ('mysql.performance.created_tmp_files', RATE),
    'Innodb_row_lock_waits': ('mysql.innodb.row_lock_waits', RATE),
    'Innodb_row_lock_time': ('mysql.innodb.row_lock_time', RATE),
    'Innodb_current_row_locks': ('mysql.innodb.current_row_locks', GAUGE),
    'Open_tables': ('mysql.performance.open_tables', GAUGE),
}


class MySql(AgentCheck):
    SERVICE_CHECK_NAME = 'mysql.can_connect'
    MAX_CUSTOM_QUERIES = 20
    DEFAULT_TIMEOUT = 5

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.mysql_version = {}
        self.greater_502 = {}

    def get_library_versions(self):
        return {"pymysql": pymysql.__version__}

    def check(self, instance):
        host, port, user, password, mysql_sock, defaults_file, tags, options, queries = \
            self._get_config(instance)

        default_timeout = self.init_config.get('default_timeout', self.DEFAULT_TIMEOUT)

        if (not host or not user) and not defaults_file:
            raise Exception("Mysql host and user are needed.")

        db = self._connect(host, port, mysql_sock, user, password, defaults_file)

        # Metadata collection
        self._collect_metadata(db, host)

        # Metric collection
        self._collect_metrics(host, db, tags, options, queries)
        if Platform.is_linux():
            self._collect_system_metrics(host, db, tags)

        # Close connection
        db.close()

    def _get_config(self, instance):
        host = instance.get('server', '')
        user = instance.get('user', '')
        port = int(instance.get('port', 0))
        password = instance.get('pass', '')
        mysql_sock = instance.get('sock', '')
        defaults_file = instance.get('defaults_file', '')
        tags = instance.get('tags', None)
        options = instance.get('options', {})
        queries = instance.get('queries', [])

        return host, port, user, password, mysql_sock, defaults_file, tags, options, queries

    def _connect(self, host, port, mysql_sock, user, password, defaults_file):
        service_check_tags = [
            'host:%s' % host,
            'port:%s' % port
        ]

        try:
            if defaults_file != '':
                db = pymysql.connect(read_default_file=defaults_file)
            elif mysql_sock != '':
                db = pymysql.connect(
                    unix_socket=mysql_sock,
                    user=user,
                    passwd=password
                )
                service_check_tags = [
                    'host:%s' % mysql_sock,
                    'port:unix_socket'
                ]
            elif port:
                db = pymysql.connect(
                    host=host,
                    port=port,
                    user=user,
                    passwd=password
                )
            else:
                db = pymysql.connect(
                    host=host,
                    user=user,
                    passwd=password
                )
            self.log.debug("Connected to MySQL")
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
                               tags=service_check_tags)
        except Exception:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                               tags=service_check_tags)
            raise

        return db

    def _collect_metrics(self, host, db, tags, options, queries):
        cursor = db.cursor()
        cursor.execute("SHOW /*!50002 GLOBAL */ STATUS;")
        status_results = dict(cursor.fetchall())
        self._rate_or_gauge_statuses(STATUS_VARS, status_results, tags)
        cursor.execute("SHOW VARIABLES LIKE 'Key%';")
        variables_results = dict(cursor.fetchall())
        cursor.close()
        del cursor

        # Compute key cache utilization metric
        key_blocks_unused = self._collect_scalar('Key_blocks_unused', status_results)
        key_cache_block_size = self._collect_scalar('key_cache_block_size', variables_results)
        key_buffer_size = self._collect_scalar('key_buffer_size', variables_results)
        key_cache_utilization = 1 - ((key_blocks_unused * key_cache_block_size) / key_buffer_size)
        self.gauge("mysql.performance.key_cache_utilization", key_cache_utilization, tags=tags)

        # Compute InnoDB buffer metrics
        # Be sure InnoDB is enabled
        if 'Innodb_page_size' in status_results:
            page_size = self._collect_scalar('Innodb_page_size', status_results)
            innodb_buffer_pool_pages_total = self._collect_scalar('Innodb_buffer_pool_pages_total',
                                                                  status_results)
            innodb_buffer_pool_pages_free = self._collect_scalar('Innodb_buffer_pool_pages_free',
                                                                 status_results)
            innodb_buffer_pool_pages_total = innodb_buffer_pool_pages_total * page_size
            innodb_buffer_pool_pages_free = innodb_buffer_pool_pages_free * page_size
            innodb_buffer_pool_pages_used = \
                innodb_buffer_pool_pages_total - innodb_buffer_pool_pages_free

            self.gauge("mysql.innodb.buffer_pool_free", innodb_buffer_pool_pages_free, tags=tags)
            self.gauge("mysql.innodb.buffer_pool_used", innodb_buffer_pool_pages_used, tags=tags)
            self.gauge("mysql.innodb.buffer_pool_total", innodb_buffer_pool_pages_total, tags=tags)

            if innodb_buffer_pool_pages_total != 0:
                innodb_buffer_pool_pages_utilization = \
                    innodb_buffer_pool_pages_used / innodb_buffer_pool_pages_total
                self.gauge("mysql.innodb.buffer_pool_utilization",
                           innodb_buffer_pool_pages_utilization, tags=tags)

        if 'galera_cluster' in options and options['galera_cluster']:
            value = self._collect_scalar('wsrep_cluster_size', status_results)
            self.gauge('mysql.galera.wsrep_cluster_size', value, tags=tags)

        if 'replication' in options and options['replication']:
            # get slave running form global status page
            slave_running = self._collect_string('Slave_running', status_results)
            if slave_running is not None:
                if slave_running.lower().strip() == 'on':
                    slave_running = 1
                else:
                    slave_running = 0
                self.gauge("mysql.replication.slave_running", slave_running, tags=tags)
            self._collect_dict(
                GAUGE,
                {"Seconds_behind_master": "mysql.replication.seconds_behind_master"},
                "SHOW SLAVE STATUS", db, tags=tags
            )

        # Collect custom query metrics
        # Max of 20 queries allowed
        if isinstance(queries, list):
            for index, check in enumerate(queries[:self.MAX_CUSTOM_QUERIES]):
                self._collect_dict(check['type'], {check['field']: check['metric']}, check['query'], db, tags=tags)

            if len(queries) > self.MAX_CUSTOM_QUERIES:
                self.warning("Maximum number (%s) of custom queries reached.  Skipping the rest."
                             % self.MAX_CUSTOM_QUERIES)


    def _collect_metadata(self, db, host):
        self._get_version(db, host)

    def _rate_or_gauge_statuses(self, statuses, dbResults, tags):
        for status, metric in statuses.iteritems():
            metric_name, metric_type = metric
            value = self._collect_scalar(status, dbResults)
            if value is not None:
                if metric_type == RATE:
                    self.rate(metric_name, value, tags=tags)
                elif metric_type == GAUGE:
                    self.gauge(metric_name, value, tags=tags)

    def _version_greater_502(self, db, host):
        # show global status was introduced in 5.0.2
        # some patch version numbers contain letters (e.g. 5.0.51a)
        # so let's be careful when we compute the version number
        if host in self.greater_502:
            return self.greater_502[host]

        greater_502 = False
        try:
            mysql_version = self._get_version(db, host)
            self.log.debug("MySQL version %s" % mysql_version)

            major = int(mysql_version[0])
            minor = int(mysql_version[1])
            patchlevel = int(re.match(r"([0-9]+)", mysql_version[2]).group(1))

            if (major, minor, patchlevel) > (5, 0, 2):
                greater_502 = True

        except Exception, exception:
            self.warning("Cannot compute mysql version, assuming older than 5.0.2: %s"
                         % str(exception))

        self.greater_502[host] = greater_502

        return greater_502

    def _get_version(self, db, host):
        if host in self.mysql_version:
            version = self.mysql_version[host]
            self.service_metadata('version', ".".join(version))
            return version

        # Get MySQL version
        cursor = db.cursor()
        cursor.execute('SELECT VERSION()')
        result = cursor.fetchone()
        cursor.close()
        del cursor
        # Version might include a description e.g. 4.1.26-log.
        # See http://dev.mysql.com/doc/refman/4.1/en/information-functions.html#function_version
        version = result[0].split('-')
        version = version[0].split('.')
        self.mysql_version[host] = version
        self.service_metadata('version', ".".join(version))
        return version

    def _collect_scalar(self, key, dict):
        return self._collect_type(key, dict, float)

    def _collect_string(self, key, dict):
        return self._collect_type(key, dict, unicode)

    def _collect_type(self, key, dict, the_type):
        self.log.debug("Collecting data with %s" % key)
        if key not in dict:
            self.log.debug("%s returned None" % key)
            return None
        self.log.debug("Collecting done, value %s" % dict[key])
        return the_type(dict[key])

    def _collect_dict(self, metric_type, field_metric_map, query, db, tags):
        """
        Query status and get a dictionary back.
        Extract each field out of the dictionary
        and stuff it in the corresponding metric.

        query: show status...
        field_metric_map: {"Seconds_behind_master": "mysqlSecondsBehindMaster"}
        """
        try:
            cursor = db.cursor()
            cursor.execute(query)
            result = cursor.fetchone()
            if result is not None:
                for field in field_metric_map.keys():
                    # Get the agent metric name from the column name
                    metric = field_metric_map[field]
                    # Find the column name in the cursor description to identify the column index
                    # http://www.python.org/dev/peps/pep-0249/
                    # cursor.description is a tuple of (column_name, ..., ...)
                    try:
                        col_idx = [d[0].lower() for d in cursor.description].index(field.lower())
                        self.log.debug("Collecting metric: %s" % metric)
                        if result[col_idx] is not None:
                            self.log.debug("Collecting done, value %s" % result[col_idx])
                            if metric_type == GAUGE:
                                self.gauge(metric, float(result[col_idx]), tags=tags)
                            elif metric_type == RATE:
                                self.rate(metric, float(result[col_idx]), tags=tags)
                            else:
                                self.gauge(metric, float(result[col_idx]), tags=tags)
                        else:
                            self.log.debug("Received value is None for index %d" % col_idx)
                    except ValueError:
                        self.log.exception("Cannot find %s in the columns %s"
                                           % (field, cursor.description))
            cursor.close()
            del cursor
        except Exception:
            self.warning("Error while running %s\n%s" % (query, traceback.format_exc()))
            self.log.exception("Error while running %s" % query)

    def _collect_system_metrics(self, host, db, tags):
        pid = None
        # The server needs to run locally, accessed by TCP or socket
        if host in ["localhost", "127.0.0.1"] or db.port == long(0):
            pid = self._get_server_pid(db)

        if pid:
            self.log.debug("pid: %s" % pid)
            # At last, get mysql cpu data out of procfs
            try:
                # See http://www.kernel.org/doc/man-pages/online/pages/man5/proc.5.html
                # for meaning: we get 13 & 14: utime and stime, in clock ticks and convert
                # them with the right sysconf value (SC_CLK_TCK)
                proc_file = open("/proc/%d/stat" % pid)
                data = proc_file.readline()
                proc_file.close()
                fields = data.split(' ')
                ucpu = fields[13]
                kcpu = fields[14]
                clk_tck = os.sysconf(os.sysconf_names["SC_CLK_TCK"])

                # Convert time to s (number of second of CPU used by mysql)
                # It's a counter, it will be divided by the period, multiply by 100
                # to get the percentage of CPU used by mysql over the period
                self.rate("mysql.performance.user_time",
                          int((float(ucpu) / float(clk_tck)) * 100), tags=tags)
                self.rate("mysql.performance.kernel_time",
                          int((float(kcpu) / float(clk_tck)) * 100), tags=tags)
            except Exception:
                self.warning("Error while reading mysql (pid: %s) procfs data\n%s"
                             % (pid, traceback.format_exc()))

    def _get_server_pid(self, db):
        pid = None

        # Try to get pid from pid file, it can fail for permission reason
        pid_file = None
        try:
            cursor = db.cursor()
            cursor.execute("SHOW VARIABLES LIKE 'pid_file'")
            pid_file = cursor.fetchone()[1]
            cursor.close()
            del cursor
        except Exception:
            self.warning("Error while fetching pid_file variable of MySQL.")

        if pid_file is not None:
            self.log.debug("pid file: %s" % str(pid_file))
            try:
                f = open(pid_file)
                pid = int(f.readline())
                f.close()
            except IOError:
                self.log.debug("Cannot read mysql pid file %s" % pid_file)

        # If pid has not been found, read it from ps
        if pid is None:
            try:
                if sys.platform.startswith("linux"):
                    ps, _, _ = get_subprocess_output(['ps', '-C', 'mysqld', '-o', 'pid'], self.log)
                    pslines = ps.strip().splitlines()
                    # First line is header, second line is mysql pid
                    if len(pslines) == 2:
                        pid = int(pslines[1])
            except Exception:
                self.log.exception("Error while fetching mysql pid from ps")

        return pid
