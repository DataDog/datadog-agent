"""Mesos Slave check

Collects metrics from mesos slave node.
"""
# stdlib
from hashlib import md5

# 3rd party
import requests

# project
from checks import AgentCheck, CheckException


class MesosSlave(AgentCheck):
    GAUGE = AgentCheck.gauge
    MONOTONIC_COUNT = AgentCheck.monotonic_count
    SERVICE_CHECK_NAME = "mesos_slave.can_connect"
    service_check_needed = True

    TASK_STATUS = {
        'TASK_STARTING'     : AgentCheck.OK,
        'TASK_RUNNING'      : AgentCheck.OK,
        'TASK_FINISHED'     : AgentCheck.OK,
        'TASK_FAILED'       : AgentCheck.CRITICAL,
        'TASK_KILLED'       : AgentCheck.WARNING,
        'TASK_LOST'         : AgentCheck.CRITICAL,
        'TASK_STAGING'      : AgentCheck.OK,
        'TASK_ERROR'        : AgentCheck.CRITICAL,
    }

    TASK_METRICS = {
        'cpus'                              : ('mesos.state.task.cpu', GAUGE),
        'mem'                               : ('mesos.state.task.mem', GAUGE),
        'disk'                              : ('mesos.state.task.disk', GAUGE),
    }

    SLAVE_TASKS_METRICS = {
        'slave/tasks_failed'                : ('mesos.slave.tasks_failed', MONOTONIC_COUNT),
        'slave/tasks_finished'              : ('mesos.slave.tasks_finished', MONOTONIC_COUNT),
        'slave/tasks_killed'                : ('mesos.slave.tasks_killed', MONOTONIC_COUNT),
        'slave/tasks_lost'                  : ('mesos.slave.tasks_lost', MONOTONIC_COUNT),
        'slave/tasks_running'               : ('mesos.slave.tasks_running', GAUGE),
        'slave/tasks_staging'               : ('mesos.slave.tasks_staging', GAUGE),
        'slave/tasks_starting'              : ('mesos.slave.tasks_starting', GAUGE),
    }

    SYSTEM_METRICS = {
        'system/cpus_total'                 : ('mesos.stats.system.cpus_total', GAUGE),
        'system/load_15min'                 : ('mesos.stats.system.load_15min', GAUGE),
        'system/load_1min'                  : ('mesos.stats.system.load_1min', GAUGE),
        'system/load_5min'                  : ('mesos.stats.system.load_5min', GAUGE),
        'system/mem_free_bytes'             : ('mesos.stats.system.mem_free_bytes', GAUGE),
        'system/mem_total_bytes'            : ('mesos.stats.system.mem_total_bytes', GAUGE),
        'slave/registered'                  : ('mesos.stats.registered', GAUGE),
        'slave/uptime_secs'                 : ('mesos.stats.uptime_secs', GAUGE),
    }

    SLAVE_RESOURCE_METRICS = {
        'slave/cpus_percent'                : ('mesos.slave.cpus_percent', GAUGE),
        'slave/cpus_total'                  : ('mesos.slave.cpus_total', GAUGE),
        'slave/cpus_used'                   : ('mesos.slave.cpus_used', GAUGE),
        'slave/disk_percent'                : ('mesos.slave.disk_percent', GAUGE),
        'slave/disk_total'                  : ('mesos.slave.disk_total', GAUGE),
        'slave/disk_used'                   : ('mesos.slave.disk_used', GAUGE),
        'slave/mem_percent'                 : ('mesos.slave.mem_percent', GAUGE),
        'slave/mem_total'                   : ('mesos.slave.mem_total', GAUGE),
        'slave/mem_used'                    : ('mesos.slave.mem_used', GAUGE),
    }

    SLAVE_EXECUTORS_METRICS = {
        'slave/executors_registering'       : ('mesos.slave.executors_registering', GAUGE),
        'slave/executors_running'           : ('mesos.slave.executors_running', GAUGE),
        'slave/executors_terminated'        : ('mesos.slave.executors_terminated', GAUGE),
        'slave/executors_terminating'       : ('mesos.slave.executors_terminating', GAUGE),
    }

    STATS_METRICS = {
        'slave/frameworks_active'           : ('mesos.slave.frameworks_active', GAUGE),
        'slave/invalid_framework_messages'  : ('mesos.slave.invalid_framework_messages', GAUGE),
        'slave/invalid_status_updates'      : ('mesos.slave.invalid_status_updates', GAUGE),
        'slave/recovery_errors'             : ('mesos.slave.recovery_errors', GAUGE),
        'slave/valid_framework_messages'    : ('mesos.slave.valid_framework_messages', GAUGE),
        'slave/valid_status_updates'        : ('mesos.slave.valid_status_updates', GAUGE),
    }

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.cluster_name = None

    def _get_json(self, url, timeout):
        # Use a hash of the URL as an aggregation key
        aggregation_key = md5(url).hexdigest()
        tags = ["url:%s" % url]
        msg = None
        status = None
        try:
            r = requests.get(url, timeout=timeout)
            if r.status_code != 200:
                status = AgentCheck.CRITICAL
                msg = "Got %s when hitting %s" % (r.status_code, url)
            else:
                status = AgentCheck.OK
                msg = "Mesos master instance detected at %s " % url
        except requests.exceptions.Timeout as e:
            # If there's a timeout
            msg = "%s seconds timeout when hitting %s" % (timeout, url)
            status = AgentCheck.CRITICAL
        except Exception as e:
            msg = str(e)
            status = AgentCheck.CRITICAL
        finally:
            if self.service_check_needed:
                self.service_check(self.SERVICE_CHECK_NAME, status, tags=tags, message=msg)
                self.service_check_needed = False
            if status is AgentCheck.CRITICAL:
                raise CheckException("Cannot connect to mesos, please check your configuration.")

        return r.json()

    def _get_state(self, url, timeout):
        return self._get_json(url + '/state.json', timeout)

    def _get_stats(self, url, timeout):
        if self.version >= [0, 22, 0]:
            endpoint = '/metrics/snapshot'
        else:
            endpoint = '/stats.json'
        return self._get_json(url + endpoint, timeout)

    def _get_constant_attributes(self, url, timeout):
        state_metrics = None
        if self.cluster_name is None:
            state_metrics = self._get_state(url, timeout)
            if state_metrics is not None:
                self.version = map(int, state_metrics['version'].split('.'))
                master_state = self._get_state('http://' + state_metrics['master_hostname'] + ':5050', timeout)
                if master_state is not None:
                    self.cluster_name = master_state.get('cluster')

        return state_metrics

    def check(self, instance):
        if 'url' not in instance:
            raise Exception('Mesos instance missing "url" value.')

        url = instance['url']
        instance_tags = instance.get('tags', [])
        tasks = instance.get('tasks', [])
        default_timeout = self.init_config.get('default_timeout', 5)
        timeout = float(instance.get('timeout', default_timeout))

        state_metrics = self._get_constant_attributes(url, timeout)
        tags = None

        if state_metrics is None:
            state_metrics = self._get_state(url, timeout)
        if state_metrics:
            tags = [
                'mesos_pid:{0}'.format(state_metrics['pid']),
                'mesos_node:slave',
            ]
            if self.cluster_name:
                tags.append('mesos_cluster:{0}'.format(self.cluster_name))

            tags += instance_tags

            for task in tasks:
                for framework in state_metrics['frameworks']:
                    for executor in framework['executors']:
                        for t in executor['tasks']:
                            if task.lower() in t['name'].lower() and t['slave_id'] == state_metrics['id']:
                                task_tags = ['task_name:' + t['name']] + tags
                                self.service_check(t['name'] + '.ok', self.TASK_STATUS[t['state']], tags=task_tags)
                                for key_name, (metric_name, metric_func) in self.TASK_METRICS.iteritems():
                                    metric_func(self, metric_name, t['resources'][key_name], tags=task_tags)

        stats_metrics = self._get_stats(url, timeout)
        if stats_metrics:
            tags = tags if tags else instance_tags
            metrics = [self.SLAVE_TASKS_METRICS, self.SYSTEM_METRICS, self.SLAVE_RESOURCE_METRICS,
                      self.SLAVE_EXECUTORS_METRICS, self.STATS_METRICS]
            for m in metrics:
                for key_name, (metric_name, metric_func) in m.iteritems():
                    metric_func(self, metric_name, stats_metrics[key_name], tags=tags)

        self.service_check_needed = True
