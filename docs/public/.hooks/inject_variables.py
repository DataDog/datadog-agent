import re
from functools import cache
from pathlib import Path

import httpx
from markdown.preprocessors import Preprocessor


@cache
def variable_replacements():
    return {
        f"<<<{variable}>>>": replacement
        for variable, replacement in (
            ("GO_VERSION", get_go_version()),
            ("PYTHON_VERSION", get_python_version()),
            ("DDA_DOCS_INSTALL", get_dda_install_docs()),
            ("DDA_DOCS_TAB_COMPLETE", get_dda_tab_complete_docs()),
            ("VSCODE_EXTENSIONS", get_vscode_extensions()),
            ("JSON_SCHEMA_CI_UNIT", get_json_schema_ci_unit()),
            ("CONFIGURED_CI_UNITS", get_configured_ci_units()),
        )
    }


@cache
def get_dda_version():
    gitlab_config = Path("ci/templates/variables.yml").read_text(encoding="utf-8")
    build_image_ref = re.search(r"^\s*CI_IMAGE_LINUX: v[^-]+-(.+)$", gitlab_config, flags=re.MULTILINE).group(1)
    version_url = f"https://raw.githubusercontent.com/DataDog/datadog-agent-buildimages/{build_image_ref}/dda.env"
    response = httpx.get(version_url)
    response.raise_for_status()
    return re.search(r"DDA_VERSION=v(.*)", response.text).group(1)


def get_go_version():
    return Path(".go-version").read_text(encoding="utf-8").strip()


def get_python_version():
    return Path(".python-version").read_text(encoding="utf-8").strip()


def get_dda_install_docs():
    version = get_dda_version()
    docs_url = f"https://raw.githubusercontent.com/DataDog/datadog-agent-dev/refs/tags/v{version}/docs/install.md"
    response = httpx.get(docs_url)
    response.raise_for_status()
    # Split out the content from the title divider
    content = response.text.split("-----", 1)[1]
    # Locate the upgrade section
    upgrade_block = re.search(r"## Upgrade.+?(/// warning)", content, flags=re.DOTALL)
    # Strip out everything after the upgrade section, and ignore its warning
    content = content[: upgrade_block.start(1)]
    # Substitute placeholder with the pinned version
    content = content.replace("<<<DDA_VERSION>>>", version)
    # Add an extra level to the headers
    return re.sub(r"^#", "##", content, flags=re.MULTILINE).strip()


def get_dda_tab_complete_docs():
    version = get_dda_version()
    docs_url = (
        f"https://raw.githubusercontent.com/DataDog/datadog-agent-dev/refs/tags/v{version}/docs/reference/cli/index.md"
    )
    response = httpx.get(docs_url)
    response.raise_for_status()
    # Extract the tab completion section
    content = re.search(r"^## Tab completion(.+?)(?=^#|\Z)", response.text, flags=re.MULTILINE | re.DOTALL)
    return content.group(1).strip()


def get_vscode_extensions():
    url = "https://raw.githubusercontent.com/DataDog/datadog-agent-buildimages/refs/heads/main/dev-envs/linux/default-vscode-extensions.txt"
    response = httpx.get(url)
    response.raise_for_status()
    marketplace_url_base = "https://marketplace.visualstudio.com/items?itemName="
    return "\n".join(
        f"- [{extension}]({marketplace_url_base}{extension})"
        for extension in response.text.splitlines()
        if not extension.startswith("#")
    )


def get_json_schema_ci_unit():
    from utils.ci.config.model.unit import CIUnit

    import jsonschema_markdown
    import msgspec

    json_schema = msgspec.json.schema(CIUnit)
    full_content = jsonschema_markdown.generate(json_schema, footer=False, hide_empty_columns=True)
    _, _, schema_doc = full_content.partition("## CIUnit")
    # Remove property table headers
    schema_doc = schema_doc.replace("#### Type: `object`", "")
    # Remove `additionalProperties` warnings for frozen structs
    schema_doc = schema_doc.replace("> ⚠️ Additional properties are not allowed.", "")
    return schema_doc.strip()


def get_configured_ci_units():
    import msgspec
    import tomllib
    from utils.ci.config.model.unit import CIUnit

    lines = []
    for unit_file in sorted(Path("ci/units").glob("*.toml")):
        data = tomllib.loads(unit_file.read_text(encoding="utf-8"))
        unit = msgspec.convert(data, CIUnit)
        lines.extend((f"## {unit.name}", "", unit.description))

    return "\n".join(lines)


class VariableInjectionPreprocessor(Preprocessor):
    def run(self, lines):  # noqa: PLR6301
        markdown = "\n".join(lines)
        for variable, replacement in variable_replacements().items():
            markdown = markdown.replace(variable, replacement)

        return markdown.splitlines()
