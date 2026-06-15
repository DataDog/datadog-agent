#!/usr/bin/env python3
"""Build a wheel from source using `build`."""

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
