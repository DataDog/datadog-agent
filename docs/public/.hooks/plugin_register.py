import os
import sys

from markdown.extensions import Extension

HERE = os.path.dirname(__file__)


def on_config(
    config,
    **kwargs,  # noqa: ARG001
):
    config.markdown_extensions.append(GlobalExtension())


class GlobalExtension(Extension):
    def extendMarkdown(self, md):  # noqa: N802, PLR6301
        sys.path.insert(0, HERE)

        from inject_variables import VariableInjectionPreprocessor

        md.preprocessors.register(VariableInjectionPreprocessor(), VariableInjectionPreprocessor.__name__, 100)

        sys.path.pop(0)
