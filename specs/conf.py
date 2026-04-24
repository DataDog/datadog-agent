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
html_theme_options = {
    'navigation_with_keys': True,
}

html_static_path = ['markdown/css']
html_css_files = [
    "https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.1.1/css/all.min.css",
    "speky.css",
]

copybutton_prompt_text = '$ '

myst_enable_extensions = [
    'colon_fence',
    'substitution',
]
myst_heading_anchors = 4
myst_substitutions = {'project': project}
