import logging
from os.path import basename
import sys

log = logging.getLogger(__name__)


def deprecate_old_command_line_tools():
    name = basename(sys.argv[0])
    if name in ['dd-forwarder', 'dogstatsd', 'dd-agent']:
        log.warn("Using this command is deprecated and will be removed in a future version,"
                 " for more information see "
                 "https://github.com/DataDog/dd-agent/wiki/Deprecation-notice--(old-command-line-tools)")
