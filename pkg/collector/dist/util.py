# stdlib
from hashlib import md5
import logging
import math
import os
import platform
import re
import signal
import socket
import sys
import time
import types
import urllib2
import uuid

# 3p
import simplejson as json
import yaml  # noqa, let's guess, probably imported somewhere
from tornado import ioloop
try:
    from yaml import CLoader as yLoader
    from yaml import CDumper as yDumper
except ImportError:
    # On source install C Extensions might have not been built
    from yaml import Loader as yLoader  # noqa, imported from here elsewhere
    from yaml import Dumper as yDumper  # noqa, imported from here elsewhere

# These classes are now in utils/, they are just here for compatibility reasons,
# if a user actually uses them in a custom check
# If you're this user, please use utils.pidfile or utils.platform instead
# FIXME: remove them at a point (6.x)
from utils.dockerutil import get_hostname as get_docker_hostname, is_dockerized
from utils.pidfile import PidFile  # noqa, see ^^^
from utils.platform import Platform
from utils.proxy import get_proxy
from utils.subprocess_output import get_subprocess_output


VALID_HOSTNAME_RFC_1123_PATTERN = re.compile(r"^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$")
MAX_HOSTNAME_LEN = 255
COLON_NON_WIN_PATH = re.compile(':(?!\\\\)')

log = logging.getLogger(__name__)

NumericTypes = (float, int, long)


def plural(count):
    if count == 1:
        return ""
    return "s"


def get_tornado_ioloop():
        return ioloop.IOLoop.current()


def get_uuid():
    # Generate a unique name that will stay constant between
    # invocations, such as platform.node() + uuid.getnode()
    # Use uuid5, which does not depend on the clock and is
    # recommended over uuid3.
    # This is important to be able to identify a server even if
    # its drives have been wiped clean.
    # Note that this is not foolproof but we can reconcile servers
    # on the back-end if need be, based on mac addresses.
    return uuid.uuid5(uuid.NAMESPACE_DNS, platform.node() + str(uuid.getnode())).hex


def get_os():
    "Human-friendly OS name"
    if sys.platform == 'darwin':
        return 'mac'
    elif sys.platform.find('freebsd') != -1:
        return 'freebsd'
    elif sys.platform.find('linux') != -1:
        return 'linux'
    elif sys.platform.find('win32') != -1:
        return 'windows'
    elif sys.platform.find('sunos') != -1:
        return 'solaris'
    else:
        return sys.platform


def headers(agentConfig):
    # Build the request headers
    return {
        'User-Agent': 'Datadog Agent/%s' % agentConfig['version'],
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'text/html, */*',
    }

def windows_friendly_colon_split(config_string):
    '''
    Perform a split by ':' on the config_string
    without splitting on the start of windows path
    '''
    if Platform.is_win32():
        # will split on path/to/module.py:blabla but not on C:\\path
        return COLON_NON_WIN_PATH.split(config_string)
    else:
        return config_string.split(':')

def getTopIndex():
    macV = None
    if sys.platform == 'darwin':
        macV = platform.mac_ver()

    # Output from top is slightly modified on OS X 10.6 (case #28239)
    if macV and macV[0].startswith('10.6.'):
        return 6
    else:
        return 5


def isnan(val):
    if hasattr(math, 'isnan'):
        return math.isnan(val)

    # for py < 2.6, use a different check
    # http://stackoverflow.com/questions/944700/how-to-check-for-nan-in-python
    return str(val) == str(1e400*0)


def cast_metric_val(val):
    # ensure that the metric value is a numeric type
    if not isinstance(val, NumericTypes):
        # Try the int conversion first because want to preserve
        # whether the value is an int or a float. If neither work,
        # raise a ValueError to be handled elsewhere
        for cast in [int, float]:
            try:
                val = cast(val)
                return val
            except ValueError:
                continue
        raise ValueError
    return val

_IDS = {}
def get_next_id(name):
    global _IDS
    current_id = _IDS.get(name, 0)
    current_id += 1
    _IDS[name] = current_id
    return current_id

def is_valid_hostname(hostname):
    if hostname.lower() in set([
        'localhost',
        'localhost.localdomain',
        'localhost6.localdomain6',
        'ip6-localhost',
    ]):
        log.warning("Hostname: %s is local" % hostname)
        return False
    if len(hostname) > MAX_HOSTNAME_LEN:
        log.warning("Hostname: %s is too long (max length is  %s characters)" % (hostname, MAX_HOSTNAME_LEN))
        return False
    if VALID_HOSTNAME_RFC_1123_PATTERN.match(hostname) is None:
        log.warning("Hostname: %s is not complying with RFC 1123" % hostname)
        return False
    return True


