# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# set up logging before importing any other components
if __name__ == '__main__':
    from config import initialize_logging  # noqa
    initialize_logging('jmxfetch')

# stdlib
from contextlib import nested
import glob
import logging
import os
import signal
import sys
import tempfile
import time

# 3p
import yaml

# project
from config import (
    DEFAULT_CHECK_FREQUENCY,
    get_confd_path,
    get_config,
    get_jmx_pipe_path,
    get_logging_config,
    PathNotFound,
    _is_affirmative
)
from util import yLoader
from utils.jmx import JMX_FETCH_JAR_NAME, JMXFiles
from utils.platform import Platform
from utils.subprocess_output import subprocess

log = logging.getLogger('jmxfetch')

JAVA_LOGGING_LEVEL = {
    logging.CRITICAL: "FATAL",
    logging.DEBUG: "DEBUG",
    logging.ERROR: "ERROR",
    logging.FATAL: "FATAL",
    logging.INFO: "INFO",
    logging.WARN: "WARN",
    logging.WARNING: "WARN",
}

_JVM_DEFAULT_MAX_MEMORY_ALLOCATION = " -Xmx200m"
_JVM_DEFAULT_SD_MAX_MEMORY_ALLOCATION = " -Xmx512m"
_JVM_DEFAULT_INITIAL_MEMORY_ALLOCATION = " -Xms50m"
JMXFETCH_MAIN_CLASS = "org.datadog.jmxfetch.App"
JMX_CHECKS = [
    'activemq',
    'activemq_58',
    'cassandra',
    'jmx',
    'solr',
    'tomcat',
]
JMX_COLLECT_COMMAND = 'collect'
JMX_LIST_COMMANDS = {
    'list_everything': 'List every attributes available that has a type supported by JMXFetch',
    'list_collected_attributes': 'List attributes that will actually be collected by your current instances configuration',
    'list_matching_attributes': 'List attributes that match at least one of your instances configuration',
    'list_not_matching_attributes': "List attributes that don't match any of your instances configuration",
    'list_limited_attributes': "List attributes that do match one of your instances configuration but that are not being collected because it would exceed the number of metrics that can be collected",
    JMX_COLLECT_COMMAND: "Start the collection of metrics based on your current configuration and display them in the console"}

LINK_TO_DOC = "See http://docs.datadoghq.com/integrations/java/ for more information"


class InvalidJMXConfiguration(Exception):
    pass


