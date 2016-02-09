# stdlib
from copy import deepcopy
from datetime import datetime, timedelta
from hashlib import md5
from Queue import Empty, Queue
import re
import time
import traceback

# 3p
from pyVim import connect
from pyVmomi import vim

# project
from checks import AgentCheck
from checks.libs.thread_pool import Pool
from checks.libs.vmware.basic_metrics import BASIC_METRICS
from util import Timer

SOURCE_TYPE = 'vsphere'
REAL_TIME_INTERVAL = 20  # Default vCenter sampling interval

# The size of the ThreadPool used to process the request queue
DEFAULT_SIZE_POOL = 4
# The interval in seconds between two refresh of the entities list
REFRESH_MORLIST_INTERVAL = 3 * 60
# The interval in seconds between two refresh of metrics metadata (id<->name)
REFRESH_METRICS_METADATA_INTERVAL = 10 * 60
# The amount of jobs batched at the same time in the queue to query available metrics
BATCH_MORLIST_SIZE = 50

# Time after which we reap the jobs that clog the queue
# TODO: use it
JOB_TIMEOUT = 10

EXCLUDE_FILTERS = {
    'AlarmStatusChangedEvent': [r'Gray'],
    'TaskEvent': [
        r'Initialize powering On',
        r'Power Off virtual machine',
        r'Power On virtual machine',
        r'Reconfigure virtual machine',
        r'Relocate virtual machine',
        r'Suspend virtual machine',
        r'Migrate virtual machine',
    ],
    'VmBeingHotMigratedEvent': [],
    'VmMessageEvent': [],
    'VmMigratedEvent': [],
    'VmPoweredOnEvent': [],
    'VmPoweredOffEvent': [],
    'VmReconfiguredEvent': [],
    'VmResumedEvent': [],
    'VmSuspendedEvent': [],
}

MORLIST = 'morlist'
METRICS_METADATA = 'metrics_metadata'
LAST = 'last'
INTERVAL = 'interval'


