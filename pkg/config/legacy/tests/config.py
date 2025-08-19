# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# 3p
import json
import logging
import logging.config
import logging.handlers
import os
import re
import string
import sys
from socket import gaierror, gethostbyname
from urllib.request import getproxies

# stdlib
import ConfigParser
from cStringIO import StringIO
from urlparse import urlparse

# CONSTANTS
TRACE_CONFIG = 'trace_config'  # used for tracing config load by service discovery
CONFIG_FROM_FILE = 'YAML file'
AUTO_CONFIG_DIR = 'auto_conf/'
SD_BACKENDS = ['docker']
SD_CONFIG_BACKENDS = ['etcd', 'consul', 'zk']  # noqa: used somewhere else
AGENT_VERSION = "5.18.0"
JMX_VERSION = "0.16.0"
DATADOG_CONF = "datadog.conf"
UNIX_CONFIG_PATH = '/etc/dd-agent'
MAC_CONFIG_PATH = '/opt/datadog-agent/etc'
DEFAULT_CHECK_FREQUENCY = 15  # seconds
LOGGING_MAX_BYTES = 10 * 1024 * 1024
SDK_INTEGRATIONS_DIR = 'integrations'
SD_PIPE_NAME = "dd-service_discovery"
SD_PIPE_UNIX_PATH = '/opt/datadog-agent/run'
SD_PIPE_WIN_PATH = "\\\\.\\pipe\\{pipename}"
SD_TEMPLATE_DIR = "/datadog/check_configs"

log = logging.getLogger(__name__)

OLD_STYLE_PARAMETERS = [
    ('apache_status_url', "apache"),
    ('cacti_mysql_server', "cacti"),
    ('couchdb_server', "couchdb"),
    ('elasticsearch', "elasticsearch"),
    ('haproxy_url', "haproxy"),
    ('hudson_home', "Jenkins"),
    ('memcache_', "memcached"),
    ('mongodb_server', "mongodb"),
    ('mysql_server', "mysql"),
    ('nginx_status_url', "nginx"),
    ('postgresql_server', "postgres"),
    ('redis_urls', "redis"),
    ('varnishstat', "varnish"),
    ('WMI', "WMI"),
]

NAGIOS_OLD_CONF_KEYS = [
    'nagios_log',
    'nagios_perf_cfg',
]

LEGACY_DATADOG_URLS = [
    "app.datadoghq.com",
    "app.datad0g.com",
]

JMX_SD_CONF_TEMPLATE = '.jmx.{}.yaml'

# These are unlikely to change, but manifests are versioned,
# so keeping these as a list just in case we change add stuff.
MANIFEST_VALIDATION = {
    'max': ['max_agent_version'],
    'min': ['min_agent_version'],
}


class PathNotFound(Exception):
    pass


def get_version():
    return AGENT_VERSION


def skip_leading_wsp(f):
    "Works on a file, returns a file-like object"
    return StringIO("\n".join(map(string.strip, f.readlines())))


def _windows_commondata_path():
    """Return the common appdata path, using ctypes
    From http://stackoverflow.com/questions/626796/\
    how-do-i-find-the-windows-common-application-data-folder-using-python
    """
    import ctypes
    from ctypes import windll, wintypes

    CSIDL_COMMON_APPDATA = 35

    _SHGetFolderPath = windll.shell32.SHGetFolderPathW
    _SHGetFolderPath.argtypes = [
        wintypes.HWND,
        ctypes.c_int,
        wintypes.HANDLE,
        wintypes.DWORD,
        wintypes.LPCWSTR,
    ]

    path_buf = wintypes.create_unicode_buffer(wintypes.MAX_PATH)
    _SHGetFolderPath(0, CSIDL_COMMON_APPDATA, 0, 0, path_buf)
    return path_buf.value


def _windows_extra_checksd_path():
    common_data = _windows_commondata_path()
    return os.path.join(common_data, 'Datadog', 'checks.d')


def _windows_checksd_path():
    if hasattr(sys, 'frozen'):
        # we're frozen - from py2exe
        prog_path = os.path.dirname(sys.executable)
        return _checksd_path(os.path.normpath(os.path.join(prog_path, '..', 'agent')))
    else:
        cur_path = os.path.dirname(__file__)
        return _checksd_path(cur_path)


def _config_path(directory):
    path = os.path.join(directory, DATADOG_CONF)
    if os.path.exists(path):
        return path
    raise PathNotFound(path)


