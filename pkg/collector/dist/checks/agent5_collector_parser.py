from datadog import statsd
import re

exp = re.compile('debug', re.IGNORECASE)

def parse(log, line):
    if exp.search(line):
        statsd.increment("logs.ddagent.debugs")
