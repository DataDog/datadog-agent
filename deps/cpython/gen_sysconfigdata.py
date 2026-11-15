"""Generate CPython's _sysconfigdata module from pyconfig.h and a dict of Makefile variables."""

import argparse
import json
import pprint
import re

_DEFINE_RE = re.compile(r"^#define ([A-Z][A-Za-z0-9_]+) (.*)\n")
_UNDEF_RE = re.compile(r"^/[*] #undef ([A-Z][A-Za-z0-9_]+) [*]/\n")


def parse_config_h(path):
    config = {}
    with open(path) as f:
        for line in f:
            if m := _DEFINE_RE.match(line):
                name, value = m.groups()
                try:
                    value = int(value)
                except ValueError:
                    pass
                config[name] = value
            elif m := _UNDEF_RE.match(line):
                config[m.group(1)] = 0
    return config


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pyconfig", required=True)
    parser.add_argument("--vars", required=True)
    parser.add_argument("--prefix", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()

    with open(args.vars) as f:
        build_time_vars = {
            key: value.replace("@PREFIX@", args.prefix) if isinstance(value, str) else value
            for key, value in json.load(f).items()
        }
    build_time_vars.update(parse_config_h(args.pyconfig))

    with open(args.out, "w") as f:
        f.write("# system configuration generated and used by the sysconfig module\n")
        f.write("build_time_vars = ")
        f.write(pprint.pformat(build_time_vars, width=100, sort_dicts=True))
        f.write("\n")


if __name__ == "__main__":
    main()