def _confd_path(directory):
    path = os.path.join(directory, 'conf.d')
    if os.path.exists(path):
        return path
    raise PathNotFound(path)


def _checksd_path(directory):
    path_override = os.environ.get('CHECKSD_OVERRIDE')
    if path_override and os.path.exists(path_override):
        return path_override

    # this is deprecated in testing on versions after SDK (5.12.0)
    path = os.path.join(directory, 'checks.d')
    if os.path.exists(path):
        return path
    raise PathNotFound(path)


def _is_affirmative(s):
    if s is None:
        return False
    # int or real bool
    if isinstance(s, int):
        return bool(s)
    # try string cast
    return s.lower() in ('yes', 'true', '1')


def get_config_path(cfg_path=""):
    return os.path.join(cfg_path, "datadog.conf")


def get_default_bind_host():
    try:
        gethostbyname('localhost')
    except gaierror:
        log.warning("localhost seems undefined in your hosts file, using 127.0.0.1 instead")
        return '127.0.0.1'
    return 'localhost'


def get_histogram_aggregates(configstr=None):
    if configstr is None:
        return None

    try:
        vals = configstr.split(',')
        valid_values = ['min', 'max', 'median', 'avg', 'sum', 'count']
        result = []

        for val in vals:
            val = val.strip()
            if val not in valid_values:
                log.warning("Ignored histogram aggregate %s, invalid", val)
                continue
            else:
                result.append(val)
    except Exception:
        log.exception("Error when parsing histogram aggregates, skipping")
        return None

    return result


def get_histogram_percentiles(configstr=None):
    if configstr is None:
        return None

    result = []
    try:
        vals = configstr.split(',')
        for val in vals:
            try:
                val = val.strip()
                floatval = float(val)
                if floatval <= 0 or floatval >= 1:
                    raise ValueError
                if len(val) > 4:
                    log.warning("Histogram percentiles are rounded to 2 digits: %s rounded", floatval)
                result.append(float(val[0:4]))
            except ValueError:
                log.warning("Bad histogram percentile value %s, must be float in ]0;1[, skipping", val)
    except Exception:
        log.exception("Error when parsing histogram percentiles, skipping")
        return None

    return result


def clean_dd_url(url):
    url = url.strip()
    if not url.startswith('http'):
        url = 'https://' + url
    return url[:-1] if url.endswith('/') else url


def remove_empty(string_array):
    return filter(lambda x: x, string_array)


