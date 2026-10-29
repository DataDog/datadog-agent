"""Format multi-line package descriptions the way omnibus does.

Descriptions are written without indentation in BUILD files. Omnibus project
descriptions instead indent every continuation line with one space, and its
packagers handle that literal differently:

- deb: the control template prepends another space to each continuation line,
  so they end up indented by two spaces. Debian displays such lines verbatim
  instead of word-wrapping them. Blank lines become " .", the Debian paragraph
  separator.
- rpm: the description is used as is, keeping the single space indentation,
  which rpm also treats as verbatim. Blank lines are kept as paragraph
  separators.

Reproduce these outputs so that Bazel and omnibus packages are displayed the
same way by package managers.
"""

def _format(description, indent, blank):
    lines = description.strip("\n").split("\n")
    return "\n".join([lines[0]] + [indent + line if line.strip() else blank for line in lines[1:]])

def deb_description(description):
    # rules_pkg leaves lines that already start with a space untouched.
    return _format(description, "  ", " .")

def rpm_description(description):
    return _format(description, " ", "")
