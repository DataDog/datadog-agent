# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# std
import logging
import simplejson as json
from collections import defaultdict
from copy import deepcopy
from os import path

# 3p
from requests.packages.urllib3.exceptions import TimeoutError

# project
from utils.checkfiles import get_check_class, get_auto_conf, get_auto_conf_images
from utils.singleton import Singleton


log = logging.getLogger(__name__)

CONFIG_FROM_AUTOCONF = 'auto-configuration'
CONFIG_FROM_FILE = 'YAML file'
CONFIG_FROM_TEMPLATE = 'template'
CONFIG_FROM_KUBE = 'Kubernetes Pod Annotation'
TRACE_CONFIG = 'trace_config'  # used for tracing config load by service discovery
CHECK_NAMES = 'check_names'
INIT_CONFIGS = 'init_configs'
INSTANCES = 'instances'
KUBE_ANNOTATIONS = 'kube_annotations'
KUBE_CONTAINER_NAME = 'kube_container_name'
KUBE_ANNOTATION_PREFIX = 'service-discovery.datadoghq.com'


class KeyNotFound(Exception):
    pass


class _TemplateCache(object):
    """
    Store templates coming from the configuration store and files from auto_conf.

    Templates from different sources are stored in separate attributes, and
    reads will look up identifiers in both of them in the right order.

    read_func is expected to return raw templates coming from the config store.

    The cache must be invalidated when an update is made to templates.
    """

    def __init__(self, read_func, root_template_path):
        self.read_func = read_func
        self.root_path = root_template_path
        self.kv_templates = defaultdict(lambda: [[]] * 3)
        self.auto_conf_templates = defaultdict(lambda: [[]] * 3)
        self._populate_auto_conf()

    def invalidate(self):
        """Clear out the KV cache"""
        log.debug("Clearing the cache for configuration templates.")
        self.kv_templates = defaultdict(lambda: [[]] * 3)

    def _populate_auto_conf(self):
        """Retrieve auto_conf templates"""
        raw_templates = get_auto_conf_images(full_tpl=True)
        for image, tpls in raw_templates.iteritems():
            for check_name, init_tpl, instance_tpl in zip(*tpls):
                if image in self.auto_conf_templates:
                    if check_name in self.auto_conf_templates[image][0]:
                        log.warning("Conflicting templates in auto_conf for image %s and check %s. "
                                "Please check your template files." % (image, check_name))
                        continue
                    self.auto_conf_templates[image][0].append(check_name)
                    self.auto_conf_templates[image][1].append(init_tpl)
                    self.auto_conf_templates[image][2].append(instance_tpl)
                else:
                    self.auto_conf_templates[image][0] = [check_name]
                    self.auto_conf_templates[image][1] = [init_tpl or {}]
                    # no list wrapping because auto_conf files already have a list of instances
                    self.auto_conf_templates[image][2] = instance_tpl or [{}]

    def _issue_read(self, identifier):
        """Perform a read against the KV store"""

        # templates from the config store
        try:
            check_names = json.loads(
                self.read_func(path.join(self.root_path, identifier, CHECK_NAMES).lstrip('/')))
            init_config_tpls = json.loads(
                self.read_func(path.join(self.root_path, identifier, INIT_CONFIGS).lstrip('/')))
            instance_tpls = json.loads(
                self.read_func(path.join(self.root_path, identifier, INSTANCES).lstrip('/')))
            return [check_names, init_config_tpls, instance_tpls]
        except KeyNotFound:
            return None

    def get_templates(self, identifier):
        """
        Return a dict of templates coming from the config store and
        the auto_conf folder and their source for a given identifier.
        Templates from kv_templates take precedence.
        """
        templates = {
            # source: [[check_names], [init_configs], [instances]]
            CONFIG_FROM_TEMPLATE: None,
            CONFIG_FROM_AUTOCONF: None
        }

        # cache miss
        if identifier not in self.kv_templates:
            try:
                tpls = self._issue_read(identifier)
            except NotImplementedError:
                # expected when get_check_names is called in auto-conf mode
                tpls = None
            except Exception:
                tpls = None
                log.exception('Failed to retrieve a template for %s.' % identifier)
            # create a key in the cache even if _issue_read doesn't return a tpl
            # so that subsequent reads don't trigger issue_read
            self.kv_templates[identifier] = tpls

        templates[CONFIG_FROM_TEMPLATE] = deepcopy(self.kv_templates[identifier])

        if identifier in self.auto_conf_templates:
            auto_conf_tpls = [[], [], []]
            unfiltered_tpls = self.auto_conf_templates[identifier]

            # add auto_conf templates only if the same check is
            # not already configured by a user-provided template.
            for idx, check_name in enumerate(unfiltered_tpls[0]):
                if not templates[CONFIG_FROM_TEMPLATE] or \
                        check_name not in templates[CONFIG_FROM_TEMPLATE][0]:
                    auto_conf_tpls[0].append(check_name)
                    auto_conf_tpls[1].append(unfiltered_tpls[1][idx])
                    auto_conf_tpls[2].append(unfiltered_tpls[2][idx])

            templates[CONFIG_FROM_AUTOCONF] = deepcopy(auto_conf_tpls)

        if templates[CONFIG_FROM_TEMPLATE] or templates[CONFIG_FROM_AUTOCONF]:
            return templates

        return None

    def get_check_names(self, identifier):
        """Return a set of all check names associated with an identifier"""
        check_names = set()

        # cache miss
        if identifier not in self.kv_templates and identifier not in self.auto_conf_templates:
            tpls = self.get_templates(identifier)

            if not tpls:
                return check_names

            auto_conf = tpls[CONFIG_FROM_AUTOCONF]
            if auto_conf:
                check_names.update(auto_conf[0])

            kv_conf = tpls[CONFIG_FROM_TEMPLATE]
            if kv_conf:
                check_names.update(kv_conf[0])

        if identifier in self.kv_templates and self.kv_templates[identifier]:
            check_names.update(set(self.kv_templates[identifier][0]))

        if identifier in self.auto_conf_templates and self.auto_conf_templates[identifier]:
            check_names.update(set(self.auto_conf_templates[identifier][0]))

        return check_names



