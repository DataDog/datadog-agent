#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import re
import subprocess
import sys
from pathlib import Path, PurePosixPath

GLOB_PATTERN = "**/*.go"

COPYRIGHT_REGEX = [
    r'^// Unless explicitly stated otherwise all files in this repository are licensed$',
    r'^// under the Apache License Version 2.0\.$',
    r'^// This product includes software developed at Datadog \(https://www\.[Dd]atadoghq\.com/\)\.$',
    r'^// Copyright 20[1-3][0-9]-([Pp]resent|20[1-3][0-9]) Datadog, (Inc|Inmetrics)\.$',
]

# These path patterns are excluded from checks
PATH_EXCLUSION_REGEX = [
    '/third_party/golang/',
    '/third_party/kubernetes/',
]

# These header matchers skip enforcement of the rules if found in the first
# line of the file
HEADER_EXCLUSION_REGEX = [
    '^// Code generated ',
    '^//go:generate ',
    '^// Copyright.* OpenTelemetry Authors',
    '^// Copyright.* The Go Authors',
]


COMPILED_COPYRIGHT_REGEX = [re.compile(regex, re.UNICODE) for regex in COPYRIGHT_REGEX]
COMPILED_PATH_EXCLUSION_REGEX = [re.compile(regex, re.UNICODE) for regex in PATH_EXCLUSION_REGEX]
COMPILED_HEADER_EXCLUSION_REGEX = [re.compile(regex, re.UNICODE) for regex in HEADER_EXCLUSION_REGEX]


class CopyrightLinter:
    """
    This class is used to enforce copyright headers on specified file patterns
    """

    @staticmethod
    def _get_repo_dir():
        script_dir = PurePosixPath(__file__).parent

        repo_dir = (
            subprocess.check_output(
                ['git', 'rev-parse', '--show-toplevel'],
                cwd=script_dir,
            )
            .decode(sys.stdout.encoding)
            .strip()
        )

        return PurePosixPath(repo_dir)

    @staticmethod
    def _is_excluded_path(filepath, exclude_matchers):
        for matcher in exclude_matchers:
            if re.search(matcher, filepath.as_posix()):
                return True

        return False

    @staticmethod
    def _get_matching_files(root_dir, glob_pattern, exclude=None):
        if exclude is None:
            exclude = []

        # Glob is a generator so we have to do the counting ourselves
        all_matching_files_cnt = 0

        filtered_files = []
        for filepath in Path(root_dir).glob(glob_pattern):
            all_matching_files_cnt += 1
            if not CopyrightLinter._is_excluded_path(filepath, exclude):
                filtered_files.append(filepath)

        excluded_files_cnt = all_matching_files_cnt - len(filtered_files)
        print(f"[INFO] Excluding {excluded_files_cnt} files based on path filters!")

        return sorted(filtered_files)

    @staticmethod
    def _get_header(filepath):
        header = []
        with open(filepath, "r") as file_obj:
            # We expect a specific header format which should be 4 lines
            for _ in range(4):
                header.append(file_obj.readline().strip())

        return header

    @staticmethod
    def _is_excluded_header(header, exclude=None):
        if exclude is None:
            exclude = []

        for matcher in exclude:
            if re.search(matcher, header[0]):
                return True

        return False

    @staticmethod
    def _has_copyright(filepath, debug=False):
        header = CopyrightLinter._get_header(filepath)
        if header is None:
            print("[WARN] Mismatch found! Could not find any content in file!")
            return False

        if len(header) > 0 and CopyrightLinter._is_excluded_header(header, exclude=COMPILED_HEADER_EXCLUSION_REGEX):
            if debug:
                print(f"[INFO] Excluding {filepath} based on header '{header[0]}'")
            return True

        if len(header) <= 3:
            print("[WARN] Mismatch found! File too small for header stanza!")
            return False

        for line_idx, matcher in enumerate(COMPILED_COPYRIGHT_REGEX):
            if not re.match(matcher, header[line_idx]):
                print(
                    f"[WARN] Mismatch found! Expected '{COPYRIGHT_REGEX[line_idx]}' pattern but got '{header[line_idx]}'"
                )
                return False

        return True

    @staticmethod
    def _assert_copyrights(files, debug=False):
        failing_files = []
        for filepath in files:
            if CopyrightLinter._has_copyright(filepath, debug=debug):
                if debug:
                    print(f"[ OK ] {filepath}")

                continue

            print(f"[FAIL] {filepath}")
            failing_files.append(filepath)

        total_files = len(files)
        if failing_files:
            pct_failing = (len(failing_files) / total_files) * 100
            print()
            print(
                f"FAIL: There are {len(failing_files)} files out of "
                + f"{total_files} ({pct_failing:.2f}%) that are missing the proper copyright!"
            )

        return failing_files

    def assert_compliance(self, debug=False):
        """
        This method applies the GLOB_PATTERN to the root of the repository and
        verifies that all files have the expected copyright header.
        """

        git_repo_dir = CopyrightLinter._get_repo_dir()

        if debug:
            print(f"[DEBG] Repo root: {git_repo_dir}")
            print(f"[DEBG] Finding all files in {git_repo_dir} matching '{GLOB_PATTERN}'...")

        matching_files = CopyrightLinter._get_matching_files(
            git_repo_dir,
            GLOB_PATTERN,
            exclude=COMPILED_PATH_EXCLUSION_REGEX,
        )
        print(f"[INFO] Found {len(matching_files)} files matching '{GLOB_PATTERN}'")

        failing_files = CopyrightLinter._assert_copyrights(matching_files, debug=debug)
        if len(failing_files) > 0:
            print("CHECK: FAIL")
            raise Exception(
                f"Copyright linting found {len(failing_files)} files that did not have the expected header!"
            )

        print("CHECK: OK")


if __name__ == '__main__':
    CopyrightLinter().assert_compliance(debug=True)
