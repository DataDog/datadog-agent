DEFAULT_PRE_COMMIT_CONFIG = ".pre-commit-config.yaml"
DEVA_PRE_COMMIT_CONFIG = ".pre-commit-config-deva.yaml"


def update_pyapp_file() -> str:
    with open(DEFAULT_PRE_COMMIT_CONFIG) as file:
        data = file.read()
        for cmd in ('invoke', 'inv'):
            data = data.replace(f"entry: '{cmd}", "entry: 'deva")
    with open(DEVA_PRE_COMMIT_CONFIG, 'w') as file:
        file.write(data)
    return DEVA_PRE_COMMIT_CONFIG
