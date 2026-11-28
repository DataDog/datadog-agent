import shutil
import unittest
from pathlib import Path

from tasks import pre_commit
from tasks.unit_tests.pre_commit_foundation_tests import GitFixture


@unittest.skipUnless(shutil.which("clang-format"), "requires clang-format")
class TestClangFormat(GitFixture):
    def test_staged_config_inheritance_and_canonical_crlf(self):
        self.stage(".gitattributes", b"*.cpp text eol=crlf\n")
        self.stage(".clang-format", b"BasedOnStyle: LLVM\nPointerAlignment: Left\n")
        self.stage("owned/.clang-format", b"BasedOnStyle: InheritParentConfig\nIndentWidth: 4\n")
        path = "owned/library with spaces.cpp"
        self.stage(path, b"int* pointer;\r\n")
        self.assert_input_preserved(pre_commit.check_clang_format, path, fails=False)
        self.git("commit", "-qm", "valid inherited style")
        self.stage(".clang-format", b"BasedOnStyle: LLVM\nPointerAlignment: Right\n")
        Path(".clang-format").write_bytes(b"BasedOnStyle: LLVM\nPointerAlignment: Left\n")
        self.assert_input_preserved(pre_commit.check_clang_format, path)

    def test_ignore_deletion_rechecks_previously_ignored_file(self):
        path = "owned/legacy.cc"
        self.stage(".clang-format", b"BasedOnStyle: LLVM\n")
        self.stage("owned/.clang-format-ignore", b"legacy.cc\n")
        self.stage(path, b"int    x;\n")
        self.assert_input_preserved(pre_commit.check_clang_format, path, fails=False)
        self.git("commit", "-qm", "ignored")
        self.git("rm", "-q", "owned/.clang-format-ignore")
        Path("owned/.clang-format-ignore").write_bytes(b"legacy.cc\n")
        self.assert_input_preserved(pre_commit.check_clang_format, path)

    def test_base_selects_owned_sources_outside_rtloader(self):
        self.stage("old/header.h", b"int    previously_unformatted;\n")
        self.git("commit", "-qm", "old debt")
        self.stage("vendor/library.c", b"int    vendor;\n")
        self.stage("deps/library.c", b"int    dependency;\n")
        self.stage("third_party/library.cpp", b"int    third_party;\n")
        self.stage("owned/vmlinux.h", b"#ifndef __VMLINUX_H__\n#define __VMLINUX_H__\nint    generated;\n#endif\n")
        self.stage("owned/types.h", b"/* Warning, this file is auto-generated */\nint    generated;\n")
        self.stage("owned/valid.C", b"int x;\n")
        self.git("commit", "-qm", "changed")
        pre_commit.check_clang_format(self.ctx, base="HEAD^")
        self.stage("owned/valid.C", b"int   x;\n")
        self.git("commit", "-qm", "formatting error")
        Path("owned/valid.C").write_bytes(b"int x;\n")
        self.assert_input_preserved(pre_commit.check_clang_format, "owned/valid.C", base="HEAD^")
