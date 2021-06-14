import pytest


@pytest.fixture
def hostname(host):
    return host.ansible.get_variables()["inventory_hostname"]


@pytest.fixture
def vars_from(host):
    def _load_vars(yaml_path):
        return host.ansible("include_vars", yaml_path)["ansible_facts"]
    return _load_vars


@pytest.fixture
def ansible_var(host):
    def _retrieve_var(name):
        raw_vars = host.ansible.get_variables()
        if name in raw_vars and (type(raw_vars[name]) != str or "{{" not in raw_vars[name]):
            # No interpolation needed, we return the raw value
            return raw_vars[name]
        else:
            # This allows variable interpolation
            # https://stackoverflow.com/questions/57820998/accessing-ansible-variables-in-molecule-test-testinfra
            return host.ansible("debug", "msg={{ " + name + " }}")["msg"]
    return _retrieve_var
