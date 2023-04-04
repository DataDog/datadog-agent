from test_builder import Platform


def confDir(config):
    if config.platform == Platform.linux:
        return "in `/etc/datadog-agent/conf.d/qa.d/conf.yaml`"

    if config.platform == Platform.mac:
        return "in `~/.datadog-agent/conf.d/qa.d/conf.yaml`"

    if config.platform == Platform.windows:
        return "in `C:\\programdata\\datadog\\conf.d\\qa.d\\conf.yaml` (you may need to enable showing hidden files to see `c:\\programdata`):"


def filePositionSharedSteps():
    return """
Repeat the steps with:
- `start_position: end` (with an existing `registry.json` so it picks up where it left off) - should tail from the cursor in registry
- `start_position: beginning` (with an existing `registry.json` so it picks up where it left off) - should tail from the cursor in registry
- `start_position: end` and delete the `registry.json` first - should start from the end
- `start_position: beginning` and delete the `registry.json` first - should start from the beginning
- `start_position: forceBeginning` (with an existing `registry.json` so it picks up where it left off) - should tail from the beginning, ignoring the cursor from the registry
- `start_position: forceEnd` (with an existing `registry.json` so it picks up where it left off) - should tail from the end, ignoring the cursor from the registry
"""
