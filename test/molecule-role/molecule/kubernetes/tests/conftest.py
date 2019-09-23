import pytest


@pytest.fixture
def hostname(host):
    return host.ansible.get_variables()["inventory_hostname"]


@pytest.fixture
def common_vars(host):
    return host.ansible("include_vars", "./common_vars.yml")["ansible_facts"]
