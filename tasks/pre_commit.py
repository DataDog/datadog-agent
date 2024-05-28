DEFAULT_PRE_COMMIT_CONFIG = ".pre-commit-config.yaml"
DEVAGENT_PRE_COMMIT_CONFIG = ".pre-commit-config-devagent.yaml"


def update_pyapp_file() -> str:
    with open(DEFAULT_PRE_COMMIT_CONFIG) as file:
        data = file.read()
        for cmd in ('invoke', 'inv'):
            data = data.replace(f"entry: '{cmd}", "entry: 'devagent")
    with open(DEVAGENT_PRE_COMMIT_CONFIG, 'w') as file:
        file.write(data)
    return DEVAGENT_PRE_COMMIT_CONFIG