class VSphereEvent(object):
    UNKNOWN = 'unknown'

    def __init__(self, raw_event, event_config=None):
        self.raw_event = raw_event
        if self.raw_event and self.raw_event.__class__.__name__.startswith('vim.event'):
            self.event_type = self.raw_event.__class__.__name__[10:]
        else:
            self.event_type = VSphereEvent.UNKNOWN

        self.timestamp = int((self.raw_event.createdTime.replace(tzinfo=None) - datetime(1970, 1, 1)).total_seconds())
        self.payload = {
            "timestamp": self.timestamp,
            "event_type": SOURCE_TYPE,
            "source_type_name": SOURCE_TYPE,
        }
        if event_config is None:
            self.event_config = {}
        else:
            self.event_config = event_config

    def _is_filtered(self):
        # Filter the unwanted types
        if self.event_type not in EXCLUDE_FILTERS:
            return True

        filters = EXCLUDE_FILTERS[self.event_type]
        for f in filters:
            if re.search(f, self.raw_event.fullFormattedMessage):
                return True

        return False

    def get_datadog_payload(self):
        if self._is_filtered():
            return None

        transform_method = getattr(self, 'transform_%s' % self.event_type.lower(), None)
        if callable(transform_method):
            return transform_method()

        # Default event transformation
        self.payload["msg_title"] = u"{0}".format(self.event_type)
        self.payload["msg_text"] = u"@@@\n{0}\n@@@".format(self.raw_event.fullFormattedMessage)

        return self.payload

    def transform_vmbeinghotmigratedevent(self):
        self.payload["msg_title"] = u"VM {0} is being migrated".format(self.raw_event.vm.name)
        self.payload["msg_text"] = u"{user} has launched a hot migration of this virtual machine:\n".format(user=self.raw_event.userName)
        changes = []
        pre_host = self.raw_event.host.name
        new_host = self.raw_event.destHost.name
        pre_dc = self.raw_event.datacenter.name
        new_dc = self.raw_event.destDatacenter.name
        pre_ds = self.raw_event.ds.name
        new_ds = self.raw_event.destDatastore.name
        if pre_host == new_host:
            changes.append(u"- No host migration: still {0}".format(new_host))
        else:
            # Insert in front if it's a change
            changes = [u"- Host MIGRATION: from {0} to {1}".format(pre_host, new_host)] + changes
        if pre_dc == new_dc:
            changes.append(u"- No datacenter migration: still {0}".format(new_dc))
        else:
            # Insert in front if it's a change
            changes = [u"- Datacenter MIGRATION: from {0} to {1}".format(pre_dc, new_dc)] + changes
        if pre_ds == new_ds:
            changes.append(u"- No datastore migration: still {0}".format(new_ds))
        else:
            # Insert in front if it's a change
            changes = [u"- Datastore MIGRATION: from {0} to {1}".format(pre_ds, new_ds)] + changes

        self.payload["msg_text"] += "\n".join(changes)

        self.payload['host'] = self.raw_event.vm.name
        self.payload['tags'] = [
            'vsphere_host:%s' % pre_host,
            'vsphere_host:%s' % new_host,
            'vsphere_datacenter:%s' % pre_dc,
            'vsphere_datacenter:%s' % new_dc,
        ]
        return self.payload

    def transform_alarmstatuschangedevent(self):
        if self.event_config.get('collect_vcenter_alarms') is None:
            return None

        def get_transition(before, after):
            vals = {
                'gray': -1,
                'green': 0,
                'yellow': 1,
                'red': 2
            }
            before = before.lower()
            after = after.lower()
            if before not in vals or after not in vals:
                return None
            if vals[before] < vals[after]:
                return 'Triggered'
            else:
                return 'Recovered'

        TO_ALERT_TYPE = {
            'green': 'success',
            'yellow': 'warning',
            'red': 'error'
        }

        def get_agg_key(alarm_event):
            return 'h:{0}|dc:{1}|a:{2}'.format(
                md5(alarm_event.entity.name).hexdigest()[:10],
                md5(alarm_event.datacenter.name).hexdigest()[:10],
                md5(alarm_event.alarm.name).hexdigest()[:10]
            )

        # Get the entity type/name
        if self.raw_event.entity.entity.__class__ == vim.VirtualMachine:
            host_type = 'VM'
        elif self.raw_event.entity.entity.__class__ == vim.HostSystem:
            host_type = 'host'
        else:
            return None
        host_name = self.raw_event.entity.name

        # Need a getattr because from is a reserved keyword...
        trans_before = getattr(self.raw_event, 'from')
        trans_after = self.raw_event.to
        transition = get_transition(trans_before, trans_after)
        # Bad transition, we shouldn't have got this transition
        if transition is None:
            return None

        self.payload['msg_title'] = u"[{transition}] {monitor} on {host_type} {host_name} is now {status}".format(
            transition=transition,
            monitor=self.raw_event.alarm.name,
            host_type=host_type,
            host_name=host_name,
            status=trans_after
        )
        self.payload['alert_type'] = TO_ALERT_TYPE[trans_after]
        self.payload['event_object'] = get_agg_key(self.raw_event)
        self.payload['msg_text'] = u"""vCenter monitor status changed on this alarm, it was {before} and it's now {after}.""".format(
            before=trans_before,
            after=trans_after
        )
        self.payload['host'] = host_name
        return self.payload

    def transform_vmmessageevent(self):
        self.payload["msg_title"] = u"VM {0} is reporting".format(self.raw_event.vm.name)
        self.payload["msg_text"] = u"@@@\n{0}\n@@@".format(self.raw_event.fullFormattedMessage)
        self.payload['host'] = self.raw_event.vm.name
        return self.payload

    def transform_vmmigratedevent(self):
        self.payload["msg_title"] = u"VM {0} has been migrated".format(self.raw_event.vm.name)
        self.payload["msg_text"] = u"@@@\n{0}\n@@@".format(self.raw_event.fullFormattedMessage)
        self.payload['host'] = self.raw_event.vm.name
        return self.payload

    def transform_vmpoweredoffevent(self):
        self.payload["msg_title"] = u"VM {0} has been powered OFF".format(self.raw_event.vm.name)
        self.payload["msg_text"] = u"""{user} has powered off this virtual machine. It was running on:
- datacenter: {dc}
- host: {host}
""".format(
            user=self.raw_event.userName,
            dc=self.raw_event.datacenter.name,
            host=self.raw_event.host.name
        )
        self.payload['host'] = self.raw_event.vm.name
        return self.payload

    def transform_vmpoweredonevent(self):
        self.payload["msg_title"] = u"VM {0} has been powered ON".format(self.raw_event.vm.name)
        self.payload["msg_text"] = u"""{user} has powered on this virtual machine. It is running on:
- datacenter: {dc}
- host: {host}
""".format(
            user=self.raw_event.userName,
            dc=self.raw_event.datacenter.name,
            host=self.raw_event.host.name
        )
        self.payload['host'] = self.raw_event.vm.name
        return self.payload

    def transform_vmresumingevent(self):
        self.payload["msg_title"] = u"VM {0} is RESUMING".format(self.raw_event.vm.name)
        self.payload["msg_text"] = u"""{user} has resumed {vm}. It will soon be powered on.""".format(
            user=self.raw_event.userName,
            vm=self.raw_event.vm.name
        )
        self.payload['host'] = self.raw_event.vm.name
        return self.payload

    def transform_vmsuspendedevent(self):
        self.payload["msg_title"] = u"VM {0} has been SUSPENDED".format(self.raw_event.vm.name)
        self.payload["msg_text"] = u"""{user} has suspended this virtual machine. It was running on:
- datacenter: {dc}
- host: {host}
""".format(
            user=self.raw_event.userName,
            dc=self.raw_event.datacenter.name,
            host=self.raw_event.host.name
        )
        self.payload['host'] = self.raw_event.vm.name
        return self.payload

    def transform_vmreconfiguredevent(self):
        self.payload["msg_title"] = u"VM {0} configuration has been changed".format(self.raw_event.vm.name)
        self.payload["msg_text"] = u"{user} saved the new configuration:\n@@@\n".format(user=self.raw_event.userName)
        # Add lines for configuration change don't show unset, that's hacky...
        config_change_lines = [line for line in self.raw_event.configSpec.__repr__().splitlines() if 'unset' not in line]
        self.payload["msg_text"] += u"\n".join(config_change_lines)
        self.payload["msg_text"] += u"\n@@@"
        self.payload['host'] = self.raw_event.vm.name
        return self.payload

