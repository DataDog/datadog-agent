# stdlib
from pprint import pprint
import inspect
import os
import sys

# datadog
from config import get_checksd_path, get_confd_path
from util import get_os


def run_check(name, path=None):
    """
    Test custom checks on Windows.
    """
    # Read the config file

    confd_path = path or os.path.join(get_confd_path(get_os()), '%s.yaml' % name)

    try:
        f = open(confd_path)
    except IOError:
        raise Exception('Unable to open configuration at %s' % confd_path)

    config_str = f.read()
    f.close()

    # Run the check
    check, instances = get_check(name, config_str)
    if not instances:
        raise Exception('YAML configuration returned no instances.')
    for instance in instances:
        check.check(instance)
        if check.has_events():
            print "Events:\n"
            pprint(check.get_events(), indent=4)
        print "Metrics:\n"
        pprint(check.get_metrics(), indent=4)


def get_check(name, config_str):
    from checks import AgentCheck

    checksd_path = get_checksd_path(get_os())
    if checksd_path not in sys.path:
        sys.path.append(checksd_path)
    check_module = __import__(name)
    check_class = None
    classes = inspect.getmembers(check_module, inspect.isclass)
    for name, clsmember in classes:
        if AgentCheck in clsmember.__bases__:
            check_class = clsmember
            break
    if check_class is None:
        raise Exception("Unable to import check %s. Missing a class that inherits AgentCheck" % name)

    agentConfig = {
        'version': '0.1',
        'api_key': 'tota'
    }

    return check_class.from_yaml(yaml_text=config_str, check_name=name,
                                 agentConfig=agentConfig)
