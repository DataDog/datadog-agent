import pytest


@pytest.fixture
def hostname(host):
    return host.ansible.get_variables()["inventory_hostname"]
