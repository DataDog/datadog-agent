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
            ("DDA_INSTALL_DOCS", get_dda_install_docs()),
        )
    }


def get_go_version():
    return Path(".go-version").read_text(encoding="utf-8").strip()


def get_dda_install_docs():
    version_url = "https://raw.githubusercontent.com/DataDog/datadog-agent-buildimages/refs/heads/main/dda.env"
    response = httpx.get(version_url)
    response.raise_for_status()
    version = re.search(r"DDA_VERSION=v(.*)", response.text).group(1)

    docs_url = "https://raw.githubusercontent.com/DataDog/datadog-agent-dev/refs/heads/main/docs/install.md"
    response = httpx.get(docs_url)
    response.raise_for_status()
    page = response.text
    content = page.split("-----", 1)[1].strip().replace("<<<DDA_VERSION>>>", version)
    return re.sub(r"^#", "##", content, flags=re.MULTILINE)


class VariableInjectionPreprocessor(Preprocessor):
    def run(self, lines):  # noqa: PLR6301
        markdown = "\n".join(lines)
        for variable, replacement in variable_replacements().items():
            markdown = markdown.replace(variable, replacement)

        return markdown.splitlines()
