# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# std
import glob
import os

# project
from config import (
    check_yaml,
    load_check_directory,
    get_confd_path
)
from utils.hostname import get_hostname
from utils.dockerutil import DockerUtil
from utils.service_discovery.config_stores import get_config_store, SD_CONFIG_BACKENDS, TRACE_CONFIG


def configcheck():
    all_valid = True
    for conf_path in glob.glob(os.path.join(get_confd_path(), "*.yaml")):
        basename = os.path.basename(conf_path)
        try:
            check_yaml(conf_path)
        except Exception as e:
            all_valid = False
            print "%s contains errors:\n    %s" % (basename, e)
        else:
            print "%s is valid" % basename
    if all_valid:
        print "All yaml files passed. You can now run the Datadog agent."
        return 0
    else:
        print("Fix the invalid yaml files above in order to start the Datadog agent. "
              "A useful external tool for yaml parsing can be found at "
              "http://yaml-online-parser.appspot.com/")
        return 1


def sd_configcheck(agentConfig):
    if agentConfig.get('service_discovery', False):
        # set the TRACE_CONFIG flag to True to make load_check_directory return
        # the source of config objects.
        # Then call load_check_directory here and pass the result to get_sd_configcheck
        # to avoid circular imports
        agentConfig[TRACE_CONFIG] = True
        configs = {
            # check_name: (config_source, config)
        }
        print("\nLoading check configurations...\n\n")
        configs = load_check_directory(agentConfig, get_hostname(agentConfig))
        get_sd_configcheck(agentConfig, configs)


def get_sd_configcheck(agentConfig, configs):
    """Trace how the configuration objects are loaded and from where.
        Also print containers detected by the agent and templates from the config store."""
    print("\nSource of the configuration objects built by the agent:\n")
    for check_name, config in configs.iteritems():
        print('Check "%s":\n  source --> %s\n  config --> %s\n' % (check_name, config[0], config[1]))

    try:
        print_containers()
    except Exception:
        print("Failed to collect containers info.")

    try:
        print_templates(agentConfig)
    except Exception:
        print("Failed to collect configuration templates.")


def print_containers():
    containers = DockerUtil().client.containers()
    print("\nContainers info:\n")
    print("Number of containers found: %s" % len(containers))
    for co in containers:
        c_id = 'ID: %s' % co.get('Id')[:12]
        c_image = 'image: %s' % co.get('Image')
        c_name = 'name: %s' % DockerUtil.container_name_extractor(co)[0]
        print("\t- %s %s %s" % (c_id, c_image, c_name))
    print('\n')


def print_templates(agentConfig):
    if agentConfig.get('sd_config_backend') in SD_CONFIG_BACKENDS:
        print("Configuration templates:\n")
        templates = {}
        sd_template_dir = agentConfig.get('sd_template_dir')
        config_store = get_config_store(agentConfig)
        try:
            templates = config_store.dump_directory(sd_template_dir)
        except Exception as ex:
            print("Failed to extract configuration templates from the backend:\n%s" % str(ex))

        for ident, tpl in templates.iteritems():
            print(
                "- Identifier %s:\n\tcheck names: %s\n\tinit_configs: %s\n\tinstances: %s" % (
                    ident,
                    tpl.get('check_names'),
                    tpl.get('init_configs'),
                    tpl.get('instances'),
                )
            )
