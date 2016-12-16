# (C) Datadog, Inc. 2012-2016
# (C) Robert Leftwich <rleftwich@lightkeeper.com> 2012
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

"""
Custom parser for supervisord log suitable for use by Datadog 'dogstreams'

Add to datadog.conf as follows:

dogstreams: [path_to_supervisord.log]:datadog.streams.supervisord:parse_supervisord

"""
# stdlib
from datetime import datetime
from datadog import statsd
import re
import time

EVENT_TYPE = "supervisor"

# supervisord log levels
SUPERVISORD_LEVELS = [
    'CRIT',   # messages that probably require immediate user attention
    'ERRO',   # messages that indicate a potentially ignorable error condition
    'WARN',   # messages that indicate issues which aren't errors
    'INFO',   # normal informational output

    # IGNORED...
    #'DEBG',   # messages useful for users trying to debug configurations
    #'TRAC',   # messages useful to developers trying to debug plugins
    #'BLAT',   # messages useful for developers trying to debug supervisor

]

# mapping between datadog and supervisord log levels
ALERT_TYPES_MAPPING = {
    "CRIT": "error",
    "ERRO": "error",
    "WARN": "warning",
    "INFO": "info",
}

# regex to extract the 'program' supervisord is managing from the text
program_matcher = re.compile("^\w+:? '?(?P<program>\w+)'?")


def parse(log, line):
    """
    Parse the supervisord.log line into a dogstream event
    """
    if len(line) == 0:
        log.info("Skipping empty line of supervisord.log")
        return None
    if log:
        log.debug('PARSE supervisord:%s' % line)
    line_items = line.split(' ', 3)
    timestamp = ' '.join(line_items[:2])
    timestamp_parts = timestamp.split(',')
    dt = datetime.strptime(timestamp_parts[0], "%Y-%m-%d %H:%M:%S")
    dt = dt.replace(microsecond=int(timestamp_parts[1]))
    date = time.mktime(dt.timetuple())
    event_type = line_items[2]
    msg = line_items[3]
    if event_type in SUPERVISORD_LEVELS:
        alert_type = ALERT_TYPES_MAPPING.get(event_type, 'info')
        if alert_type == 'info' and 'success' in msg:
            alert_type = 'success'

        statsd.event(title = msg.strip(),
                     text = msg,
                     alert_type = alert_type,
                    date_happened=date)
        

if __name__ == "__main__":
    import sys
    import pprint
    import logging
    logging.basicConfig()
    log = logging.getLogger()
    lines = open(sys.argv[1]).readlines()
    pprint.pprint([parse_supervisord(log, line) for line in lines])
