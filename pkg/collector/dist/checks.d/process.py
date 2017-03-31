# (C) Datadog, Inc. 2010-2016
# (C) Luca Cipriani <luca@c9.io> 2013
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
from collections import defaultdict
import time

# 3p
import psutil

# project
from agent import AgentCheck
from config import _is_affirmative
from utils.platform import Platform


DEFAULT_AD_CACHE_DURATION = 120
DEFAULT_PID_CACHE_DURATION = 120


ATTR_TO_METRIC = {
    'thr':              'threads',
    'cpu':              'cpu.pct',
    'rss':              'mem.rss',
    'vms':              'mem.vms',
    'real':             'mem.real',
    'open_fd':          'open_file_descriptors',
    'open_handle':      'open_handles',  # win32 only
    'r_count':          'ioread_count',  # FIXME: namespace me correctly (6.x), io.r_count
    'w_count':          'iowrite_count',  # FIXME: namespace me correctly (6.x) io.r_bytes
    'r_bytes':          'ioread_bytes',  # FIXME: namespace me correctly (6.x) io.w_count
    'w_bytes':          'iowrite_bytes',  # FIXME: namespace me correctly (6.x) io.w_bytes
    'ctx_swtch_vol':    'voluntary_ctx_switches',  # FIXME: namespace me correctly (6.x), ctx_swt.voluntary
    'ctx_swtch_invol':  'involuntary_ctx_switches',  # FIXME: namespace me correctly (6.x), ctx_swt.involuntary
    'run_time':         'run_time',
    'mem_pct':          'mem.pct'
}

ATTR_TO_METRIC_RATE = {
    'minflt':           'mem.page_faults.minor_faults',
    'cminflt':          'mem.page_faults.children_minor_faults',
    'majflt':           'mem.page_faults.major_faults',
    'cmajflt':          'mem.page_faults.children_major_faults'
}