def get_config(options=None):
    # General config
    agentConfig = {
        'check_freq': DEFAULT_CHECK_FREQUENCY,
        'collect_orchestrator_tags': True,
        'dogstatsd_port': 8125,
        'dogstatsd_target': 'http://localhost:17123',
        'graphite_listen_port': None,
        'hostname': None,
        'listen_port': None,
        'tags': None,
        'version': get_version(),
        'watchdog': True,
        'additional_checksd': '/etc/dd-agent/checks.d/',
        'bind_host': get_default_bind_host(),
        'statsd_metric_namespace': None,
        'utf8_decoding': False,
    }

    if "darwin" in sys.platform:
        agentConfig['additional_checksd'] = '/opt/datadog-agent/etc/checks.d'
    elif "win32" in sys.platform:
        agentConfig['additional_checksd'] = _windows_extra_checksd_path()

    # Config handling
    try:
        # Find the right config file
        path = os.path.realpath(__file__)
        path = os.path.dirname(path)

        config_path = get_config_path(path)
        config = ConfigParser.ConfigParser()
        config.readfp(skip_leading_wsp(open(config_path)))

        # bulk import
        for option in config.options('Main'):
            agentConfig[option] = config.get('Main', option)

        # Store developer mode setting in the agentConfig
        if config.has_option('Main', 'developer_mode'):
            agentConfig['developer_mode'] = _is_affirmative(config.get('Main', 'developer_mode'))

        # Allow an override with the --profile option
        if options is not None and options.profile:
            agentConfig['developer_mode'] = True

        # Core config
        # ap
        if not config.has_option('Main', 'api_key'):
            log.warning("No API key was found. Aborting.")
            sys.exit(2)

        if not config.has_option('Main', 'dd_url'):
            log.warning("No dd_url was found. Aborting.")
            sys.exit(2)

        # Endpoints
        dd_urls = map(clean_dd_url, config.get('Main', 'dd_url').split(','))
        api_keys = (el.strip() for el in config.get('Main', 'api_key').split(','))

        # For collector and dogstatsd
        agentConfig['dd_url'] = dd_urls[0]
        agentConfig['api_key'] = api_keys[0]

        # Forwarder endpoints logic
        # endpoints is:
        # {
        #    'https://app.datadoghq.com': ['api_key_abc', 'api_key_def'],
        #    'https://app.example.com': ['api_key_xyz']
        # }
        endpoints = {}
        dd_urls = remove_empty(dd_urls)
        api_keys = remove_empty(api_keys)
        if len(dd_urls) == 1:
            if len(api_keys) > 0:
                endpoints[dd_urls[0]] = api_keys
        else:
            assert len(dd_urls) == len(api_keys), 'Please provide one api_key for each url'
            for i, dd_url in enumerate(dd_urls):
                endpoints[dd_url] = endpoints.get(dd_url, []) + [api_keys[i]]

        agentConfig['endpoints'] = endpoints

        # Forwarder or not forwarder
        agentConfig['use_forwarder'] = options is not None and options.use_forwarder
        if agentConfig['use_forwarder']:
            listen_port = 17123
            if config.has_option('Main', 'listen_port'):
                listen_port = int(config.get('Main', 'listen_port'))
            bind_host = agentConfig['bind_host']
            agentConfig['dd_url'] = f"http://{bind_host}:{listen_port}"
        # FIXME: Legacy dd_url command line switch
        elif options is not None and options.dd_url is not None:
            agentConfig['dd_url'] = options.dd_url

        # Forwarder timeout
        agentConfig['forwarder_timeout'] = 20
        if config.has_option('Main', 'forwarder_timeout'):
            agentConfig['forwarder_timeout'] = int(config.get('Main', 'forwarder_timeout'))

        # Extra checks.d path
        # the linux directory is set by default
        if config.has_option('Main', 'additional_checksd'):
            agentConfig['additional_checksd'] = config.get('Main', 'additional_checksd')

        if config.has_option('Main', 'use_dogstatsd'):
            agentConfig['use_dogstatsd'] = config.get('Main', 'use_dogstatsd').lower() in ("yes", "true")
        else:
            agentConfig['use_dogstatsd'] = True

        # Service discovery
        if config.has_option('Main', 'service_discovery_backend'):
            try:
                additional_config = extract_agent_config(config)
                agentConfig.update(additional_config)
            except Exception:
                log.error('Failed to load the agent configuration related to service discovery. It will not be used.')

        # Concerns only Windows
        if config.has_option('Main', 'use_web_info_page'):
            agentConfig['use_web_info_page'] = config.get('Main', 'use_web_info_page').lower() in ("yes", "true")
        else:
            agentConfig['use_web_info_page'] = True

        # local traffic only? Default to no
        agentConfig['non_local_traffic'] = False
        if config.has_option('Main', 'non_local_traffic'):
            agentConfig['non_local_traffic'] = config.get('Main', 'non_local_traffic').lower() in ("yes", "true")

        # DEPRECATED
        if config.has_option('Main', 'use_ec2_instance_id'):
            use_ec2_instance_id = config.get('Main', 'use_ec2_instance_id')
            # translate yes into True, the rest into False
            agentConfig['use_ec2_instance_id'] = use_ec2_instance_id.lower() == 'yes'

        if config.has_option('Main', 'check_freq'):
            try:
                agentConfig['check_freq'] = int(config.get('Main', 'check_freq'))
            except Exception:
                pass

        # Custom histogram aggregate/percentile metrics
        if config.has_option('Main', 'histogram_aggregates'):
            agentConfig['histogram_aggregates'] = get_histogram_aggregates(config.get('Main', 'histogram_aggregates'))

        if config.has_option('Main', 'histogram_percentiles'):
            agentConfig['histogram_percentiles'] = get_histogram_percentiles(
                config.get('Main', 'histogram_percentiles')
            )

        # Disable Watchdog (optionally)
        if config.has_option('Main', 'watchdog'):
            if config.get('Main', 'watchdog').lower() in ('no', 'false'):
                agentConfig['watchdog'] = False

        # Optional graphite listener
        if config.has_option('Main', 'graphite_listen_port'):
            agentConfig['graphite_listen_port'] = int(config.get('Main', 'graphite_listen_port'))
        else:
            agentConfig['graphite_listen_port'] = None

        # Dogstatsd config
        dogstatsd_defaults = {
            'dogstatsd_port': 8125,
            'dogstatsd_target': 'http://' + agentConfig['bind_host'] + ':17123',
        }
        for key, value in dogstatsd_defaults.items():
            if config.has_option('Main', key):
                agentConfig[key] = config.get('Main', key)
            else:
                agentConfig[key] = value

        # Create app:xxx tags based on monitored apps
        agentConfig['create_dd_check_tags'] = config.has_option('Main', 'create_dd_check_tags') and _is_affirmative(
            config.get('Main', 'create_dd_check_tags')
        )

        # Forwarding to external statsd server
        if config.has_option('Main', 'statsd_forward_host'):
            agentConfig['statsd_forward_host'] = config.get('Main', 'statsd_forward_host')
            if config.has_option('Main', 'statsd_forward_port'):
                agentConfig['statsd_forward_port'] = int(config.get('Main', 'statsd_forward_port'))

        # Optional config
        # FIXME not the prettiest code ever...
        if config.has_option('Main', 'use_mount'):
            agentConfig['use_mount'] = _is_affirmative(config.get('Main', 'use_mount'))

        if options is not None and options.autorestart:
            agentConfig['autorestart'] = True
        elif config.has_option('Main', 'autorestart'):
            agentConfig['autorestart'] = _is_affirmative(config.get('Main', 'autorestart'))

        if config.has_option('Main', 'check_timings'):
            agentConfig['check_timings'] = _is_affirmative(config.get('Main', 'check_timings'))

        if config.has_option('Main', 'exclude_process_args'):
            agentConfig['exclude_process_args'] = _is_affirmative(config.get('Main', 'exclude_process_args'))

        try:
            filter_device_re = config.get('Main', 'device_blacklist_re')
            agentConfig['device_blacklist_re'] = re.compile(filter_device_re)
        except ConfigParser.NoOptionError:
            pass

        # Dogstream config
        if config.has_option("Main", "dogstream_log"):
            # Older version, single log support
            log_path = config.get("Main", "dogstream_log")
            if config.has_option("Main", "dogstream_line_parser"):
                agentConfig["dogstreams"] = ':'.join([log_path, config.get("Main", "dogstream_line_parser")])
            else:
                agentConfig["dogstreams"] = log_path

        elif config.has_option("Main", "dogstreams"):
            agentConfig["dogstreams"] = config.get("Main", "dogstreams")

        if config.has_option("Main", "nagios_perf_cfg"):
            agentConfig["nagios_perf_cfg"] = config.get("Main", "nagios_perf_cfg")

        if config.has_option("Main", "use_curl_http_client"):
            agentConfig["use_curl_http_client"] = _is_affirmative(config.get("Main", "use_curl_http_client"))
        else:
            # Default to False as there are some issues with the curl client and ELB
            agentConfig["use_curl_http_client"] = False

        if config.has_section('WMI'):
            agentConfig['WMI'] = {}
            for key, value in config.items('WMI'):
                agentConfig['WMI'][key] = value

        if config.has_option("Main", "skip_ssl_validation"):
            agentConfig["skip_ssl_validation"] = _is_affirmative(config.get("Main", "skip_ssl_validation"))

        agentConfig["collect_instance_metadata"] = True
        if config.has_option("Main", "collect_instance_metadata"):
            agentConfig["collect_instance_metadata"] = _is_affirmative(config.get("Main", "collect_instance_metadata"))

        agentConfig["proxy_forbid_method_switch"] = False
        if config.has_option("Main", "proxy_forbid_method_switch"):
            agentConfig["proxy_forbid_method_switch"] = _is_affirmative(
                config.get("Main", "proxy_forbid_method_switch")
            )

        agentConfig["collect_ec2_tags"] = False
        if config.has_option("Main", "collect_ec2_tags"):
            agentConfig["collect_ec2_tags"] = _is_affirmative(config.get("Main", "collect_ec2_tags"))

        agentConfig["collect_orchestrator_tags"] = True
        if config.has_option("Main", "collect_orchestrator_tags"):
            agentConfig["collect_orchestrator_tags"] = _is_affirmative(config.get("Main", "collect_orchestrator_tags"))

        agentConfig["utf8_decoding"] = False
        if config.has_option("Main", "utf8_decoding"):
            agentConfig["utf8_decoding"] = _is_affirmative(config.get("Main", "utf8_decoding"))

        agentConfig["gce_updated_hostname"] = False
        if config.has_option("Main", "gce_updated_hostname"):
            agentConfig["gce_updated_hostname"] = _is_affirmative(config.get("Main", "gce_updated_hostname"))

    except ConfigParser.NoSectionError:
        sys.stderr.write('Config file not found or incorrectly formatted.\n')
        sys.exit(2)

    except ConfigParser.ParsingError:
        sys.stderr.write('Config file not found or incorrectly formatted.\n')
        sys.exit(2)

    except ConfigParser.NoOptionError as e:
        sys.stderr.write(f'There are some items missing from your config file, but nothing fatal [{e}]')

    # Storing proxy settings in the agentConfig
    agentConfig['proxy_settings'] = get_proxy(agentConfig)
    if agentConfig.get('ca_certs', None) is None:
        agentConfig['ssl_certificate'] = 'datadog-cert.pem'
    else:
        agentConfig['ssl_certificate'] = agentConfig['ca_certs']

    return agentConfig


