"""Unit tests for gather_licenses.py"""

import unittest

from compliance.gather_licenses import origin_to_module, purl_to_package


class TestOriginToModule(unittest.TestCase):
    """Tests for the origin_to_module function."""

    def test_bazel_target_with_deps_prefix(self):
        """Test Bazel targets in the format @@//deps/package:target"""
        self.assertEqual(origin_to_module("@@//deps/msodbcsql18:license"), "msodbcsql18")

    def test_bazel_repo_rules_format(self):
        """Test repo rules format @+_repo_rules+package//:target"""
        self.assertEqual(origin_to_module("@@+_repo_rules+bzip2//:license"), "bzip2")
        self.assertEqual(origin_to_module("@@+_repo_rules+dbus//:license"), "dbus")
        self.assertEqual(origin_to_module("@@+_repo_rules+gcrypt//:license"), "gcrypt")
        self.assertEqual(origin_to_module("@@+_repo_rules+gpg-error//:license"), "gpg-error")
        self.assertEqual(origin_to_module("@@+_repo_rules+krb5//:license"), "krb5")
        self.assertEqual(origin_to_module("@@+_repo_rules+lua//:license"), "lua")
        self.assertEqual(origin_to_module("@@+_repo_rules+nghttp2//:license"), "nghttp2")
        self.assertEqual(origin_to_module("@@+_repo_rules+popt//:license"), "popt")
        self.assertEqual(origin_to_module("@@+_repo_rules+rpm//:license"), "rpm")
        self.assertEqual(origin_to_module("@@+_repo_rules+sqlite3//:license"), "sqlite3")
        self.assertEqual(origin_to_module("@@+_repo_rules+unixodbc//:license"), "unixodbc")
        self.assertEqual(origin_to_module("@@+_repo_rules+xmlsec//:license"), "xmlsec")
        self.assertEqual(origin_to_module("@@+_repo_rules+xz//:license"), "xz")
        self.assertEqual(origin_to_module("@@+_repo_rules+zlib//:license"), "zlib")

    def test_rules_flex_format(self):
        """Test rules_flex format @rules_flex+//:target"""
        self.assertEqual(origin_to_module("@@rules_flex+//:license_file"), "rules_flex")

    def test_simple_repo_rules_format(self):
        """Test simple repo rules format without the _repo_rules prefix"""
        self.assertEqual(origin_to_module("@@xz+//:license_file"), "xz")

    def test_purl_generic_format(self):
        """Test PURL (package URL) format with generic type"""
        self.assertEqual(
            origin_to_module(
                "pkg:generic/libpcap@1.10.5?download_url=https://www.tcpdump.org/release/libpcap-1.10.5.tar.xz"
            ),
            "libpcap",
        )
        self.assertEqual(
            origin_to_module(
                "pkg:generic/nfsiostat@2.1.1?download_url=https://mirrors.edge.kernel.org/pub/linux/utils/nfs-utils/2.1.1/nfs-utils-2.1.1.tar.gz"
            ),
            "nfsiostat",
        )
        self.assertEqual(
            origin_to_module(
                "pkg:generic/attr@3.5?download_url=https://github.com/SELinuxProject/selinux/releases/download/3.5/libselinux-3.5.tar.gz"
            ),
            "attr",
        )
        self.assertEqual(
            origin_to_module(
                "pkg:generic/libsepol@3.5?download_url=https://github.com/SELinuxProject/selinux/releases/download/3.5/libsepol-3.5.tar.gz"
            ),
            "libsepol",
        )
        self.assertEqual(
            origin_to_module(
                "pkg:generic/acl@2.3.1?download_url=https://download.savannah.nongnu.org/releases/acl/acl-2.3.1.tar.xz"
            ),
            "acl",
        )
        self.assertEqual(
            origin_to_module(
                "pkg:generic/attr@2.5.1?download_url=https://download.savannah.nongnu.org/releases/attr/attr-2.5.1.tar.xz"
            ),
            "attr",
        )
        self.assertEqual(
            origin_to_module(
                "pkg:generic/freetds@1.1.36?download_url=https://www.freetds.org/files/stable/freetds-1.1.36.tar.gz"
            ),
            "freetds",
        )

    def test_single_slash_bazel_target(self):
        """Test Bazel targets with single slash format //package:target"""
        self.assertEqual(origin_to_module("//deps/curl:license"), "curl")
        self.assertEqual(origin_to_module("//deps/openssl:license"), "openssl")

    def test_trailing_plus_and_slash(self):
        """Test that trailing + and / are properly stripped"""
        self.assertEqual(origin_to_module("@@+_repo_rules+test+//:license"), "test")
        self.assertEqual(origin_to_module("@@+_repo_rules+test///:license"), "test")

    def test_different_target_names(self):
        """Test that different target names (not just :license) work"""
        self.assertEqual(origin_to_module("@@+_repo_rules+zlib//:license_file"), "zlib")
        self.assertEqual(origin_to_module("@@//deps/package:anything"), "package")

    def test_simple_single_atsign(self):
        """Test single @ sign formats"""
        self.assertEqual(origin_to_module("@+_repo_rules+zlib//:license"), "zlib")

    def test_at_rules_format(self):
        """Test @rules_X+ format"""
        self.assertEqual(origin_to_module("@rules_flex+//:license_file"), "rules_flex")


class TestPurlToPackage(unittest.TestCase):
    """Tests for the purl_to_package helper function."""

    def test_basic_purl(self):
        """Test basic PURL parsing"""
        self.assertEqual(purl_to_package("pkg:generic/libpcap@1.10.5?download_url=https://example.com"), "libpcap")

    def test_purl_without_query(self):
        """Test PURL without query parameters"""
        self.assertEqual(purl_to_package("pkg:generic/curl@7.85.0"), "curl")

    def test_purl_with_multiple_query_params(self):
        """Test PURL with multiple query parameters"""
        result = purl_to_package("pkg:generic/libsepol@3.5?download_url=https://example.com&other=value")
        self.assertEqual(result, "libsepol")

    def test_purl_with_hyphens_in_name(self):
        """Test PURL where package name has hyphens"""
        self.assertEqual(purl_to_package("pkg:generic/gpg-error@1.45?download_url=https://example.com"), "gpg-error")


if __name__ == '__main__':
    unittest.main()