def get_hostname(config=None):
    """
    Get the canonical host name this agent should identify as. This is
    the authoritative source of the host name for the agent.

    Tries, in order:

      * agent config (datadog.conf, "hostname:")
      * 'hostname -f' (on unix)
      * socket.gethostname()
    """
    hostname = None

    # first, try the config
    if config is None:
        from config import get_config
        config = get_config(parse_args=True)
    config_hostname = config.get('hostname')
    if config_hostname and is_valid_hostname(config_hostname):
        return config_hostname

    # Try to get GCE instance name
    if hostname is None:
        gce_hostname = GCE.get_hostname(config)
        if gce_hostname is not None:
            if is_valid_hostname(gce_hostname):
                return gce_hostname

    # Try to get the docker hostname
    if hostname is None and is_dockerized():
        docker_hostname = get_docker_hostname()
        if docker_hostname is not None and is_valid_hostname(docker_hostname):
            return docker_hostname

    # then move on to os-specific detection
    if hostname is None:
        def _get_hostname_unix():
            try:
                # try fqdn
                out, _, rtcode = get_subprocess_output(['/bin/hostname', '-f'], log)
                if rtcode == 0:
                    return out.strip()
            except Exception:
                return None

        os_name = get_os()
        if os_name in ['mac', 'freebsd', 'linux', 'solaris']:
            unix_hostname = _get_hostname_unix()
            if unix_hostname and is_valid_hostname(unix_hostname):
                hostname = unix_hostname

    # if we have an ec2 default hostname, see if there's an instance-id available
    if (Platform.is_ecs_instance()) or (hostname is not None and True in [hostname.lower().startswith(p) for p in [u'ip-', u'domu']]):
        instanceid = EC2.get_instance_id(config)
        if instanceid:
            hostname = instanceid

    # fall back on socket.gethostname(), socket.getfqdn() is too unreliable
    if hostname is None:
        try:
            socket_hostname = socket.gethostname()
        except socket.error:
            socket_hostname = None
        if socket_hostname and is_valid_hostname(socket_hostname):
            hostname = socket_hostname

    if hostname is None:
        log.critical('Unable to reliably determine host name. You can define one in datadog.conf or in your hosts file')
        raise Exception('Unable to reliably determine host name. You can define one in datadog.conf or in your hosts file')
    else:
        return hostname

class GCE(object):
    URL = "http://169.254.169.254/computeMetadata/v1/?recursive=true"
    TIMEOUT = 0.1 # second
    SOURCE_TYPE_NAME = 'google cloud platform'
    metadata = None
    EXCLUDED_ATTRIBUTES = ["kube-env", "startup-script", "sshKeys", "user-data",
    "cli-cert", "ipsec-cert", "ssl-cert"]


    @staticmethod
    def _get_metadata(agentConfig):
        if GCE.metadata is not None:
            return GCE.metadata

        if not agentConfig['collect_instance_metadata']:
            log.info("Instance metadata collection is disabled. Not collecting it.")
            GCE.metadata = {}
            return GCE.metadata

        socket_to = None
        try:
            socket_to = socket.getdefaulttimeout()
            socket.setdefaulttimeout(GCE.TIMEOUT)
        except Exception:
            pass

        try:
            opener = urllib2.build_opener()
            opener.addheaders = [('X-Google-Metadata-Request','True')]
            GCE.metadata = json.loads(opener.open(GCE.URL).read().strip())

        except Exception:
            GCE.metadata = {}

        try:
            if socket_to is None:
                socket_to = 3
            socket.setdefaulttimeout(socket_to)
        except Exception:
            pass
        return GCE.metadata



    @staticmethod
    def get_tags(agentConfig):
        if not agentConfig['collect_instance_metadata']:
            return None

        try:
            host_metadata = GCE._get_metadata(agentConfig)
            tags = []

            for key, value in host_metadata['instance'].get('attributes', {}).iteritems():
                if key in GCE.EXCLUDED_ATTRIBUTES:
                    continue
                tags.append("%s:%s" % (key, value))

            tags.extend(host_metadata['instance'].get('tags', []))
            tags.append('zone:%s' % host_metadata['instance']['zone'].split('/')[-1])
            tags.append('instance-type:%s' % host_metadata['instance']['machineType'].split('/')[-1])
            tags.append('internal-hostname:%s' % host_metadata['instance']['hostname'])
            tags.append('instance-id:%s' % host_metadata['instance']['id'])
            tags.append('project:%s' % host_metadata['project']['projectId'])
            tags.append('numeric_project_id:%s' % host_metadata['project']['numericProjectId'])

            GCE.metadata['hostname'] = host_metadata['instance']['hostname'].split('.')[0]

            return tags
        except Exception:
            return None

    @staticmethod
    def get_hostname(agentConfig):
        try:
            host_metadata = GCE._get_metadata(agentConfig)
            hostname = host_metadata['instance']['hostname']
            if agentConfig.get('gce_updated_hostname'):
                return hostname
            else:
                return hostname.split('.')[0]
        except Exception:
            return None

    @staticmethod
    def get_host_aliases(agentConfig):
        try:
            host_metadata = GCE._get_metadata(agentConfig)
            project_id = host_metadata['project']['projectId']
            instance_name = host_metadata['instance']['hostname'].split('.')[0]
            return ['%s.%s' % (instance_name, project_id)]
        except Exception:
            return None