def extract_agent_config(config):
    # get merged into the real agentConfig
    agentConfig = {}

    backend = config.get('Main', 'service_discovery_backend')
    agentConfig['service_discovery'] = True

    conf_backend = None
    if config.has_option('Main', 'sd_config_backend'):
        conf_backend = config.get('Main', 'sd_config_backend')

    if backend not in SD_BACKENDS:
        log.error("The backend %s is not supported. Service discovery won't be enabled.", backend)
        agentConfig['service_discovery'] = False

    if conf_backend is None:
        log.warning('No configuration backend provided for service discovery. Only auto config templates will be used.')
    elif conf_backend not in SD_CONFIG_BACKENDS:
        log.error("The config backend %s is not supported. Only auto config templates will be used.", conf_backend)
        conf_backend = None
    agentConfig['sd_config_backend'] = conf_backend

    additional_config = extract_sd_config(config)
    agentConfig.update(additional_config)
    return agentConfig


def extract_sd_config(config):
    """Extract configuration about service discovery for the agent"""
    sd_config = {}
    if config.has_option('Main', 'sd_config_backend'):
        sd_config['sd_config_backend'] = config.get('Main', 'sd_config_backend')
    else:
        sd_config['sd_config_backend'] = None
    if config.has_option('Main', 'sd_template_dir'):
        sd_config['sd_template_dir'] = config.get('Main', 'sd_template_dir')
    else:
        sd_config['sd_template_dir'] = SD_TEMPLATE_DIR
    if config.has_option('Main', 'sd_backend_host'):
        sd_config['sd_backend_host'] = config.get('Main', 'sd_backend_host')
    if config.has_option('Main', 'sd_backend_port'):
        sd_config['sd_backend_port'] = config.get('Main', 'sd_backend_port')
    if config.has_option('Main', 'sd_jmx_enable'):
        sd_config['sd_jmx_enable'] = config.get('Main', 'sd_jmx_enable')
    return sd_config