class JMXFetch(object):
    """
    Start JMXFetch if any JMX check is configured
    """
    def __init__(self, confd_path, agentConfig):
        self.confd_path = confd_path
        self.agentConfig = agentConfig
        self.logging_config = get_logging_config()
        self.check_frequency = DEFAULT_CHECK_FREQUENCY
        self.service_discovery = _is_affirmative(self.agentConfig.get('sd_jmx_enable', False))

        self.jmx_process = None
        self.jmx_checks = None

    def terminate(self):
        self.jmx_process.terminate()

    def _handle_sigterm(self, signum, frame):
        # Terminate jmx process on SIGTERM signal
        log.debug("Caught sigterm. Stopping subprocess.")
        self.jmx_process.terminate()

    def register_signal_handlers(self):
        """
        Enable SIGTERM and SIGINT handlers
        """
        try:
            # Gracefully exit on sigterm
            signal.signal(signal.SIGTERM, self._handle_sigterm)

            # Handle Keyboard Interrupt
            signal.signal(signal.SIGINT, self._handle_sigterm)

        except ValueError:
            log.exception("Unable to register signal handlers.")

    def configure(self, checks_list=None, clean_status_file=True):
        """
        Instantiate JMXFetch parameters, clean potential previous run leftovers.
        """
        if clean_status_file:
            JMXFiles.clean_status_file()

        self.jmx_checks, self.invalid_checks, self.java_bin_path, self.java_options, \
            self.tools_jar_path, self.custom_jar_paths = \
            self.get_configuration(self.confd_path, checks_list=checks_list)

    def should_run(self):
        """
        Should JMXFetch run ?
        """
        return self.jmx_checks is not None and self.jmx_checks != []

    def run(self, command=None, checks_list=None, reporter=None, redirect_std_streams=False):
        """
        Run JMXFetch

        redirect_std_streams: if left to False, the stdout and stderr of JMXFetch are streamed
        directly to the environment's stdout and stderr and cannot be retrieved via python's
        sys.stdout and sys.stderr. Set to True to redirect these streams to python's sys.stdout
        and sys.stderr.
        """

        if checks_list or self.jmx_checks is None:
            # (Re)set/(re)configure JMXFetch parameters when `checks_list` is specified or
            # no configuration was found
            self.configure(checks_list)

        try:
            command = command or JMX_COLLECT_COMMAND

            if len(self.invalid_checks) > 0:
                try:
                    JMXFiles.write_status_file(self.invalid_checks)
                except Exception:
                    log.exception("Error while writing JMX status file")

            if len(self.jmx_checks) > 0 or self.service_discovery:
                return self._start(self.java_bin_path, self.java_options, self.jmx_checks,
                                   command, reporter, self.tools_jar_path, self.custom_jar_paths, redirect_std_streams)
            else:
                # We're exiting purposefully, so exit with zero (supervisor's expected
                # code). HACK: Sleep a little bit so supervisor thinks we've started cleanly
                # and thus can exit cleanly.
                time.sleep(4)
                log.info("No valid JMX integration was found. Exiting ...")
        except Exception:
            log.exception("Error while initiating JMXFetch")
            raise

    @classmethod
    def get_configuration(cls, confd_path, checks_list=None):
        """
        Return a tuple (jmx_checks, invalid_checks, java_bin_path, java_options, tools_jar_path)

        jmx_checks: list of yaml files that are jmx checks
        (they have the is_jmx flag enabled or they are in JMX_CHECKS)
        and that have at least one instance configured

        invalid_checks: dictionary whose keys are check names that are JMX checks but
        they have a bad configuration. Values of the dictionary are exceptions generated
        when checking the configuration

        java_bin_path: is the path to the java executable. It was
        previously set in the "instance" part of the yaml file of the
        jmx check. So we need to parse yaml files to get it.
        We assume that this value is alwayws the same for every jmx check
        so we can return the first value returned

        java_options: is string contains options that will be passed to java_bin_path
        We assume that this value is alwayws the same for every jmx check
        so we can return the first value returned

        tools_jar_path:  Path to tools.jar, which is only part of the JDK and that is
        required to connect to a local JMX instance using the attach api.
        """
        jmx_checks = []
        java_bin_path = None
        java_options = None
        tools_jar_path = None
        custom_jar_paths = []
        invalid_checks = {}

        jmx_confd_checks = get_jmx_checks(confd_path, auto_conf=False)

        for check in jmx_confd_checks:
            check_config = check['check_config']
            check_name = check['check_name']
            filename = check['filename']
            try:
                is_jmx, check_java_bin_path, check_java_options, check_tools_jar_path, check_custom_jar_paths = \
                    cls._is_jmx_check(check_config, check_name, checks_list)
                if is_jmx:
                    jmx_checks.append(filename)
                    if java_bin_path is None and check_java_bin_path is not None:
                        java_bin_path = check_java_bin_path
                    if java_options is None and check_java_options is not None:
                        java_options = check_java_options
                    if tools_jar_path is None and check_tools_jar_path is not None:
                        tools_jar_path = check_tools_jar_path
                    if check_custom_jar_paths:
                        custom_jar_paths.extend(check_custom_jar_paths)
            except InvalidJMXConfiguration as e:
                log.error("%s check does not have a valid JMX configuration: %s" % (check_name, e))
                # Make sure check_name is a string - Fix issues with Windows
                check_name = check_name.encode('ascii', 'ignore')
                invalid_checks[check_name] = str(e)

        return (jmx_checks, invalid_checks, java_bin_path, java_options, tools_jar_path, custom_jar_paths)

    def _start(self, path_to_java, java_run_opts, jmx_checks, command, reporter, tools_jar_path, custom_jar_paths, redirect_std_streams):
        if reporter is None:
            statsd_host = self.agentConfig.get('bind_host', 'localhost')
            if statsd_host == "0.0.0.0":
                # If statsd is bound to all interfaces, just use localhost for clients
                statsd_host = "localhost"
            statsd_port = self.agentConfig.get('dogstatsd_port', "8125")
            reporter = "statsd:%s:%s" % (statsd_host, statsd_port)

        log.info("Starting jmxfetch:")
        try:
            path_to_java = path_to_java or "java"
            java_run_opts = java_run_opts or ""
            path_to_jmxfetch = self._get_path_to_jmxfetch()
            path_to_status_file = JMXFiles.get_status_file_path()

            classpath = path_to_jmxfetch
            if tools_jar_path is not None:
                classpath = r"%s:%s" % (tools_jar_path, classpath)
            if custom_jar_paths:
                classpath = r"%s:%s" % (':'.join(custom_jar_paths), classpath)

            subprocess_args = [
                path_to_java,  # Path to the java bin
                '-classpath',
                classpath,
                JMXFETCH_MAIN_CLASS,
                '--check_period', str(self.check_frequency * 1000),  # Period of the main loop of jmxfetch in ms
                '--conf_directory', r"%s" % self.confd_path,  # Path of the conf.d directory that will be read by jmxfetch,
                '--log_level', JAVA_LOGGING_LEVEL.get(self.logging_config.get("log_level"), "INFO"),  # Log Level: Mapping from Python log level to log4j log levels
                '--log_location', r"%s" % self.logging_config.get('jmxfetch_log_file'),  # Path of the log file
                '--reporter', reporter,  # Reporter to use
                '--status_location', r"%s" % path_to_status_file,  # Path to the status file to write
                command,  # Name of the command
            ]

            if Platform.is_windows():
                # Signal handlers are not supported on Windows:
                # use a file to trigger JMXFetch exit instead
                path_to_exit_file = JMXFiles.get_python_exit_file_path()
                subprocess_args.insert(len(subprocess_args) - 1, '--exit_file_location')
                subprocess_args.insert(len(subprocess_args) - 1, path_to_exit_file)

            if self.service_discovery:
                pipe_path = get_jmx_pipe_path()
                subprocess_args.insert(4, '--tmp_directory')
                subprocess_args.insert(5, pipe_path)
                subprocess_args.insert(4, '--sd_standby')

            if jmx_checks:
                subprocess_args.insert(4, '--check')
                for check in jmx_checks:
                    subprocess_args.insert(5, check)

            # Specify a maximum memory allocation pool for the JVM
            if "Xmx" not in java_run_opts and "XX:MaxHeapSize" not in java_run_opts:
                java_run_opts += _JVM_DEFAULT_SD_MAX_MEMORY_ALLOCATION if self.service_discovery else _JVM_DEFAULT_MAX_MEMORY_ALLOCATION
            # Specify the initial memory allocation pool for the JVM
            if "Xms" not in java_run_opts and "XX:InitialHeapSize" not in java_run_opts:
                java_run_opts += _JVM_DEFAULT_INITIAL_MEMORY_ALLOCATION

            for opt in java_run_opts.split():
                subprocess_args.insert(1, opt)

            log.info("Running %s" % " ".join(subprocess_args))

            # Launch JMXfetch subprocess manually, w/o get_subprocess_output(), since it's a special case
            with nested(tempfile.TemporaryFile(), tempfile.TemporaryFile()) as (stdout_f, stderr_f):
                jmx_process = subprocess.Popen(
                    subprocess_args,
                    close_fds=not redirect_std_streams,  # only set to True when the streams are not redirected, for WIN compatibility
                    stdout=stdout_f if redirect_std_streams else None,
                    stderr=stderr_f if redirect_std_streams else None
                )
                self.jmx_process = jmx_process

                # Register SIGINT and SIGTERM signal handlers
                self.register_signal_handlers()

                # Wait for JMXFetch to return
                jmx_process.wait()

                if redirect_std_streams:
                    # Write out the stdout and stderr of JMXFetch to sys.stdout and sys.stderr
                    stderr_f.seek(0)
                    err = stderr_f.read()
                    stdout_f.seek(0)
                    out = stdout_f.read()
                    sys.stdout.write(out)
                    sys.stderr.write(err)

            return jmx_process.returncode

        except OSError:
            java_path_msg = "Couldn't launch JMXTerm. Is Java in your PATH ?"
            log.exception(java_path_msg)
            invalid_checks = {}
            for check in jmx_checks:
                check_name = check.split('.')[0]
                check_name = check_name.encode('ascii', 'ignore')
                invalid_checks[check_name] = java_path_msg
            JMXFiles.write_status_file(invalid_checks)
            raise
        except Exception:
            log.exception("Couldn't launch JMXFetch")
            raise

    @staticmethod
    def _is_jmx_check(check_config, check_name, checks_list):
        init_config = check_config.get('init_config', {}) or {}
        java_bin_path = None
        java_options = None
        is_jmx = False
        is_attach_api = False
        tools_jar_path = init_config.get("tools_jar_path")
        custom_jar_paths = init_config.get("custom_jar_paths")

        if init_config is None:
            init_config = {}

        if checks_list:
            if check_name in checks_list:
                is_jmx = True

        elif init_config.get('is_jmx') or check_name in JMX_CHECKS:
            is_jmx = True

        if is_jmx:
            instances = check_config.get('instances', [])
            if type(instances) != list or len(instances) == 0:
                raise InvalidJMXConfiguration("You need to have at least one instance "
                                              "defined in the YAML file for this check")

            for inst in instances:
                if type(inst) != dict:
                    raise InvalidJMXConfiguration("Each instance should be"
                                                  " a dictionary. %s" % LINK_TO_DOC)
                host = inst.get('host', None)
                port = inst.get('port', None)
                conf = inst.get('conf', init_config.get('conf', None))
                tools_jar_path = inst.get('tools_jar_path')

                # Support for attach api using a process name regex
                proc_regex = inst.get('process_name_regex')
                # Support for a custom jmx URL
                jmx_url = inst.get('jmx_url')
                name = inst.get('name')

                if proc_regex is not None:
                    is_attach_api = True
                elif jmx_url is not None:
                    if name is None:
                        raise InvalidJMXConfiguration("A name must be specified when using a jmx_url")
                else:
                    if host is None:
                        raise InvalidJMXConfiguration("A host must be specified")
                    if port is None or type(port) != int:
                        raise InvalidJMXConfiguration("A numeric port must be specified")

                if conf is None:
                    log.warning("%s doesn't have a 'conf' section. Only basic JVM metrics"
                                " will be collected. %s" % (check_name, LINK_TO_DOC))
                else:
                    if type(conf) != list or len(conf) == 0:
                        raise InvalidJMXConfiguration("'conf' section should be a list"
                                                      " of configurations %s" % LINK_TO_DOC)

                    for config in conf:
                        include = config.get('include', None)
                        if include is None:
                            raise InvalidJMXConfiguration("Each configuration must have an"
                                                          " 'include' section. %s" % LINK_TO_DOC)

                        if type(include) != dict:
                            raise InvalidJMXConfiguration("'include' section must"
                                                          " be a dictionary %s" % LINK_TO_DOC)

            if java_bin_path is None:
                if init_config and init_config.get('java_bin_path'):
                    # We get the java bin path from the yaml file
                    # for backward compatibility purposes
                    java_bin_path = init_config.get('java_bin_path')

                else:
                    for instance in instances:
                        if instance and instance.get('java_bin_path'):
                            java_bin_path = instance.get('java_bin_path')

            if java_options is None:
                if init_config and init_config.get('java_options'):
                    java_options = init_config.get('java_options')
                else:
                    for instance in instances:
                        if instance and instance.get('java_options'):
                            java_options = instance.get('java_options')

            if is_attach_api:
                if tools_jar_path is None:
                    for instance in instances:
                        if instance and instance.get("tools_jar_path"):
                            tools_jar_path = instance.get("tools_jar_path")

                if tools_jar_path is None:
                    raise InvalidJMXConfiguration("You must specify the path to tools.jar"
                                                  " in your JDK.")
                elif not os.path.isfile(tools_jar_path):
                    raise InvalidJMXConfiguration("Unable to find tools.jar at %s" % tools_jar_path)
            else:
                tools_jar_path = None

            if custom_jar_paths:
                if isinstance(custom_jar_paths, basestring):
                    custom_jar_paths = [custom_jar_paths]
                for custom_jar_path in custom_jar_paths:
                    if not os.path.isfile(custom_jar_path):
                        raise InvalidJMXConfiguration("Unable to find custom jar at %s" % custom_jar_path)

        return is_jmx, java_bin_path, java_options, tools_jar_path, custom_jar_paths

    def _get_path_to_jmxfetch(self):
        return os.path.realpath(os.path.join(os.path.abspath(__file__), "..", "checks",
            "libs", JMX_FETCH_JAR_NAME))


