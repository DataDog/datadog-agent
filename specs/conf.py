project = 'Datadog OpenTelemetry Agent'
language = 'en'
author = 'Datadog'
copyright = '2016-2026'
release = '7.78.0'

# -- General configuration ---------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#general-configuration

extensions = ['myst_parser', 'sphinx_copybutton', 'sphinx_design']

# -- Options for HTML output -------------------------------------------------
# https://www.sphinx-doc.org/en/master/usage/configuration.html#options-for-html-output

html_theme = 'furo'
html_logo = 'https://opentelemetry.io/img/logos/opentelemetry-horizontal-color.svg'

html_theme_options = {
    'navigation_with_keys': True,
}

# -- Myst Parser Options -----------------------------------------------------

myst_fence_as_directive = []
myst_enable_extensions = [
    'attrs_inline',   # used to specify the language of an inline code snippet: `void*`{l=C}
    'attrs_block',    # used to specify the author of a quote
    'colon_fence',    # Allow using ::: instead of ```
    'substitution',   # Allow using {{project}} to reference conf.py variables
]
myst_heading_anchors = 4
myst_words_per_minute = 100
myst_substitutions = {'project': project}
