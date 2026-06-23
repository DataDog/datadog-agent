#!/usr/bin/env python3
"""Build a wheel from source using `build`.

The choice of `build` as the build frontend is based on it being the most minimalist
officially supported PEP-517-compliant build frontend, with a very minimal set of dependencies
and with reduced risk of side effects. Using `pip` would potentially require more care
around controlling the environment to ensure the right build dependencies (like a predefined build backend)
are present and we don't break hermeticity.
"""

import argparse

from build import ProjectBuilder


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--src", required=True)
    parser.add_argument("--output-dir", required=True)
    args = parser.parse_args()

    ProjectBuilder(args.src).build("wheel", args.output_dir)


if __name__ == "__main__":
    main()