def get_proxy(agentConfig):
    proxy_settings = {}

    # First we read the proxy configuration from datadog.conf
    proxy_host = agentConfig.get('proxy_host')
    if proxy_host is not None:
        proxy_settings['host'] = proxy_host
        try:
            proxy_settings['port'] = int(agentConfig.get('proxy_port', 3128))
        except ValueError:
            log.error('Proxy port must be an Integer. Defaulting it to 3128')
            proxy_settings['port'] = 3128

        proxy_settings['user'] = agentConfig.get('proxy_user')
        proxy_settings['password'] = agentConfig.get('proxy_password')
        log.debug(
            "Proxy Settings: %s:*****@%s:%s", proxy_settings['user'], proxy_settings['host'], proxy_settings['port']
        )
        return proxy_settings

    # If no proxy configuration was specified in datadog.conf
    # We try to read it from the system settings
    try:
        proxy = getproxies().get('https')
        if proxy is not None:
            parse = urlparse(proxy)
            proxy_settings['host'] = parse.hostname
            proxy_settings['port'] = int(parse.port)
            proxy_settings['user'] = parse.username
            proxy_settings['password'] = parse.password

            log.debug(
                "Proxy Settings: %s:*****@%s:%s", proxy_settings['user'], proxy_settings['host'], proxy_settings['port']
            )
            return proxy_settings

    except Exception as e:
        log.debug("Error while trying to fetch proxy settings using urllib %s. Proxy is probably not set", str(e))

    return None


def main():
    res = {}
    for k, v in get_config().items():
        res[k] = str(v)
    return json.dumps(res, sort_keys=True)


if __name__ == "__main__":
    print(main())
