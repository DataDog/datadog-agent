from tasks.e2e_framework.config import Config
from tasks.e2e_framework.tool import info

# Defaults that work with fakeintake-based tests (the overwhelming majority of E2E
# scenarios). For the rare real-intake case, set E2E_API_KEY / E2E_APP_KEY in the
# environment or fill the values in ~/.test_infra_config.yaml manually.
_DEFAULT_API_KEY = "0" * 32
_DEFAULT_APP_KEY = "0" * 40


def setup_agent_config(config: Config):
    """
    Populate the agent block with placeholder API/APP keys if they are missing.
    Existing user-provided values are preserved.
    """
    if config.configParams.agent is None:
        config.configParams.agent = Config.Params.Agent(apiKey=None, appKey=None)

    agent = config.configParams.agent
    if not agent.apiKey:
        agent.apiKey = _DEFAULT_API_KEY
    if not agent.appKey:
        agent.appKey = _DEFAULT_APP_KEY

    if agent.apiKey == _DEFAULT_API_KEY and agent.appKey == _DEFAULT_APP_KEY:
        info(
            "✓ Datadog API/APP keys: placeholder values (works with fakeintake; set E2E_API_KEY/E2E_APP_KEY for real intake)"
        )
    else:
        info("✓ Datadog API/APP keys already configured")
