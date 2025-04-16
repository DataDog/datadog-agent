from pathlib import Path

from markdown.preprocessors import Preprocessor


def variable_replacements():
    return {
        f"<<<{variable}>>>": replacement
        for variable, replacement in (
            ("GO_VERSION", get_go_version()),
        )
    }


def get_go_version():
    return Path(".go-version").read_text(encoding="utf-8").strip()


class VariableInjectionPreprocessor(Preprocessor):
    def run(self, lines):  # noqa: PLR6301
        markdown = "\n".join(lines)
        for variable, replacement in variable_replacements().items():
            markdown = markdown.replace(variable, replacement)

        return markdown.splitlines()