class EC2(object):
    """Retrieve EC2 metadata
    """
    EC2_METADATA_HOST = "http://169.254.169.254"
    METADATA_URL_BASE = EC2_METADATA_HOST + "/latest/meta-data"
    INSTANCE_IDENTITY_URL = EC2_METADATA_HOST + "/latest/dynamic/instance-identity/document"
    TIMEOUT = 0.1  # second
    metadata = {}

    @staticmethod
    def get_tags(agentConfig):
        if not agentConfig['collect_instance_metadata']:
            log.info("Instance metadata collection is disabled. Not collecting it.")
            return []

        socket_to = None
        try:
            socket_to = socket.getdefaulttimeout()
            socket.setdefaulttimeout(EC2.TIMEOUT)
        except Exception:
            pass

        try:
            iam_role = urllib2.urlopen(EC2.METADATA_URL_BASE + "/iam/security-credentials/").read().strip()
            iam_params = json.loads(urllib2.urlopen(EC2.METADATA_URL_BASE + "/iam/security-credentials/" + unicode(iam_role)).read().strip())
            instance_identity = json.loads(urllib2.urlopen(EC2.INSTANCE_IDENTITY_URL).read().strip())
            region = instance_identity['region']

            import boto.ec2
            proxy_settings = get_proxy(agentConfig) or {}
            connection = boto.ec2.connect_to_region(
                region,
                aws_access_key_id=iam_params['AccessKeyId'],
                aws_secret_access_key=iam_params['SecretAccessKey'],
                security_token=iam_params['Token'],
                proxy=proxy_settings.get('host'), proxy_port=proxy_settings.get('port'),
                proxy_user=proxy_settings.get('user'), proxy_pass=proxy_settings.get('password')
            )

            tag_object = connection.get_all_tags({'resource-id': EC2.metadata['instance-id']})

            EC2_tags = [u"%s:%s" % (tag.name, tag.value) for tag in tag_object]
            if agentConfig.get('collect_security_groups') and EC2.metadata.get('security-groups'):
                EC2_tags.append(u"security-group-name:{0}".format(EC2.metadata.get('security-groups')))

        except Exception:
            log.exception("Problem retrieving custom EC2 tags")
            EC2_tags = []

        try:
            if socket_to is None:
                socket_to = 3
            socket.setdefaulttimeout(socket_to)
        except Exception:
            pass

        return EC2_tags

    @staticmethod
    def get_metadata(agentConfig):
        """Use the ec2 http service to introspect the instance. This adds latency if not running on EC2
        """
        # >>> import urllib2
        # >>> urllib2.urlopen('http://169.254.169.254/latest/', timeout=1).read()
        # 'meta-data\nuser-data'
        # >>> urllib2.urlopen('http://169.254.169.254/latest/meta-data', timeout=1).read()
        # 'ami-id\nami-launch-index\nami-manifest-path\nhostname\ninstance-id\nlocal-ipv4\npublic-keys/\nreservation-id\nsecurity-groups'
        # >>> urllib2.urlopen('http://169.254.169.254/latest/meta-data/instance-id', timeout=1).read()
        # 'i-deadbeef'

        # Every call may add TIMEOUT seconds in latency so don't abuse this call
        # python 2.4 does not support an explicit timeout argument so force it here
        # Rather than monkey-patching urllib2, just lower the timeout globally for these calls

        if not agentConfig['collect_instance_metadata']:
            log.info("Instance metadata collection is disabled. Not collecting it.")
            return {}

        socket_to = None
        try:
            socket_to = socket.getdefaulttimeout()
            socket.setdefaulttimeout(EC2.TIMEOUT)
        except Exception:
            pass

        for k in ('instance-id', 'hostname', 'local-hostname', 'public-hostname', 'ami-id', 'local-ipv4', 'public-keys/', 'public-ipv4', 'reservation-id', 'security-groups'):
            try:
                v = urllib2.urlopen(EC2.METADATA_URL_BASE + "/" + unicode(k)).read().strip()
                assert type(v) in (types.StringType, types.UnicodeType) and len(v) > 0, "%s is not a string" % v
                EC2.metadata[k.rstrip('/')] = v
            except Exception:
                pass

        try:
            if socket_to is None:
                socket_to = 3
            socket.setdefaulttimeout(socket_to)
        except Exception:
            pass

        return EC2.metadata

    @staticmethod
    def get_instance_id(agentConfig):
        try:
            return EC2.get_metadata(agentConfig).get("instance-id", None)
        except Exception:
            return None


