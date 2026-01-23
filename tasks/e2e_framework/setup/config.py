from tasks.e2e_framework.config import Config
from tasks.e2e_framework.tool import warn


def check_config(config: Config):
    aws = config.get_aws()
    if aws.privateKeyPassword:
        warn("WARNING: privateKeyPassword is set. Please ensure privateKeyPath is used ONLY for E2E tests.")
