project = 'Datadog OpenTelemetry Agent'
language = 'en'
author = 'Datadog'
copyright = '2016-2026'
release = '7.78.0'

extensions = ['myst_parser', 'sphinx_copybutton', 'sphinx_design']

html_theme = 'furo'
html_logo = 'https://opentelemetry.io/img/logos/opentelemetry-horizontal-color.svg'
html_theme_options = {
    'navigation_with_keys': True,
}

myst_enable_extensions = ['colon_fence']
