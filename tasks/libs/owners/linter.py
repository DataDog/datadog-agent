import os
import sys

from tasks.libs.common.color import color_message


def directory_has_packages_without_owner(owners, folder="pkg"):
    """Check every package in `pkg` has an owner"""

    error = False

    for x in os.listdir(folder):
        path = os.path.join("/" + folder, x)
        if all(owner[1].rstrip('/') != path for owner in owners.paths):
            if not error:
                print(
                    color_message("The following packages don't have owner in CODEOWNER file", "red"), file=sys.stderr
                )
                error = True
            print(color_message(f"\t- {path}", "orange"), file=sys.stderr)

    return error


def codeowner_has_orphans(owners):
    """Check that every rule in codeowners file point to an existing file/directory"""

    err_invalid_rule_path = False
    err_orphans_path = False

    for rule in owners.paths:
        try:
            # Get the static part of the rule path, removing matching subpath (such as '*')
            static_root = _get_static_root(rule[1])
        except Exception:
            err_invalid_rule_path = True
            print(
                color_message(
                    f"[UNSUPPORTED] The following rule's path does not start with '/' anchor: {rule[1]}", "red"
                ),
                file=sys.stderr,
            )
            continue

        if not _is_pattern_in_fs(static_root, rule[0]):
            if not err_orphans_path:
                print(
                    color_message(
                        "The following rules are outdated: they don't point to existing file/directory", "red"
                    ),
                    file=sys.stderr,
                )
                err_orphans_path = True
            print(color_message(f"\t- {rule[1]}\t{rule[2]}", "orange"), file=sys.stderr)

    return err_invalid_rule_path or err_orphans_path


def _get_static_root(pattern):
    """_get_static_root returns the longest prefix path from the pattern without any wildcards."""
    result = "."

    if not pattern.startswith("/"):
        raise Exception()

    # We remove the '/' anchor character from the path
    pattern = pattern[1:]

    for elem in pattern.split("/"):
        if '*' in elem:
            return result
        result = os.path.join(result, elem)
    return result


def _is_pattern_in_fs(path, pattern):
    """Checks if a given pattern matches any file within the specified path.

    Args:
        path (str): The file or directory path to search within.
        pattern (re.Pattern): The compiled regular expression pattern to match against file paths.

    Returns:
        bool: True if the pattern matches any file path within the specified path, False otherwise.
    """
    if os.path.isfile(path):
        return True
    elif os.path.isdir(path):
        for root, _, files in os.walk(path):
            # Check if root is matching the the pattern, without "./" at the begining
            if pattern.match(root[2:]):
                return True
            for name in files:
                # file_path is the relative path from the root of the repo, without "./" at the begining
                file_path = os.path.join(root, name)[2:]

                # Check if the file path matches any of the regex patterns
                if pattern.match(file_path):
                    return True
    return False