class AbstractConfigStore(object):
    """Singleton for config stores"""
    __metaclass__ = Singleton

    previous_config_index = None

    def __init__(self, agentConfig):
        self.client = None
        self.agentConfig = agentConfig
        self.settings = self._extract_settings(agentConfig)
        self.client = self.get_client()
        self.sd_template_dir = agentConfig.get('sd_template_dir')
        self.auto_conf_images = get_auto_conf_images()

        # this cache is used to determine which check to
        # reload based on the image linked to a docker event
        #
        # it is invalidated entirely when a change is detected in the config store
        self.template_cache = _TemplateCache(self.client_read, self.sd_template_dir)

    @classmethod
    def _drop(cls):
        """Drop the config store instance. This is only used for testing."""
        if cls in cls._instances:
            del cls._instances[cls]

    def _extract_settings(self, config):
        raise NotImplementedError()

    def get_client(self, reset=False):
        raise NotImplementedError()

    def client_read(self, path, **kwargs):
        raise NotImplementedError()

    def dump_directory(self, path, **kwargs):
        raise NotImplementedError()

    def _get_kube_config(self, identifier, kube_annotations, kube_container_name):
        try:
            prefix = '{}/{}.'.format(KUBE_ANNOTATION_PREFIX, kube_container_name)
            check_names = json.loads(kube_annotations[prefix + CHECK_NAMES])
            init_config_tpls = json.loads(kube_annotations[prefix + INIT_CONFIGS])
            instance_tpls = json.loads(kube_annotations[prefix + INSTANCES])
            return [check_names, init_config_tpls, instance_tpls]
        except KeyError:
            return None
        except json.JSONDecodeError:
            log.exception('Could not decode the JSON configuration template '
                          'for the kubernetes pod with ident %s...' % identifier)
            return None

    def _get_auto_config(self, image_name):
        from jmxfetch import get_jmx_checks

        jmx_checknames = get_jmx_checks(auto_conf=True)

        ident = self._get_image_ident(image_name)
        templates = []
        if ident in self.auto_conf_images:
            check_names = self.auto_conf_images[ident]

            for check_name in check_names:
                # get the check class to verify it matches
                check = get_check_class(self.agentConfig, check_name) if check_name not in jmx_checknames else True
                if check is None:
                    log.info("Failed auto configuring check %s for %s." % (check_name, image_name))
                    continue
                auto_conf = get_auto_conf(check_name)
                init_config, instances = auto_conf.get('init_config', {}), auto_conf.get('instances', [])
                templates.append((check_name, init_config, instances[0] or {}))

        return templates

    def get_checks_to_refresh(self, identifier, **kwargs):
        to_check = set()

        # try from the cache
        to_check.update(self.template_cache.get_check_names(identifier))

        kube_annotations = kwargs.get(KUBE_ANNOTATIONS)
        kube_container_name = kwargs.get(KUBE_CONTAINER_NAME)

        # then from annotations
        if kube_annotations:
            kube_config = self._get_kube_config(identifier, kube_annotations, kube_container_name)
            if kube_config is not None:
                to_check.update(kube_config[0])

        # lastly, try with legacy name for auto-conf
        to_check.update(self.template_cache.get_check_names(self._get_image_ident(identifier)))

        return to_check

    def get_check_tpls(self, identifier, **kwargs):
        """Retrieve template configs for an identifier from the config_store or auto configuration."""

        # this flag is used when no valid configuration store was provided
        # it makes the method skip directly to the auto_conf
        if kwargs.get('auto_conf') is True:
            # When not using a configuration store on kubernetes, check the pod
            # annotations for configs before falling back to autoconf.
            kube_annotations = kwargs.get(KUBE_ANNOTATIONS)
            kube_container_name = kwargs.get(KUBE_CONTAINER_NAME)
            if kube_annotations:
                kube_config = self._get_kube_config(identifier, kube_annotations, kube_container_name)
                if kube_config is not None:
                    check_names, init_config_tpls, instance_tpls = kube_config
                    source = CONFIG_FROM_KUBE
                    return [(source, vs)
                            for vs in zip(check_names, init_config_tpls, instance_tpls)]

            # in auto config mode, identifier is the image name
            auto_config = self._get_auto_config(identifier)
            if auto_config:
                source = CONFIG_FROM_AUTOCONF
                return [(source, conf) for conf in auto_config]
            else:
                log.debug('No auto config was found for image %s, leaving it alone.' % identifier)
                return []
        else:
            configs = self.read_config_from_store(identifier)

            if not configs:
                return []

        res = []

        for source, config in configs.iteritems():
            if not config:
                continue

            check_names, init_config_tpls, instance_tpls = config
            if len(check_names) != len(init_config_tpls) or len(check_names) != len(instance_tpls):
                log.error('Malformed configuration template: check_names, init_configs '
                          'and instances are not all the same length. Container with identifier {} '
                          'will not be configured by the service discovery'.format(identifier))
                continue

            res += [(source, values)
                for values in zip(check_names, init_config_tpls, instance_tpls)]

        return res

    def read_config_from_store(self, identifier):
        """Query templates from the cache. Fallback to canonical identifier for auto-config."""
        try:
            res = self.template_cache.get_templates(identifier)

            if not res:
                log.debug("No template found for {}, trying with auto-config...".format(identifier))
                image_ident = self._get_image_ident(identifier)
                res = self.template_cache.get_templates(image_ident)

                if not res:
                    # at this point no check is considered applicable to this identifier.
                    return []

        except Exception as ex:
            log.debug(
                'No config template found for {0}. Error: {1}'.format(identifier, str(ex)))
            return []

        return res

    def _get_image_ident(self, ident):
        """Extract an identifier from the image"""
        # handle the 'redis@sha256:...' format
        if '@' in ident:
            return ident.split('@')[0].split('/')[-1]
        # if a custom image store is used there can be a port which adds a colon
        elif ident.count(':') > 1:
            return ident.split(':')[1].split('/')[-1]
        # otherwise we just strip the tag and keep the image name
        else:
            return ident.split(':')[0].split('/')[-1]

    def crawl_config_template(self):
        """Return whether or not configuration templates have changed since the previous crawl"""
        try:
            config_index = self.client_read(self.sd_template_dir.lstrip('/'), recursive=True, watch=True)
        except KeyNotFound:
            log.debug('No config template found (expected if running on auto-config alone).'
                      ' Not Triggering a config reload.')
            return False
        except TimeoutError:
            msg = 'Request for the configuration template timed out.'
            raise Exception(msg)
        # Initialize the config index reference
        if self.previous_config_index is None:
            self.previous_config_index = config_index
            return False
        # Config has been modified since last crawl
        # in this case a full config reload is triggered and the identifier_to_checks cache is rebuilt
        if config_index != self.previous_config_index:
            log.info('Detected an update in config templates, reloading check configs...')
            self.previous_config_index = config_index
            self.template_cache.invalidate()
            return True
        return False
