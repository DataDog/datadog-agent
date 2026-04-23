project = 'Fleet Installer'
language = 'en'

extensions = [
    'myst_parser',
    'sphinx_copybutton',
    'sphinx_design',
]

primary_domain = None
source_suffix = {'.rst': 'restructuredtext', '.md': 'markdown'}

html_theme = 'furo'

myst_enable_extensions = [
    'colon_fence',
    'substitution',
]
myst_heading_anchors = 4
myst_substitutions = {'project': project}
