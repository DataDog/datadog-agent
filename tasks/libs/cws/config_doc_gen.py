import json
from dataclasses import dataclass

import tasks.libs.cws.common as common


@dataclass
class ConfigSetting:
    name: str  # noqa: F841
    config_key: str
    env_var: str
    description: str
    type: str
    default_value: str
    visibility: str  # noqa: F841


def build_settings(settings):
    output = []
    for setting in settings:
        output.append(
            ConfigSetting(
                setting["name"],
                setting["config_key"],
                setting["env_var"],
                setting["description"],
                setting["type"],
                setting.get("default_value", ""),
                setting["visibility"],
            )
        )
    return output


def generate_config_documentation(input: str, output: str, template: str):
    with open(input) as config_json_file:
        json_top_node = json.load(config_json_file)

    public_settings = build_settings(json_top_node.get("public_settings", []))
    warning_settings = build_settings(json_top_node.get("warning_settings", []))

    with open(output, "w") as output_file:
        print(
            common.fill_template(
                template,
                public_settings=public_settings,
                warning_settings=warning_settings,
            ),
            file=output_file,
        )


if __name__ == "__main__":
    import sys

    generate_config_documentation(sys.argv[1], sys.argv[2], sys.argv[3])
