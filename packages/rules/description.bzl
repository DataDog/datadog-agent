"""Format multi-line package descriptions for deb and rpm.

Descriptions are written without indentation in BUILD files. Continuation
lines are indented so that package managers display them as written instead
of re-wrapping them:

- deb: two spaces, since one space would allow re-wrapping. Blank lines
  become " .", the Debian paragraph separator.
- rpm: one space. Blank lines are kept as paragraph separators.
"""

def _format(description, indent, blank):
    lines = description.strip("\n").split("\n")
    return "\n".join([lines[0]] + [indent + line if line.strip() else blank for line in lines[1:]])

def deb_description(description):
    # rules_pkg leaves lines that already start with a space untouched.
    return _format(description, "  ", " .")

def rpm_description(description):
    return _format(description, " ", "")
