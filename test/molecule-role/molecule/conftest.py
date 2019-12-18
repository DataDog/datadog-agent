import pytest


@pytest.fixture
def hostname(host):
    return host.ansible.get_variables()["inventory_hostname"]


@pytest.fixture
def common_vars(host):
    return host.ansible("include_vars", "./common_vars.yml")["ansible_facts"]


@pytest.fixture
def ansible_var(host):
    def _debug_var(name):
        # This allows variable interpolation
        # https://stackoverflow.com/questions/57820998/accessing-ansible-variables-in-molecule-test-testinfra
        return host.ansible("debug", "msg={{ " + name + " }}")["msg"]
    return _debug_var
