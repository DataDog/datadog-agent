#!/opt/datadog-agent/embedded/bin/python
"""
    Datadog
    www.datadoghq.com
    ----
    Cloud-Scale Monitoring. Monitoring that tracks your dynamic infrastructure.

    Licensed under Simplified BSD License (see LICENSE)
    (C) Datadog, Inc. 2010-2016 all rights reserved
"""

# set up logging before importing any other components
from config import initialize_logging  # noqa
initialize_logging('service')

# stdlib
from collections import deque
import logging
import multiprocessing
import os
import psutil
import time

# win32
import win32serviceutil
import servicemanager
import win32service

# project
from config import get_config, get_config_path, get_confd_path
from jmxfetch import JMXFetch
from utils.jmx import JMXFiles
from utils.windows_configuration import get_registry_conf, update_conf_file


log = logging.getLogger('service')


SERVICE_SLEEP_INTERVAL = 1


class AgentSvc(win32serviceutil.ServiceFramework):
    _svc_name_ = "DatadogAgent"
    _svc_display_name_ = "Datadog Agent"
    _svc_description_ = "Sends metrics to Datadog"
    _MAX_JMXFETCH_RESTARTS = 3
    devnull = None

    def __init__(self, args):
        win32serviceutil.ServiceFramework.__init__(self, args)

        AgentSvc.devnull = open(os.devnull, 'w')

        config = get_config(parse_args=False, can_query_registry=False)
        if config['api_key'] == 'APIKEYHERE':
            self._update_config_file(config)

        # Let's have an uptime counter
        self.start_ts = None

        # find the main agent dir, for instance C:\Program Files\Datadog\Datadog Agent
        dd_dir = self._find_dd_dir()
        # clean the env vars
        agent_env = self._prepare_agent_env(dd_dir)
        # cd to the agent dir to easily launch the 4 components
        os.chdir(os.path.join(dd_dir, 'agent'))

        embedded_python = os.path.join(dd_dir, 'embedded', 'python.exe')
        # Keep a list of running processes so we can start/end as needed.
        # Processes will start started in order and stopped in reverse order.
        self.procs = {
            'forwarder': DDProcess(
                "forwarder",
                [embedded_python, "ddagent.py"],
                agent_env
            ),
            'collector': DDProcess(
                "collector",
                [embedded_python, "agent.py", "foreground", "--use-local-forwarder"],
                agent_env
            ),
            'dogstatsd': DDProcess(
                "dogstatsd",
                [embedded_python, "dogstatsd.py", "--use-local-forwarder"],
                agent_env,
                enabled=config.get("use_dogstatsd", True)
            ),
            'jmxfetch': JMXFetchProcess(
                "jmxfetch",
                [embedded_python, "jmxfetch.py"],
                agent_env,
                max_restarts=self._MAX_JMXFETCH_RESTARTS,
                enabled=self._is_jmxfetch_enabled(config)
            ),
        }

    def _find_dd_dir(self):
        # This file is somewhere in the dist directory of the agent
        file_dir = os.path.dirname(os.path.realpath(__file__))
        search_dir, current_dir = os.path.split(file_dir)
        # So we go all the way up to the dist directory to find the actual agent dir
        while current_dir and current_dir != 'dist':
            search_dir, current_dir = os.path.split(search_dir)
        dd_dir = search_dir
        # If we don't find it, we use the default
        if not current_dir:
            dd_dir = os.path.join('C:\\', 'Program Files', 'Datadog', 'Datadog Agent')

        return dd_dir

    def _prepare_agent_env(self, dd_dir):
        # preparing a clean env for the agent processes
        env = os.environ.copy()
        if env.get('PYTHONPATH'):
            del env['PYTHONPATH']
        if env.get('PYTHONHOME'):
            del env['PYTHONHOME']
        if env['PATH'][-1] != ';':
            env['PATH'] += ';'
        env['PATH'] += "{};{};".format(os.path.join(dd_dir, 'bin'), os.path.join(dd_dir, 'embedded'))
        log.debug('env: %s', env)

        return env

    def _is_jmxfetch_enabled(self, config):
        confd_path = get_confd_path()
        jmxfetch = JMXFetch(confd_path, config)
        jmxfetch.configure()
        return jmxfetch.should_run()

    def _update_config_file(self, config):
        log.debug('Querying registry to get missing config options')
        registry_conf = get_registry_conf(config)
        config.update(registry_conf)
        if registry_conf:
            log.info('Updating conf file options: %s', registry_conf.keys())
            update_conf_file(registry_conf, get_config_path())

    def SvcStop(self):
        # Stop all services.
        self.running = False
        log.info('Stopping service...')
        # Stop all services.
        for proc in self.procs.values():
            proc.terminate()
        AgentSvc.devnull.close()
        log.info("Agent processes stopped.")

        # Let's log the uptime
        if self.start_ts is not None:
            secs = int(time.time()-self.start_ts)
            mins = int(secs/60)
            hours = int(secs/3600)
            log.info("Uptime: %d hours %d minutes %d seconds", hours, mins % 60, secs % 60)

        self.ReportServiceStatus(win32service.SERVICE_STOP_PENDING)

    def SvcDoRun(self):
        log.info('Starting service...')
        servicemanager.LogMsg(
            servicemanager.EVENTLOG_INFORMATION_TYPE,
            servicemanager.PYS_SERVICE_STARTED,
            (self._svc_name_, '')
        )
        self.start_ts = time.time()

        # Start all services.
        for proc in self.procs.values():
            proc.start()

        # Loop to keep the service running since all DD services are
        # running in separate processes
        self.running = True
        while self.running:
            # Restart any processes that might have died.
            for name, proc in self.procs.iteritems():
                if not proc.is_alive() and proc.is_enabled():
                    log.warning("%s has died. Restarting...", name)
                    proc.restart()

            time.sleep(SERVICE_SLEEP_INTERVAL)

        servicemanager.LogMsg(
            servicemanager.EVENTLOG_INFORMATION_TYPE,
            servicemanager.PYS_SERVICE_STOPPED,
            (self._svc_name_, '')
        )
        log.info("Service stopped.")


