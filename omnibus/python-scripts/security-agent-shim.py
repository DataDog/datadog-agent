import os
import argparse
import re
import time

def config_re_match(field: str, config: str):
    pattern = f"^\\s*{field}\\s*:"
    return re.search(pattern, config, re.MULTILINE)

def is_runtime_security_config_enabled(core_config: str, sysprobe_config: str):
    if os.environ.get('DD_RUNTIME_SECURITY_CONFIG_ENABLED', 'false').lower() == 'true':
        return True
    if config_re_match("runtime_security_config", core_config):
        return True
    if config_re_match("runtime_security_config", sysprobe_config):
        return True
    return False

def is_compliance_config_enabled(core_config: str):
    if os.environ.get('DD_COMPLIANCE_CONFIG_ENABLED', 'false').lower() == 'true':
        return True
    if config_re_match("compliance_config", core_config):
        return True
    return False

parser = argparse.ArgumentParser(prog='security-agent shim')
parser.add_argument('-c', '--cfgpath', default='/etc/datadog-agent/datadog.yaml')
parser.add_argument('--sysprobe-config', default='/etc/datadog-agent/system-probe.yaml')

args = parser.parse_args()

with open(args.cfgpath) as f:
    core_config = f.read()
with open(args.sysprobe_config) as f:
    sysprobe_config = f.read()

if is_runtime_security_config_enabled(core_config, sysprobe_config) or is_compliance_config_enabled(core_config):
    os.execlp("security-agent", "security-agent", "-c", args.cfgpath, "--sysprobe-config", args.sysprobe_config)

# a sleep is necessary so that sysV doesn't think the agent has failed
# to startup because of an error. Only applies on Debian 7.
time.sleep(5)
