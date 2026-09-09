# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

import tempfile
import unittest
from pathlib import Path
from unittest import mock

from bazel.rules.ebpf.clang_with_unused_inputs import find_unused_inputs, main, parse_args, parse_depfile


class ClangWithUnusedInputsTest(unittest.TestCase):
    def test_parse_depfile(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            depfile = Path(directory) / "program.d"
            depfile.write_text(
                "program.bc: source.c \\\n include/used.h include/escaped\\ header.h\n",
                encoding="utf-8",
            )

            self.assertEqual(
                parse_depfile(depfile),
                {"source.c", "include/used.h", "include/escaped header.h"},
            )

    def test_find_unused_inputs_normalizes_paths(self) -> None:
        self.assertEqual(
            find_unused_inputs(
                ["source.c", "include/used.h", "include/unused.h"],
                {"source.c", "include/../include/used.h"},
            ),
            ["include/unused.h"],
        )

    def test_parse_args_expands_multiline_param_file(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            params = Path(directory) / "wrapper.params"
            params.write_text(
                "--compiler\nclang\n"
                "--depfile\nprogram.d\n"
                "--unused-inputs-list\nprogram.unused\n"
                "--declared-input\nsource.c\n"
                "--\n",
                encoding="utf-8",
            )

            args = parse_args([f"@{params}", "-MD"])

            self.assertEqual(args.compiler, "clang")
            self.assertEqual(args.declared_input, ["source.c"])
            self.assertEqual(args.compiler_args, ["--", "-MD"])

    def test_main_writes_unused_inputs(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            depfile = Path(directory) / "program.d"
            depfile.write_text("program.bc: source.c include/used.h\n", encoding="utf-8")
            unused_inputs = Path(directory) / "program.unused_inputs"

            with mock.patch(
                "bazel.rules.ebpf.clang_with_unused_inputs.subprocess.run",
                return_value=mock.Mock(returncode=0),
            ) as run:
                result = main(
                    [
                        "--compiler",
                        "clang",
                        "--depfile",
                        str(depfile),
                        "--unused-inputs-list",
                        str(unused_inputs),
                        "--declared-input",
                        "source.c",
                        "--declared-input",
                        "include/used.h",
                        "--declared-input",
                        "include/unused.h",
                        "--",
                        "-MD",
                    ]
                )

            self.assertEqual(result, 0)
            self.assertEqual(unused_inputs.read_text(encoding="utf-8"), "include/unused.h\n")
            run.assert_called_once_with(["clang", "-MD"], check=False)


if __name__ == "__main__":
    unittest.main()