class ProcessCheck(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances=None):
        super(ProcessCheck, self).__init__(name, init_config, agentConfig, instances)

        # ad stands for access denied
        # We cache the PIDs getting this error and don't iterate on them
        # more often than `access_denied_cache_duration`
        # This cache is for all PIDs so it's global, but it should
        # be refreshed by instance
        self.last_ad_cache_ts = {}
        self.ad_cache = set()
        self.access_denied_cache_duration = int(
            init_config.get(
                'access_denied_cache_duration',
                DEFAULT_AD_CACHE_DURATION
            )
        )

        # By default cache the PID list for a while
        # Sometimes it's not wanted b/c it can mess with no-data monitoring
        # This cache is indexed per instance
        self.last_pid_cache_ts = {}
        self.pid_cache = {}
        self.pid_cache_duration = int(
            init_config.get(
                'pid_cache_duration',
                DEFAULT_PID_CACHE_DURATION
            )
        )

        self._conflicting_procfs = False
        self._deprecated_init_procfs = False
        if Platform.is_linux():
            procfs_path = init_config.get('procfs_path')
            if procfs_path:
                if 'procfs_path' in agentConfig and procfs_path != agentConfig.get('procfs_path').rstrip('/'):
                    self._conflicting_procfs = True
                else:
                    self._deprecated_init_procfs = True
                    psutil.PROCFS_PATH = procfs_path

        # Process cache, indexed by instance
        self.process_cache = defaultdict(dict)

    def should_refresh_ad_cache(self, name):
        now = time.time()
        return now - self.last_ad_cache_ts.get(name, 0) > self.access_denied_cache_duration

    def should_refresh_pid_cache(self, name):
        now = time.time()
        return now - self.last_pid_cache_ts.get(name, 0) > self.pid_cache_duration

    def find_pids(self, name, search_string, exact_match, ignore_ad=True):
        """
        Create a set of pids of selected processes.
        Search for search_string
        """
        if not self.should_refresh_pid_cache(name):
            return self.pid_cache[name]

        ad_error_logger = self.log.debug
        if not ignore_ad:
            ad_error_logger = self.log.error

        refresh_ad_cache = self.should_refresh_ad_cache(name)

        matching_pids = set()

        for proc in psutil.process_iter():
            # Skip access denied processes
            if not refresh_ad_cache and proc.pid in self.ad_cache:
                continue

            found = False
            for string in search_string:
                try:
                    if exact_match:
                        if proc.name() == string:
                            found = True
                    else:
                        cmdline = proc.cmdline()
                        if string in ' '.join(cmdline):
                            found = True
                except psutil.NoSuchProcess:
                    self.log.warning('Process disappeared while scanning')
                except psutil.AccessDenied as e:
                    ad_error_logger('Access denied to process with PID %s', proc.pid)
                    ad_error_logger('Error: %s', e)
                    if refresh_ad_cache:
                        self.ad_cache.add(proc.pid)
                    if not ignore_ad:
                        raise
                else:
                    if refresh_ad_cache:
                        self.ad_cache.discard(proc.pid)
                    if found:
                        matching_pids.add(proc.pid)
                        break

        self.pid_cache[name] = matching_pids
        self.last_pid_cache_ts[name] = time.time()
        if refresh_ad_cache:
            self.last_ad_cache_ts[name] = time.time()
        return matching_pids

    def psutil_wrapper(self, process, method, accessors, *args, **kwargs):
        """
        A psutil wrapper that is calling
        * psutil.method(*args, **kwargs) and returns the result
        OR
        * psutil.method(*args, **kwargs).accessor[i] for each accessors given in
        a list, the result being indexed in a dictionary by the accessor name
        """

        if accessors is None:
            result = None
        else:
            result = {}

        # Ban certain method that we know fail
        if method == 'memory_info_ex'\
                and (Platform.is_win32() or Platform.is_solaris()):
            return result
        elif method == 'num_fds' and not Platform.is_unix():
            return result
        elif method == 'num_handles' and not Platform.is_win32():
            return result

        try:
            res = getattr(process, method)(*args, **kwargs)
            if accessors is None:
                result = res
            else:
                for acc in accessors:
                    try:
                        result[acc] = getattr(res, acc)
                    except AttributeError:
                        self.log.debug("psutil.%s().%s attribute does not exist", method, acc)
        except (NotImplementedError, AttributeError):
            self.log.debug("psutil method %s not implemented", method)
        except psutil.AccessDenied:
            self.log.debug("psutil was denied acccess for method %s", method)
        except psutil.NoSuchProcess:
            self.warning("Process {0} disappeared while scanning".format(process.pid))

        return result

    def get_process_state(self, name, pids):
        st = defaultdict(list)

        # Remove from cache the processes that are not in `pids`
        cached_pids = set(self.process_cache[name].keys())
        pids_to_remove = cached_pids - pids
        for pid in pids_to_remove:
            del self.process_cache[name][pid]

        for pid in pids:
            st['pids'].append(pid)

            new_process = False
            # If the pid's process is not cached, retrieve it
            if pid not in self.process_cache[name] or not self.process_cache[name][pid].is_running():
                new_process = True
                try:
                    self.process_cache[name][pid] = psutil.Process(pid)
                    self.log.debug('New process in cache: %s' % pid)
                # Skip processes dead in the meantime
                except psutil.NoSuchProcess:
                    self.warning('Process %s disappeared while scanning' % pid)
                    # reset the PID cache now, something changed
                    self.last_pid_cache_ts[name] = 0
                    continue

            p = self.process_cache[name][pid]

            meminfo = self.psutil_wrapper(p, 'memory_info', ['rss', 'vms'])
            st['rss'].append(meminfo.get('rss'))
            st['vms'].append(meminfo.get('vms'))

            mem_percent = self.psutil_wrapper(p, 'memory_percent', None)
            st['mem_pct'].append(mem_percent)

            # will fail on win32 and solaris
            shared_mem = self.psutil_wrapper(p, 'memory_info_ex', ['shared']).get('shared')
            if shared_mem is not None and meminfo.get('rss') is not None:
                st['real'].append(meminfo['rss'] - shared_mem)
            else:
                st['real'].append(None)

            ctxinfo = self.psutil_wrapper(p, 'num_ctx_switches', ['voluntary', 'involuntary'])
            st['ctx_swtch_vol'].append(ctxinfo.get('voluntary'))
            st['ctx_swtch_invol'].append(ctxinfo.get('involuntary'))

            st['thr'].append(self.psutil_wrapper(p, 'num_threads', None))

            cpu_percent = self.psutil_wrapper(p, 'cpu_percent', None)
            if not new_process:
                # psutil returns `0.` for `cpu_percent` the first time it's sampled on a process,
                # so save the value only on non-new processes
                st['cpu'].append(cpu_percent)

            st['open_fd'].append(self.psutil_wrapper(p, 'num_fds', None))
            st['open_handle'].append(self.psutil_wrapper(p, 'num_handles', None))

            ioinfo = self.psutil_wrapper(p, 'io_counters', ['read_count', 'write_count', 'read_bytes', 'write_bytes'])
            st['r_count'].append(ioinfo.get('read_count'))
            st['w_count'].append(ioinfo.get('write_count'))
            st['r_bytes'].append(ioinfo.get('read_bytes'))
            st['w_bytes'].append(ioinfo.get('write_bytes'))

            pagefault_stats = self.get_pagefault_stats(pid)
            if pagefault_stats is not None:
                (minflt, cminflt, majflt, cmajflt) = pagefault_stats
                st['minflt'].append(minflt)
                st['cminflt'].append(cminflt)
                st['majflt'].append(majflt)
                st['cmajflt'].append(cmajflt)
            else:
                st['minflt'].append(None)
                st['cminflt'].append(None)
                st['majflt'].append(None)
                st['cmajflt'].append(None)

            #calculate process run time
            create_time = self.psutil_wrapper(p, 'create_time', None)
            if create_time is not None:
                now = time.time()
                run_time = now - create_time
                st['run_time'].append(run_time)

        return st

    def get_pagefault_stats(self, pid):
        if not Platform.is_linux():
            return None

        def file_to_string(path):
            with open(path, 'r') as f:
                res = f.read()
            return res

        # http://man7.org/linux/man-pages/man5/proc.5.html
        try:
            data = file_to_string('/%s/%s/stat' % (psutil.PROCFS_PATH, pid))
        except Exception:
            self.log.debug('error getting proc stats: file_to_string failed'
                           'for /%s/%s/stat' % (psutil.PROCFS_PATH, pid))
            return None

        return map(lambda i: int(i), data.split()[9:13])

    def check(self, instance):
        name = instance.get('name', None)
        tags = instance.get('tags', [])
        exact_match = _is_affirmative(instance.get('exact_match', True))
        search_string = instance.get('search_string', None)
        ignore_ad = _is_affirmative(instance.get('ignore_denied_access', True))
        pid = instance.get('pid')
        pid_file = instance.get('pid_file')

        if self._conflicting_procfs:
            self.warning('The `procfs_path` defined in `process.yaml` is different from the one defined in '
                         '`datadog.conf`. This is currently not supported by the Agent. Defaulting to the '
                         'value defined in `datadog.conf`: {}'.format(psutil.PROCFS_PATH))
        elif self._deprecated_init_procfs:
            self.warning('DEPRECATION NOTICE: Specifying `procfs_path` in `process.yaml` is deprecated. '
                         'Please specify it in `datadog.conf` instead')

        if not isinstance(search_string, list) and pid is None and pid_file is None:
            raise ValueError('"search_string" or "pid" or "pid_file" parameter is required')

        if name is None:
            raise KeyError('The "name" of process groups is mandatory')

        if search_string is not None:
            pids = self.find_pids(
                name,
                search_string,
                exact_match,
                ignore_ad=ignore_ad
            )
        elif pid is not None:
            # we use Process(pid) as a means to search, if pid not found
            # psutil.NoSuchProcess is raised.
            pids = self._get_pid_set(pid)
        elif pid_file is not None:
            with open(pid_file, 'r') as file_pid:
                pid_line = file_pid.readline().strip()
                pids = self._get_pid_set(int(pid_line))
        else:
            raise ValueError('The "search_string" or "pid" options are required for process identification')

        proc_state = self.get_process_state(name, pids)

        # FIXME 6.x remove the `name` tag
        tags.extend(['process_name:%s' % name, name])

        self.log.debug('ProcessCheck: process %s analysed', name)
        self.gauge('system.processes.number', len(pids), tags=tags)

        for attr, mname in ATTR_TO_METRIC.iteritems():
            vals = [x for x in proc_state[attr] if x is not None]
            # skip []
            if vals:
                if attr == 'run_time':
                    self.gauge('system.processes.%s.avg' % mname, sum(vals)/len(vals), tags=tags)
                    self.gauge('system.processes.%s.max' % mname, max(vals), tags=tags)
                    self.gauge('system.processes.%s.min' % mname, min(vals), tags=tags)

                # FIXME 6.x: change this prefix?
                else:
                    self.gauge('system.processes.%s' % mname, sum(vals), tags=tags)

        for attr, mname in ATTR_TO_METRIC_RATE.iteritems():
            vals = [x for x in proc_state[attr] if x is not None]
            if vals:
                self.rate('system.processes.%s' % mname, sum(vals), tags=tags)

        self._process_service_check(name, len(pids), instance.get('thresholds', None), tags)

    def _get_pid_set(self, pid):
        try:
            return {psutil.Process(pid).pid}
        except psutil.NoSuchProcess:
            return set()

    def _process_service_check(self, name, nb_procs, bounds, tags):
        """
        Report a service check, for each process in search_string.
        Report as OK if the process is in the warning thresholds
                   CRITICAL             out of the critical thresholds
                   WARNING              out of the warning thresholds
        """
        # FIXME 6.x remove the `process:name` tag
        service_check_tags = tags + ["process:%s" % name]
        status = AgentCheck.OK
        message_str = "PROCS %s: %s processes found for %s"
        status_str = {
            AgentCheck.OK: "OK",
            AgentCheck.WARNING: "WARNING",
            AgentCheck.CRITICAL: "CRITICAL"
        }

        if not bounds and nb_procs < 1:
            status = AgentCheck.CRITICAL
        elif bounds:
            warning = bounds.get('warning', [1, float('inf')])
            critical = bounds.get('critical', [1, float('inf')])

            if warning[1] < nb_procs or nb_procs < warning[0]:
                status = AgentCheck.WARNING
            if critical[1] < nb_procs or nb_procs < critical[0]:
                status = AgentCheck.CRITICAL

        self.service_check(
            "process.up",
            status,
            tags=service_check_tags,
            message=message_str % (status_str[status], nb_procs, name)
        )
