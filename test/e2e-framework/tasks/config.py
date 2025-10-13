from pathlib import Path
from typing import Dict, Optional

import yaml
from invoke.exceptions import Exit
from pydantic import BaseModel, Extra
from termcolor import colored

from .tool import info

profile_filename = ".test_infra_config.yaml"


class Config(BaseModel, extra=Extra.forbid):
    class Params(BaseModel, extra=Extra.forbid):
        class Aws(BaseModel, extra=Extra.forbid):
            keyPairName: Optional[str]
            publicKeyPath: Optional[str]
            privateKeyPath: Optional[str] = None
            privateKeyPassword: Optional[str] = None
            account: Optional[str]
            teamTag: Optional[str]

            def get_account(self) -> str:
                if self.account is None:
                    return "agent-sandbox"
                if self.account == "sandbox":
                    print(
                        colored(
                            """
Warning: You are deploying to the sandbox account, this AWS account is no longer supported.
You should consider moving to the agent-sandbox account. Please follow https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/3492282517/Getting+started+with+E2E to set it up.
                          """,
                            "yellow",
                        )
                    )
                return self.account

        aws: Optional[Aws]

        class Azure(BaseModel, extra=Extra.forbid):
            _DEFAULT_ACCOUNT = "agent-sandbox"
            publicKeyPath: Optional[str] = None
            account: Optional[str] = _DEFAULT_ACCOUNT

        azure: Optional[Azure] = None

        class GCP(BaseModel, extra=Extra.forbid):
            _DEFAULT_ACCOUNT = "agent-sandbox"
            publicKeyPath: Optional[str] = None
            pullSecretPath: Optional[str] = None
            account: Optional[str] = _DEFAULT_ACCOUNT

        gcp: Optional[GCP] = None

        class Local(BaseModel, extra=Extra.forbid):
            publicKeyPath: Optional[str] = None

        local: Optional[Local] = None

        class Agent(BaseModel, extra=Extra.forbid):
            apiKey: Optional[str]
            appKey: Optional[str]
            verifyCodeSignature: Optional[bool] = True  # noqa used in e2e tests

        agent: Optional[Agent]

        class Pulumi(BaseModel, extra=Extra.forbid):
            logLevel: Optional[int] = None
            logToStdErr: Optional[bool] = None
            verboseProgressStreams: Optional[bool] = None  # noqa used in e2e tests

        pulumi: Optional[Pulumi] = None

        devMode: Optional[bool] = False  # noqa used in e2e tests

    configParams: Optional[Params] = None

    stackParams: Optional[Dict[str, Dict[str, str]]] = None

    class Options(BaseModel, extra=Extra.forbid):
        checkKeyPair: Optional[bool]

    options: Optional[Options] = None

    def get_options(self) -> Options:
        if self.options is None:
            return Config.Options(checkKeyPair=False)
        return self.options

    def get_azure(self) -> Params.Azure:
        default = Config.Params.Azure(publicKeyPath=None)
        if self.configParams is None:
            return default
        if self.configParams.azure is None:
            return default
        return self.configParams.azure

    def get_gcp(self) -> Params.GCP:
        default = Config.Params.GCP(publicKeyPath=None)
        if self.configParams is None:
            return default
        if self.configParams.gcp is None:
            return default
        return self.configParams.gcp

    def get_aws(self) -> Params.Aws:
        default = Config.Params.Aws(keyPairName=None, publicKeyPath=None, account=None, teamTag=None)
        if self.configParams is None:
            return default
        if self.configParams.aws is None:
            return default
        return self.configParams.aws

    def get_local(self) -> Params.Local:
        default = Config.Params.Local(publicKeyPath=None)
        if self.configParams is None:
            return default
        if self.configParams.local is None:
            return default
        return self.configParams.local

    def get_agent(self) -> Params.Agent:
        default = Config.Params.Agent(apiKey=None, appKey=None)
        if self.configParams is None:
            return default
        if self.configParams.agent is None:
            return default
        return self.configParams.agent

    def get_pulumi(self) -> Params.Pulumi:
        default = Config.Params.Pulumi(
            logLevel=None,
            logToStdErr=None,
            verboseProgressStreams=None,
        )
        if self.configParams is None:
            return default
        if self.configParams.pulumi is None:
            return default
        return self.configParams.pulumi

    def get_stack_params(self) -> Dict[str, Dict[str, str]]:
        if self.stackParams is None:
            return {}
        return self.stackParams

    def save_to_local_config(self, config_path: Optional[str] = None):
        profile_path = get_full_profile_path(config_path)
        try:
            with open(profile_path, "w") as outfile:
                yaml.dump(self.dict(), outfile)
        except Exception as e:
            raise Exit(f"Error saving config file {profile_path}: {e}")
        info(f"Configuration file saved at {profile_path}")


def get_local_config(profile_path: Optional[str] = None) -> Config:
    profile_path = get_full_profile_path(profile_path)
    try:
        with open(profile_path) as f:
            content = f.read()
            config_dict = yaml.load(content, Loader=yaml.Loader)
            return Config.model_validate(config_dict)
    except FileNotFoundError:
        return Config.model_validate({})


def get_full_profile_path(profile_path: Optional[str] = None) -> str:
    if profile_path:
        return str(
            Path(profile_path).expanduser().absolute()
        )  # Return absolute path to config file, handle "~"" with expanduser
    return str(Path.home().joinpath(profile_filename))