def get_jmx_checks(confd_path=None, auto_conf=False):
    jmx_checks = []

    if not confd_path:
        confd_path = get_confd_path()

    if auto_conf:
        path = confd_path + '/auto_conf'
    else:
        path = confd_path

    for conf in glob.glob(os.path.join(path, '*.yaml')):
        filename = os.path.basename(conf)
        check_name = filename.split('.')[0]
        if os.path.exists(conf):
            with open(conf, 'r') as f:
                try:
                    check_config = yaml.load(f.read(), Loader=yLoader)
                    assert check_config is not None
                except Exception:
                    log.error("Unable to parse yaml config in %s" % conf)
                    continue

        init_config = check_config.get('init_config', {}) or {}

        if init_config.get('is_jmx') or check_name in JMX_CHECKS:
            # If called by `get_configuration()` we should return the check_config and check_name
            if auto_conf:
                jmx_checks.append(check_name)
            else:
                jmx_checks.append({'check_config': check_config, 'check_name': check_name, 'filename': filename})

    if auto_conf:
        # Calls from SD expect all JMX checks, let's add check names in JMX_CHECKS
        for check in JMX_CHECKS:
            if check not in jmx_checks:
                jmx_checks.append(check)

    return jmx_checks

def init(config_path=None):
    agentConfig = get_config(parse_args=False, cfg_path=config_path)
    try:
        confd_path = get_confd_path()
    except PathNotFound as e:
        log.error("No conf.d folder found at '%s' or in the directory where"
                  "the Agent is currently deployed.\n" % e.args[0])

    return confd_path, agentConfig


def main(config_path=None):
    """ JMXFetch main entry point """
    confd_path, agentConfig = init(config_path)

    jmx = JMXFetch(confd_path, agentConfig)
    return jmx.run()

if __name__ == '__main__':
    sys.exit(main())