class Watchdog(object):
    """Simple signal-based watchdog that will scuttle the current process
    if it has not been reset every N seconds, or if the processes exceeds
    a specified memory threshold.
    Can only be invoked once per process, so don't use with multiple threads.
    If you instantiate more than one, you're also asking for trouble.
    """
    def __init__(self, duration, max_mem_mb = None):
        import resource

        #Set the duration
        self._duration = int(duration)
        signal.signal(signal.SIGALRM, Watchdog.self_destruct)

        # cap memory usage
        if max_mem_mb is not None:
            self._max_mem_kb = 1024 * max_mem_mb
            max_mem_bytes = 1024 * self._max_mem_kb
            resource.setrlimit(resource.RLIMIT_AS, (max_mem_bytes, max_mem_bytes))
            self.memory_limit_enabled = True
        else:
            self.memory_limit_enabled = False

    @staticmethod
    def self_destruct(signum, frame):
        try:
            import traceback
            log.error("Self-destructing...")
            log.error(traceback.format_exc())
        finally:
            os.kill(os.getpid(), signal.SIGKILL)


    def reset(self):
        # self destruct if using too much memory, as tornado will swallow MemoryErrors
        if self.memory_limit_enabled:
            mem_usage_kb = int(os.popen('ps -p %d -o %s | tail -1' % (os.getpid(), 'rss')).read())
            if mem_usage_kb > (0.95 * self._max_mem_kb):
                Watchdog.self_destruct(signal.SIGKILL, sys._getframe(0))

        log.debug("Resetting watchdog for %d" % self._duration)
        signal.alarm(self._duration)


class LaconicFilter(logging.Filter):
    """
    Filters messages, only print them once while keeping memory under control
    """
    LACONIC_MEM_LIMIT = 1024

    def __init__(self, name=""):
        logging.Filter.__init__(self, name)
        self.hashed_messages = {}

    def hash(self, msg):
        return md5(msg).hexdigest()

    def filter(self, record):
        try:
            h = self.hash(record.getMessage())
            if h in self.hashed_messages:
                return 0
            else:
                # Don't blow up our memory
                if len(self.hashed_messages) >= LaconicFilter.LACONIC_MEM_LIMIT:
                    self.hashed_messages.clear()
                self.hashed_messages[h] = True
                return 1
        except Exception:
            return 1

class Timer(object):
    """ Helper class """

    def __init__(self):
        self.start()

    def _now(self):
        return time.time()

    def start(self):
        self.started = self._now()
        self.last = self.started
        return self

    def step(self):
        now = self._now()
        step = now - self.last
        self.last = now
        return step

    def total(self, as_sec=True):
        return self._now() - self.started

"""
Iterable Recipes
"""

def chunks(iterable, chunk_size):
    """Generate sequences of `chunk_size` elements from `iterable`."""
    iterable = iter(iterable)
    while True:
        chunk = [None] * chunk_size
        count = 0
        try:
            for _ in range(chunk_size):
                chunk[count] = iterable.next()
                count += 1
            yield chunk[:count]
        except StopIteration:
            if count:
                yield chunk[:count]
            break
