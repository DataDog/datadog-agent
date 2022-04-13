#!/usr/bin/env python3

# This tool uses the output of `go_deps.go` and tracks down the dependency
# tree branches that import the specified module. This is useful when
# tracking down what parent depenedency is causing a particular module
# to be downloaded/used.

import argparse


def parse_line(line):
    """
    This method goes line-by-line in the file and returns the machine
    usable fields from it.
    """
    trimmed_line = line.strip()
    leading_spaces = len(line) - len(trimmed_line)

    # Strip the minus
    module_dep = trimmed_line[2:]
    # return the depth level and the dependency name
    return int(leading_spaces / 4) + 1, module_dep


def dep_why(dep_tree_filename, target_module):
    """
    This method recursively goes down the dependency tree and prints out
    the branches that lead to the specified target module
    """

    print(f"Searching for '{target_module}' in {dep_tree_filename}...")

    found = False
    current_path = []
    with open(dep_tree_filename) as infile:
        for line in infile:
            level, module_dep = parse_line(line)

            current_path = current_path[: level + 1]

            current_path.append(module_dep)

            module_name = module_dep.split('@')[0]
            if module_name == target_module:
                found = True
                print(current_path[1:])

    if not found:
        print(f"'{target_module}' was not found in the dependency tree!")


if __name__ == '__main__':
    parser = argparse.ArgumentParser(prog='dep_why')
    parser.add_argument(
        '-f',
        '--filename',
        default='dependency_tree.txt',
        help="Dependency tree file",
    )
    parser.add_argument('module')

    args = parser.parse_args()

    dep_why(args.filename, args.module)
