# std
import logging
import os
import tempfile
import time

# 3rd party
import yaml

# datadog
from config import _windows_commondata_path, get_confd_path
from util import yDumper
from utils.pidfile import PidFile
from utils.platform import Platform

# JMXFetch java version
JMX_FETCH_JAR_NAME = "jmxfetch-0.9.0-jar-with-dependencies.jar"

log = logging.getLogger(__name__)


def jmx_command(args, agent_config, redirect_std_streams=False):
    """
    Run JMXFetch with the given command if it is valid (and print user-friendly info if it's not)
    """
    from jmxfetch import JMX_LIST_COMMANDS, JMXFetch
    if len(args) < 1 or args[0] not in JMX_LIST_COMMANDS.keys():
        print "#" * 80
        print "JMX tool to be used to help configuring your JMX checks."
        print "See http://docs.datadoghq.com/integrations/java/ for more information"
        print "#" * 80
        print "\n"
        print "You have to specify one of the following commands:"
        for command, desc in JMX_LIST_COMMANDS.iteritems():
            print "      - %s [OPTIONAL: LIST OF CHECKS]: %s" % (command, desc)
        print "Example: sudo /etc/init.d/datadog-agent jmx list_matching_attributes tomcat jmx solr"
        print "\n"

    else:
        jmx_command = args[0]
        checks_list = args[1:]
        confd_directory = get_confd_path()

        jmx_process = JMXFetch(confd_directory, agent_config)
        jmx_process.configure()
        should_run = jmx_process.should_run()

        if should_run:
            jmx_process.run(jmx_command, checks_list, reporter="console", redirect_std_streams=redirect_std_streams)
        else:
            print "Couldn't find any valid JMX configuration in your conf.d directory: %s" % confd_directory
            print "Have you enabled any JMX check ?"
            print "If you think it's not normal please get in touch with Datadog Support"


class JMXFiles(object):
    """
    A small helper class for JMXFetch status & exit files.
    """
    _STATUS_FILE = 'jmx_status.yaml'
    _PYTHON_STATUS_FILE = 'jmx_status_python.yaml'
    _JMX_EXIT_FILE = 'jmxfetch_exit'

    @classmethod
    def _get_dir(cls):
        if Platform.is_win32():
            path = os.path.join(_windows_commondata_path(), 'Datadog')
        elif os.path.isdir(PidFile.get_dir()):
            path = PidFile.get_dir()
        else:
            path = tempfile.gettempdir()
        return path

    @classmethod
    def _get_file_path(cls, file):
        return os.path.join(cls._get_dir(), file)

    @classmethod
    def get_status_file_path(cls):
        return cls._get_file_path(cls._STATUS_FILE)

    @classmethod
    def get_python_status_file_path(cls):
        return cls._get_file_path(cls._PYTHON_STATUS_FILE)

    @classmethod
    def get_python_exit_file_path(cls):
        return cls._get_file_path(cls._JMX_EXIT_FILE)

    @classmethod
    def write_status_file(cls, invalid_checks):
        data = {
            'timestamp': time.time(),
            'invalid_checks': invalid_checks
        }
        stream = file(os.path.join(cls._get_dir(), cls._PYTHON_STATUS_FILE), 'w')
        yaml.dump(data, stream, Dumper=yDumper)
        stream.close()

    @classmethod
    def write_exit_file(cls):
        """
        Create a 'special' file, which acts as a trigger to exit JMXFetch.
        Note: Windows only
        """
        open(os.path.join(cls._get_dir(), cls._JMX_EXIT_FILE), 'a').close()

    @classmethod
    def clean_status_file(cls):
        """
        Removes JMX status files
        """
        try:
            os.remove(os.path.join(cls._get_dir(), cls._STATUS_FILE))
        except OSError:
            pass
        try:
            os.remove(os.path.join(cls._get_dir(), cls._PYTHON_STATUS_FILE))
        except OSError:
            pass

    @classmethod
    def clean_exit_file(cls):
        """
        Remove exit file trigger -may not exist-.
        Note: Windows only
        """
        try:
            os.remove(os.path.join(cls._get_dir(), cls._JMX_EXIT_FILE))
        except OSError:
            pass

    @classmethod
    def get_jmx_appnames(cls):
        """
        Retrieves the running JMX checks based on the {tmp}/jmx_status.yaml file
        updated by JMXFetch (and the only communication channel between JMXFetch
        and the collector since JMXFetch).
        """
        check_names = []
        jmx_status_path = os.path.join(cls._get_dir(), cls._STATUS_FILE)
        if os.path.exists(jmx_status_path):
            jmx_checks = yaml.load(file(jmx_status_path)).get('checks', {})
            check_names = [name for name in jmx_checks.get('initialized_checks', {}).iterkeys()]
        return check_names