class DDProcess(object):
    """
    Starts and monitors a Datadog process.
    Restarts when it exits until the limit set is reached.
    """
    DEFAULT_MAX_RESTARTS = 5
    _RESTART_TIMEFRAME = 3600
    _STOP_TIMEOUT = 3

    def __init__(self, name, command, env, enabled=True, max_restarts=None):
        self._name = name
        self._command = command
        self._env = env.copy()
        self._enabled = enabled
        self._proc = None
        self._restarts = deque([])
        self._max_restarts = max_restarts or self.DEFAULT_MAX_RESTARTS

    def start(self):
        if self.is_enabled():
            log.info("Starting %s.", self._name)
            self._proc = psutil.Popen(
                self._command,
                stdout=AgentSvc.devnull,
                stderr=AgentSvc.devnull,
                env=self._env
            )
        else:
            log.info("%s is not enabled, not starting it.", self._name)

    def stop(self):
        if self._proc is not None and self._proc.is_running():
            log.info("Stopping %s...", self._name)
            children_proc = self._proc.children()

            self._proc.terminate()

            # Should never have to wait, terminate is equivalent to kill on Windows
            self._proc.wait(timeout=self._STOP_TIMEOUT)
            if self._proc.is_running():
                log.debug("%s didn't exit. Killing it.", self._name)
                self._proc.kill()

            # Mostly for JMXFetch, if it didn't exit yet, kill it
            for child_proc in children_proc:
                if child_proc.is_running():
                    child_proc.kill()

            log.info("%s is stopped.", self._name)
        else:
            log.debug('%s was not running.', self._name)

    def terminate(self):
        self.stop()

    def is_alive(self):
        return self._proc is not None and self._proc.is_running()

    def is_enabled(self):
        return self._enabled

    def _can_restart(self):
        now = time.time()
        while(self._restarts and self._restarts[0] < now - self._RESTART_TIMEFRAME):
            self._restarts.popleft()

        return len(self._restarts) < self._max_restarts

    def restart(self):
        if not self._can_restart():
            log.error(
                "%s reached the limit of restarts (%d tries during the last %ds"
                " (max authorized: %d)). Not restarting.",
                self._name, len(self._restarts),
                self._RESTART_TIMEFRAME, self._max_restarts
            )
            self._enabled = False
            return

        self._restarts.append(time.time())

        if self.is_alive():
            self.stop()

        self.start()


class JMXFetchProcess(DDProcess):
    # It's an entire JMXFetch run, because JMXFetch only checks that the exit
    # file exists at the beginning of each run
    _JMX_STOP_TIMEOUT = 20

    def start(self):
        if self.is_enabled():
            JMXFiles.clean_exit_file()

        super(JMXFetchProcess, self).start()

    def stop(self):
        """
        Override `stop` method to properly exit JMXFetch.
        """
        if self._proc is not None and self._proc.is_running():
            JMXFiles.write_exit_file()
            try:
                self._proc.wait(timeout=self._JMX_STOP_TIMEOUT)
            except psutil.TimeoutExpired:
                log.debug("JMXFetch process didn't stop after %ss, killing it", self._JMX_STOP_TIMEOUT)

        super(JMXFetchProcess, self).stop()


if __name__ == '__main__':
    multiprocessing.freeze_support()
    win32serviceutil.HandleCommandLine(AgentSvc)