def atomic_method(method):
    """ Decorator to catch the exceptions that happen in detached thread atomic tasks
    and display them in the logs.
    """
    def wrapper(*args, **kwargs):
        try:
            method(*args, **kwargs)
        except Exception as e:
            args[0].exceptionq.put("A worker thread crashed:\n" + traceback.format_exc())
    return wrapper

class VSphereCheck(AgentCheck):
    """ Get performance metrics from a vCenter server and upload them to Datadog
    References:
        http://pubs.vmware.com/vsphere-51/index.jsp#com.vmware.wssdk.apiref.doc/vim.PerformanceManager.html

    *_atomic jobs perform one single task asynchronously in the ThreadPool, we
    don't know exactly when they will finish, but we reap them if they're stuck.
    The other calls are performed synchronously.
    """

    SERVICE_CHECK_NAME = 'vcenter.can_connect'

    def __init__(self, name, init_config, agentConfig, instances):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.time_started = time.time()
        self.pool_started = False
        self.exceptionq = Queue()

        # Connections open to vCenter instances
        self.server_instances = {}

        # Event configuration
        self.event_config = {}
        # Caching resources, timeouts
        self.cache_times = {}
        for instance in self.instances:
            i_key = self._instance_key(instance)
            self.cache_times[i_key] = {
                MORLIST: {
                    LAST: 0,
                    INTERVAL: init_config.get('refresh_morlist_interval',
                                    REFRESH_MORLIST_INTERVAL)
                },
                METRICS_METADATA: {
                    LAST: 0,
                    INTERVAL: init_config.get('refresh_metrics_metadata_interval',
                                    REFRESH_METRICS_METADATA_INTERVAL)
                }
            }

            self.event_config[i_key] = instance.get('event_config')

        # First layer of cache (get entities from the tree)
        self.morlist_raw = {}
        # Second layer, processed from the first one
        self.morlist = {}
        # Metrics metadata, basically perfCounterId -> {name, group, description}
        self.metrics_metadata = {}

        self.latest_event_query = {}

    def stop(self):
        self.stop_pool()

    def start_pool(self):
        self.log.info("Starting Thread Pool")
        self.pool_size = int(self.init_config.get('threads_count', DEFAULT_SIZE_POOL))

        self.pool = Pool(self.pool_size)
        self.pool_started = True
        self.jobs_status = {}

    def stop_pool(self):
        self.log.info("Stopping Thread Pool")
        if self.pool_started:
            self.pool.terminate()
            self.pool.join()
            self.jobs_status.clear()
            assert self.pool.get_nworkers() == 0
            self.pool_started = False

    def restart_pool(self):
        self.stop_pool()
        self.start_pool()

    def _clean(self):
        now = time.time()
        # TODO: use that
        for name in self.jobs_status.keys():
            start_time = self.jobs_status[name]
            if now - start_time > JOB_TIMEOUT:
                self.log.critical("Restarting Pool. One check is stuck.")
                self.restart_pool()
                break

    def _query_event(self, instance):
        i_key = self._instance_key(instance)
        last_time = self.latest_event_query.get(i_key)

        server_instance = self._get_server_instance(instance)
        event_manager = server_instance.content.eventManager

        # Be sure we don't duplicate any event, never query the "past"
        if not last_time:
            last_time = self.latest_event_query[i_key] = \
                event_manager.latestEvent.createdTime + timedelta(seconds=1)

        query_filter = vim.event.EventFilterSpec()
        time_filter = vim.event.EventFilterSpec.ByTime(beginTime=self.latest_event_query[i_key])
        query_filter.time = time_filter

        try:
            new_events = event_manager.QueryEvents(query_filter)
            self.log.debug("Got {0} events from vCenter event manager".format(len(new_events)))
            for event in new_events:
                normalized_event = VSphereEvent(event, self.event_config[i_key])
                # Can return None if the event if filtered out
                event_payload = normalized_event.get_datadog_payload()
                if event_payload is not None:
                    self.event(event_payload)
                last_time = event.createdTime + timedelta(seconds=1)
        except Exception as e:
            # Don't get stuck on a failure to fetch an event
            # Ignore them for next pass
            self.log.warning("Unable to fetch Events %s", e)
            last_time = event_manager.latestEvent.createdTime + timedelta(seconds=1)

        self.latest_event_query[i_key] = last_time

    def _instance_key(self, instance):
        i_key = instance.get('name')
        if i_key is None:
            raise Exception("Must define a unique 'name' per vCenter instance")
        return i_key

    def _should_cache(self, instance, entity):
        i_key = self._instance_key(instance)
        now = time.time()
        return now - self.cache_times[i_key][entity][LAST] > self.cache_times[i_key][entity][INTERVAL]

    def _get_server_instance(self, instance):
        i_key = self._instance_key(instance)

        service_check_tags = [
            'vcenter_server:{0}'.format(instance.get('name')),
            'vcenter_host:{0}'.format(instance.get('host')),
        ]

        if i_key not in self.server_instances:
            try:
                server_instance = connect.SmartConnect(
                    host=instance.get('host'),
                    user=instance.get('username'),
                    pwd=instance.get('password')
                )
            except Exception as e:
                err_msg = "Connection to %s failed: %s" % (instance.get('host'), e)
                self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                        tags=service_check_tags, message=err_msg)
                raise Exception(err_msg)

            self.server_instances[i_key] = server_instance

        # Test if the connection is working
        try:
            self.server_instances[i_key].RetrieveContent()
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
                    tags=service_check_tags)
        except Exception as e:
            err_msg = "Connection to %s died unexpectedly: %s" % (instance.get('host'), e)
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                    tags=service_check_tags, message=err_msg)
            raise Exception(err_msg)

        return self.server_instances[i_key]

    def _compute_needed_metrics(self, instance, available_metrics):
        """ Compare the available metrics for one MOR we have computed and intersect them
        with the set of metrics we want to report
        """
        if instance.get('all_metrics', False):
            return available_metrics

        i_key = self._instance_key(instance)
        wanted_metrics = []
        # Get only the basic metrics
        for metric in available_metrics:
            # No cache yet, skip it for now
            if (i_key not in self.metrics_metadata
                    or metric.counterId not in self.metrics_metadata[i_key]):
                continue
            if self.metrics_metadata[i_key][metric.counterId]['name'] in BASIC_METRICS:
                wanted_metrics.append(metric)

        return wanted_metrics


    def get_external_host_tags(self):
        """ Returns a list of tags for every host that is detected by the vSphere
        integration.
        List of pairs (hostname, list_of_tags)
        """
        self.log.info("Sending external_host_tags now")
        external_host_tags = []
        for instance in self.instances:
            i_key = self._instance_key(instance)
            mor_list = self.morlist[i_key].items()
            for mor_name, mor in mor_list:
                external_host_tags.append((mor['hostname'], {SOURCE_TYPE: mor['tags']}))

        return external_host_tags

    @atomic_method
    def _cache_morlist_raw_atomic(self, i_key, obj_type, obj, tags, regexes=None):
        """ Compute tags for a single node in the vCenter rootFolder
        and queue other such jobs for children nodes.
        Usual hierarchy:
        rootFolder
            - datacenter1
                - compute_resource1 == cluster
                    - host1
                    - host2
                    - host3
                - compute_resource2
                    - host5
                        - vm1
                        - vm2
        If it's a node we want to query metric for, queue it in self.morlist_raw
        that will be processed by another job.
        """
        ### <TEST-INSTRUMENTATION>
        t = Timer()
        self.log.debug("job_atomic: Exploring MOR {0} (type={1})".format(obj, obj_type))
        ### </TEST-INSTRUMENTATION>
        tags_copy = deepcopy(tags)

        if obj_type == 'rootFolder':
            for datacenter in obj.childEntity:
                # Skip non-datacenter
                if not hasattr(datacenter, 'hostFolder'):
                    continue
                self.pool.apply_async(
                    self._cache_morlist_raw_atomic,
                    args=(i_key, 'datacenter', datacenter, tags_copy, regexes)
                )

        elif obj_type == 'datacenter':
            dc_tag = "vsphere_datacenter:%s" % obj.name
            tags_copy.append(dc_tag)
            for compute_resource in obj.hostFolder.childEntity:
                # Skip non-compute resource
                if not hasattr(compute_resource, 'host'):
                    continue
                self.pool.apply_async(
                    self._cache_morlist_raw_atomic,
                    args=(i_key, 'compute_resource', compute_resource, tags_copy, regexes)
                )

        elif obj_type == 'compute_resource':
            if obj.__class__ == vim.ClusterComputeResource:
                cluster_tag = "vsphere_cluster:%s" % obj.name
                tags_copy.append(cluster_tag)
            for host in obj.host:
                # Skip non-host
                if not hasattr(host, 'vm'):
                    continue
                self.pool.apply_async(
                    self._cache_morlist_raw_atomic,
                    args=(i_key, 'host', host, tags_copy, regexes)
                )

        elif obj_type == 'host':
            if regexes and regexes.get('host_include') is not None:
                match = re.search(regexes['host_include'], obj.name)
                if not match:
                    self.log.debug(u"Filtered out VM {0} because of host_include_only_regex".format(obj.name))
                    return
            watched_mor = dict(mor_type='host', mor=obj, hostname=obj.name, tags=tags_copy+['vsphere_type:host'])
            self.morlist_raw[i_key].append(watched_mor)

            host_tag = "vsphere_host:%s" % obj.name
            tags_copy.append(host_tag)
            for vm in obj.vm:
                if vm.runtime.powerState != 'poweredOn':
                    continue
                self.pool.apply_async(
                    self._cache_morlist_raw_atomic,
                    args=(i_key, 'vm', vm, tags_copy, regexes)
                )

        elif obj_type == 'vm':
            if regexes and regexes.get('vm_include') is not None:
                match = re.search(regexes['vm_include'], obj.name)
                if not match:
                    self.log.debug(u"Filtered out VM {0} because of vm_include_only_regex".format(obj.name))
                    return
            watched_mor = dict(mor_type='vm', mor=obj, hostname=obj.name, tags=tags_copy+['vsphere_type:vm'])
            self.morlist_raw[i_key].append(watched_mor)

        ### <TEST-INSTRUMENTATION>
        self.histogram('datadog.agent.vsphere.morlist_raw_atomic.time', t.total())
        ### </TEST-INSTRUMENTATION>

    def _cache_morlist_raw(self, instance):
        """ Initiate the first layer to refresh self.morlist by queueing
        _cache_morlist_raw_atomic on the rootFolder in a recursive/asncy approach
        """

        i_key = self._instance_key(instance)
        self.log.debug("Caching the morlist for vcenter instance %s" % i_key)
        if i_key in self.morlist_raw and len(self.morlist_raw[i_key]) > 0:
            self.log.debug(
                "Skipping morlist collection now, RAW results "
                "processing not over (latest refresh was {0}s ago)".format(
                    time.time() - self.cache_times[i_key][MORLIST][LAST])
            )
            return
        self.morlist_raw[i_key] = []

        server_instance = self._get_server_instance(instance)
        root_folder = server_instance.content.rootFolder

        instance_tag = "vcenter_server:%s" % instance.get('name')
        regexes = {
            'host_include': instance.get('host_include_only_regex'),
            'vm_include': instance.get('vm_include_only_regex')
        }
        self.pool.apply_async(
            self._cache_morlist_raw_atomic,
            args=(i_key, 'rootFolder', root_folder, [instance_tag], regexes)
        )
        self.cache_times[i_key][MORLIST][LAST] = time.time()

    @atomic_method
    def _cache_morlist_process_atomic(self, instance, mor):
        """ Process one item of the self.morlist_raw list by querying the available
        metrics for this MOR and then putting it in self.morlist
        """
        ### <TEST-INSTRUMENTATION>
        t = Timer()
        ### </TEST-INSTRUMENTATION>
        i_key = self._instance_key(instance)
        server_instance = self._get_server_instance(instance)
        perfManager = server_instance.content.perfManager

        self.log.debug(
            "job_atomic: Querying available metrics"
            " for MOR {0} (type={1})".format(mor['mor'], mor['mor_type'])
        )

        available_metrics = perfManager.QueryAvailablePerfMetric(
            mor['mor'], intervalId=REAL_TIME_INTERVAL)

        mor['metrics'] = self._compute_needed_metrics(instance, available_metrics)
        mor_name = str(mor['mor'])

        if mor_name in self.morlist[i_key]:
            # Was already here last iteration
            self.morlist[i_key][mor_name]['metrics'] = mor['metrics']
        else:
            self.morlist[i_key][mor_name] = mor

        self.morlist[i_key][mor_name]['last_seen'] = time.time()

        ### <TEST-INSTRUMENTATION>
        self.histogram('datadog.agent.vsphere.morlist_process_atomic.time', t.total())
        ### </TEST-INSTRUMENTATION>

    def _cache_morlist_process(self, instance):
        """ Empties the self.morlist_raw by popping items and running asynchronously
        the _cache_morlist_process_atomic operation that will get the available
        metrics for this MOR and put it in self.morlist
        """
        i_key = self._instance_key(instance)
        if i_key not in self.morlist:
            self.morlist[i_key] = {}

        batch_size = self.init_config.get('batch_morlist_size', BATCH_MORLIST_SIZE)

        for i in xrange(batch_size):
            try:
                mor = self.morlist_raw[i_key].pop()
                self.pool.apply_async(self._cache_morlist_process_atomic, args=(instance, mor))
            except (IndexError, KeyError):
                self.log.debug("No more work to process in morlist_raw")
                return

    def _vacuum_morlist(self, instance):
        """ Check if self.morlist doesn't have some old MORs that are gone, ie
        we cannot get any metrics from them anyway (or =0)
        """
        i_key = self._instance_key(instance)
        morlist = self.morlist[i_key].items()

        for mor_name, mor in morlist:
            last_seen = mor['last_seen']
            if (time.time() - last_seen) > 2 * REFRESH_MORLIST_INTERVAL:
                del self.morlist[i_key][mor_name]

    def _cache_metrics_metadata(self, instance):
        """ Get from the server instance, all the performance counters metadata
        meaning name/group/description... attached with the corresponding ID
        """
        ### <TEST-INSTRUMENTATION>
        t = Timer()
        ### </TEST-INSTRUMENTATION>

        i_key = self._instance_key(instance)
        self.log.info("Warming metrics metadata cache for instance {0}".format(i_key))
        server_instance = self._get_server_instance(instance)
        perfManager = server_instance.content.perfManager

        new_metadata = {}
        for counter in perfManager.perfCounter:
            d = dict(
                name = "%s.%s" % (counter.groupInfo.key, counter.nameInfo.key),
                unit = counter.unitInfo.key,
                instance_tag = 'instance'  # FIXME: replace by what we want to tag!
            )
            new_metadata[counter.key] = d
        self.cache_times[i_key][METRICS_METADATA][LAST] = time.time()

        self.log.info("Finished metadata collection for instance {0}".format(i_key))
        # Reset metadata
        self.metrics_metadata[i_key] = new_metadata

        ### <TEST-INSTRUMENTATION>
        self.histogram('datadog.agent.vsphere.metric_metadata_collection.time', t.total())
        ### </TEST-INSTRUMENTATION>

    def _transform_value(self, instance, counter_id, value):
        """ Given the counter_id, look up for the metrics metadata to check the vsphere
        type of the counter and apply pre-reporting transformation if needed.
        """
        i_key = self._instance_key(instance)
        if counter_id in self.metrics_metadata[i_key]:
            unit = self.metrics_metadata[i_key][counter_id]['unit']
            if unit == 'percent':
                return float(value) / 100

        # Defaults to return the value without transformation
        return value

    @atomic_method
    def _collect_metrics_atomic(self, instance, mor):
        """ Task that collects the metrics listed in the morlist for one MOR
        """
        ### <TEST-INSTRUMENTATION>
        t = Timer()
        ### </TEST-INSTRUMENTATION>

        i_key = self._instance_key(instance)
        server_instance = self._get_server_instance(instance)
        perfManager = server_instance.content.perfManager
        query = vim.PerformanceManager.QuerySpec(maxSample=1,
                                                 entity=mor['mor'],
                                                 metricId=mor['metrics'],
                                                 intervalId=20,
                                                 format='normal')
        results = perfManager.QueryPerf(querySpec=[query])
        if results:
            for result in results[0].value:
                if result.id.counterId not in self.metrics_metadata[i_key]:
                    self.log.debug("Skipping this metric value, because there is no metadata about it")
                    continue
                instance_name = result.id.instance or "none"
                value = self._transform_value(instance, result.id.counterId, result.value[0])
                self.gauge(
                    "vsphere.%s" % self.metrics_metadata[i_key][result.id.counterId]['name'],
                    value,
                    hostname=mor['hostname'],
                    tags=['instance:%s' % instance_name]
                )

        ### <TEST-INSTRUMENTATION>
        self.histogram('datadog.agent.vsphere.metric_colection.time', t.total())
        ### </TEST-INSTRUMENTATION>

    def collect_metrics(self, instance):
        """ Calls asynchronously _collect_metrics_atomic on all MORs, as the
        job queue is processed the Aggregator will receive the metrics.
        """
        i_key = self._instance_key(instance)
        if i_key not in self.morlist:
            self.log.debug("Not collecting metrics for this instance, nothing to do yet: {0}".format(i_key))
            return

        mors = self.morlist[i_key].items()
        self.log.debug("Collecting metrics of %d mors" % len(mors))

        vm_count = 0

        for mor_name, mor in mors:
            if mor['mor_type'] == 'vm':
                vm_count += 1
            if 'metrics' not in mor:
                # self.log.debug("Skipping entity %s collection because we didn't cache its metrics yet" % mor['hostname'])
                continue

            self.pool.apply_async(self._collect_metrics_atomic, args=(instance, mor))

        self.gauge('vsphere.vm.count', vm_count, tags=["vcenter_server:%s" % instance.get('name')])

    def check(self, instance):
        if not self.pool_started:
            self.start_pool()
        ### <TEST-INSTRUMENTATION>
        self.gauge('datadog.agent.vsphere.queue_size', self.pool._workq.qsize(), tags=['instant:initial'])
        ### </TEST-INSTRUMENTATION>

        # First part: make sure our object repository is neat & clean
        if self._should_cache(instance, METRICS_METADATA):
            self._cache_metrics_metadata(instance)

        if self._should_cache(instance, MORLIST):
            self._cache_morlist_raw(instance)
        self._cache_morlist_process(instance)
        self._vacuum_morlist(instance)

        # Second part: do the job
        self.collect_metrics(instance)
        self._query_event(instance)

        # For our own sanity
        self._clean()

        thread_crashed = False
        try:
            while True:
                self.log.critical(self.exceptionq.get_nowait())
                thread_crashed = True
        except Empty:
            pass
        if thread_crashed:
            self.stop_pool()
            raise Exception("One thread in the pool crashed, check the logs")

        ### <TEST-INSTRUMENTATION>
        self.gauge('datadog.agent.vsphere.queue_size', self.pool._workq.qsize(), tags=['instant:final'])
        ### </TEST-INSTRUMENTATION>

if __name__ == '__main__':
    check, _instances = VSphereCheck.from_yaml('conf.d/vsphere.yaml')
    try:
        for i in xrange(200):
            print "Loop %d" % i
            for instance in check.instances:
                check.check(instance)
                if check.has_events():
                    print 'Events: %s' % (check.get_events())
                print 'Metrics: %d' % (len(check.get_metrics()))
            time.sleep(10)
    except Exception as e:
        print "Whoops something happened {0}".format(traceback.format_exc())
    finally:
        check.stop()
