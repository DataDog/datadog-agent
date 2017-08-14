# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

## Common functions for tests ##

def assert_instance_init(check):
    # assertions on `instances`
    assert hasattr(check, 'instances')
    assert isinstance(check.instances, list)
    assert len(check.instances) > 0
    assert 'foo_instance' in check.instances[0]

def assert_agent_config_init(check, new_style=True):
    # assertions on `agentConfig`
    assert hasattr(check, 'agentConfig')
    assert isinstance(check.agentConfig, dict)

    if new_style:
        assert len(check.agentConfig) == 0
    else:
        # old style: the full agent config should be initialized
        assert 'foo_agent' in check.agentConfig
        assert 'bar_agent' == check.agentConfig['foo_agent']

def assert_init_config_init(check):
    # assertions on `init_config`
    assert hasattr(check, 'init_config')
    assert isinstance(check.init_config, dict)
    assert 'foo_init' in check.init_config
    assert 'bar_init' == check.init_config['foo_init']

