import stat
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from tasks.libs.package import wheel_libs


class TestMangledLibNameParsing(unittest.TestCase):
    def test_parses_versioned_mangled_names(self):
        self.assertEqual(wheel_libs.parse_mangled_lib_name("libcrypto-51ce5003.so.3"), ("libcrypto", "libcrypto.so.3"))
        self.assertEqual(wheel_libs.parse_mangled_lib_name("libssl-5fe3ef2a.so.3"), ("libssl", "libssl.so.3"))
        self.assertEqual(wheel_libs.parse_mangled_lib_name("libz-4e3ca1c7.so.1.2.7"), ("libz", "libz.so.1"))
        self.assertEqual(wheel_libs.parse_mangled_lib_name("libzstd-8d126cbb.so.1.5.5"), ("libzstd", "libzstd.so.1"))
        self.assertEqual(wheel_libs.parse_mangled_lib_name("libkrb5-04e0cbc2.so.3.3"), ("libkrb5", "libkrb5.so.3"))
        self.assertEqual(
            wheel_libs.parse_mangled_lib_name("libgssapi_krb5-8da44e5f.so.2.2"),
            ("libgssapi_krb5", "libgssapi_krb5.so.2"),
        )
        self.assertEqual(
            wheel_libs.parse_mangled_lib_name("libk5crypto-37a76880.so.3.1"),
            ("libk5crypto", "libk5crypto.so.3"),
        )
        self.assertEqual(
            wheel_libs.parse_mangled_lib_name("libcom_err-c2c4a5b1.so.3.0"),
            ("libcom_err", "libcom_err.so.3"),
        )
        self.assertEqual(
            wheel_libs.parse_mangled_lib_name("libkrb5support-c059b95f.so.0.1"),
            ("libkrb5support", "libkrb5support.so.0"),
        )
        self.assertEqual(wheel_libs.parse_mangled_lib_name("libcurl-53143668.so.4.8.0"), ("libcurl", "libcurl.so.4"))
        self.assertEqual(wheel_libs.parse_mangled_lib_name("libodbc-0febc3ca.so.2.0.0"), ("libodbc", "libodbc.so.2"))

    def test_parses_unversioned_mangled_name(self):
        self.assertEqual(wheel_libs.parse_mangled_lib_name("liblmdb-05a8c963.so"), ("liblmdb", "liblmdb.so"))

    def test_rejects_non_mangled_names(self):
        self.assertIsNone(wheel_libs.parse_mangled_lib_name("libcrypto.so.3"))
        self.assertIsNone(wheel_libs.parse_mangled_lib_name("_rust.abi3.so"))
        self.assertIsNone(wheel_libs.parse_mangled_lib_name("libfoo-short.so.1"))
        self.assertIsNone(wheel_libs.parse_mangled_lib_name("libfoo-zzzzzzzz.so.1"))


class TestReadelfParsers(unittest.TestCase):
    def test_parse_dynamic_section(self):
        sample = """
Dynamic section at offset 0x1000 contains 20 entries:
  Tag        Type                         Name/Value
 0x0000000000000001 (NEEDED)             Shared library: [libssl-5fe3ef2a.so.3]
 0x0000000000000001 (NEEDED)             Shared library: [libcrypto-51ce5003.so.3]
 0x000000000000000e (SONAME)             Library soname: [libpq-2691e7a0.so.5.18]
 0x000000000000000f (RPATH)              Library rpath: [$ORIGIN:$ORIGIN/../psycopg_c.libs]
"""
        info = wheel_libs.parse_dynamic_section(sample)
        self.assertEqual(info.needed, ("libssl-5fe3ef2a.so.3", "libcrypto-51ce5003.so.3"))
        self.assertEqual(info.rpaths, ("$ORIGIN", "$ORIGIN/../psycopg_c.libs"))
        self.assertEqual(info.soname, "libpq-2691e7a0.so.5.18")

    def test_parse_dynamic_symbols_and_verneed(self):
        syms_out = """
Symbol table '.dynsym' contains 5 entries:
   Num:    Value          Size Type    Bind   Vis      Ndx Name
     0: 0000000000000000     0 NOTYPE  LOCAL  DEFAULT  UND
     1: 0000000000000000     0 FUNC    GLOBAL DEFAULT  UND compressBound@ZLIB_1.2.0 (3)
     2: 0000000000000000     0 FUNC    GLOBAL DEFAULT  UND SSL_new@OPENSSL_3.0.0 (2)
     3: 0000000000010000    16 FUNC    GLOBAL DEFAULT   14 EVP_Digest@@OPENSSL_3.0.0
     4: 0000000000020000    16 FUNC    WEAK   DEFAULT   14 unversioned_sym
"""
        defined, defined_names, defined_versions, undefined = wheel_libs.parse_dynamic_symbols(syms_out)
        self.assertIn(("EVP_Digest", "OPENSSL_3.0.0"), defined)
        self.assertIn(("EVP_Digest", None), defined)
        self.assertIn(("unversioned_sym", None), defined)
        self.assertEqual(defined_names, frozenset({"EVP_Digest", "unversioned_sym"}))
        self.assertEqual(defined_versions, frozenset({"OPENSSL_3.0.0"}))
        self.assertEqual(
            undefined,
            (
                ("compressBound", "ZLIB_1.2.0", "GLOBAL"),
                ("SSL_new", "OPENSSL_3.0.0", "GLOBAL"),
            ),
        )

        ver_out = """
Version symbols section '.gnu.version' contains 5 entries:
  000:   0 (*local*)       3 (ZLIB_1.2.0)

Version needs section '.gnu.version_r' contains 2 entries:
 Addr: 0x0000000000001234  Offset: 0x00001234  Link: 5 (.dynstr)
  000000: Version: 1  File: libz-4e3ca1c7.so.1.2.7  Cnt: 1
  0x0010:   Name: ZLIB_1.2.0  Flags: none  Version: 3
  0x0020: Version: 1  File: libssl-5fe3ef2a.so.3  Cnt: 1
  0x0030:   Name: OPENSSL_3.0.0  Flags: none  Version: 2
"""
        verneed = wheel_libs.parse_version_info(ver_out)
        self.assertEqual(
            verneed,
            {
                "libz-4e3ca1c7.so.1.2.7": frozenset({"ZLIB_1.2.0"}),
                "libssl-5fe3ef2a.so.3": frozenset({"OPENSSL_3.0.0"}),
            },
        )


class TestCompatibilityAndDeduplication(unittest.TestCase):
    def _make_elf(self, path: Path) -> Path:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(b"\x7fELF" + b"\x00" * 60)
        return path

    def test_compatibility_detects_missing_version_and_symbol(self):
        consumer = wheel_libs.ElfSymbolInfo(
            defined=frozenset(),
            defined_names=frozenset(),
            defined_versions=frozenset(),
            undefined=(
                ("compressBound", "ZLIB_1.2.0", "GLOBAL"),
                ("deflateInit_", None, "GLOBAL"),
                ("printf", "GLIBC_2.2.5", "GLOBAL"),
            ),
            verneed={"libz-4e3ca1c7.so.1.2.7": frozenset({"ZLIB_1.2.0"})},
        )
        wheel_lib = wheel_libs.ElfSymbolInfo(
            defined=frozenset({("compressBound", "ZLIB_1.2.0"), ("compressBound", None), ("deflateInit_", None)}),
            defined_names=frozenset({"compressBound", "deflateInit_"}),
            defined_versions=frozenset({"ZLIB_1.2.0"}),
            undefined=(),
            verneed={},
        )
        agent_unversioned = wheel_libs.ElfSymbolInfo(
            defined=frozenset({("compressBound", None), ("deflateInit_", None)}),
            defined_names=frozenset({"compressBound", "deflateInit_"}),
            defined_versions=frozenset(),
            undefined=(),
            verneed={},
        )
        issues = wheel_libs.check_consumer_compatibility(
            consumer, "libz-4e3ca1c7.so.1.2.7", wheel_lib, agent_unversioned
        )
        self.assertEqual(len(issues), 2)
        self.assertIn("ZLIB_1.2.0", issues[0])
        self.assertIn("compressBound@ZLIB_1.2.0", issues[1])

        agent_compatible = wheel_libs.ElfSymbolInfo(
            defined=frozenset({("compressBound", "ZLIB_1.2.0"), ("compressBound", None), ("deflateInit_", None)}),
            defined_names=frozenset({"compressBound", "deflateInit_"}),
            defined_versions=frozenset({"ZLIB_1.2.0"}),
            undefined=(),
            verneed={},
        )
        self.assertEqual(
            wheel_libs.check_consumer_compatibility(consumer, "libz-4e3ca1c7.so.1.2.7", wheel_lib, agent_compatible),
            [],
        )

    def test_build_updated_rpath(self):
        self.assertEqual(
            wheel_libs.build_updated_rpath(("$ORIGIN", "$ORIGIN/../foo.libs"), "/opt/datadog-agent/embedded/lib"),
            "$ORIGIN:$ORIGIN/../foo.libs:/opt/datadog-agent/embedded/lib",
        )
        self.assertEqual(
            wheel_libs.build_updated_rpath(
                ("$ORIGIN", "/opt/datadog-agent/embedded/lib"), "/opt/datadog-agent/embedded/lib"
            ),
            "$ORIGIN:/opt/datadog-agent/embedded/lib",
        )
        self.assertEqual(
            wheel_libs.build_updated_rpath((), "/opt/datadog-agent/embedded/lib"),
            "/opt/datadog-agent/embedded/lib",
        )

    def test_deduplicate_end_to_end_with_mocked_elf_tools(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            embedded_lib = root / "embedded" / "lib"
            site_packages = embedded_lib / "python3.13" / "site-packages"

            # Agent ships libcrypto.so.3 and libssl.so.3, but NOT libkrb5.so.3 (e.g. Heroku)
            agent_crypto = self._make_elf(embedded_lib / "libcrypto.so.3")
            agent_ssl = self._make_elf(embedded_lib / "libssl.so.3")

            # Wheels bundle libcrypto, libssl, libkrb5, and private libpq
            wheel_crypto_1 = self._make_elf(site_packages / "cryptography.libs" / "libcrypto-51ce5003.so.3")
            wheel_ssl_1 = self._make_elf(site_packages / "cryptography.libs" / "libssl-5fe3ef2a.so.3")
            rust_ext = self._make_elf(site_packages / "cryptography" / "hazmat" / "bindings" / "_rust.abi3.so")

            wheel_crypto_2 = self._make_elf(site_packages / "psycopg_c.libs" / "libcrypto-51ce5003.so.3")
            wheel_ssl_2 = self._make_elf(site_packages / "psycopg_c.libs" / "libssl-5fe3ef2a.so.3")
            wheel_krb5 = self._make_elf(site_packages / "psycopg_c.libs" / "libkrb5-04e0cbc2.so.3.3")
            wheel_pq = self._make_elf(site_packages / "psycopg_c.libs" / "libpq-2691e7a0.so.5.18")
            # Make _rust.abi3.so read-only to verify chmod handling
            rust_ext.chmod(0o444)

            dyn_map = {
                rust_ext: wheel_libs.ElfDynamicInfo(
                    needed=("libssl-5fe3ef2a.so.3", "libcrypto-51ce5003.so.3"),
                    rpaths=("$ORIGIN/../../../cryptography.libs",),
                ),
                wheel_pq: wheel_libs.ElfDynamicInfo(
                    needed=("libssl-5fe3ef2a.so.3", "libcrypto-51ce5003.so.3", "libkrb5-04e0cbc2.so.3.3"),
                    rpaths=("$ORIGIN",),
                ),
                wheel_crypto_1: wheel_libs.ElfDynamicInfo(needed=(), rpaths=("$ORIGIN",)),
                wheel_ssl_1: wheel_libs.ElfDynamicInfo(needed=("libcrypto-51ce5003.so.3",), rpaths=("$ORIGIN",)),
                wheel_crypto_2: wheel_libs.ElfDynamicInfo(needed=(), rpaths=("$ORIGIN",)),
                wheel_ssl_2: wheel_libs.ElfDynamicInfo(needed=("libcrypto-51ce5003.so.3",), rpaths=("$ORIGIN",)),
                wheel_krb5: wheel_libs.ElfDynamicInfo(needed=(), rpaths=("$ORIGIN",)),
            }

            ssl_syms = wheel_libs.ElfSymbolInfo(
                defined=frozenset({("SSL_new", "OPENSSL_3.0.0"), ("SSL_new", None)}),
                defined_names=frozenset({"SSL_new"}),
                defined_versions=frozenset({"OPENSSL_3.0.0"}),
                undefined=(),
                verneed={},
            )
            crypto_syms = wheel_libs.ElfSymbolInfo(
                defined=frozenset({("EVP_Digest", "OPENSSL_3.0.0"), ("EVP_Digest", None)}),
                defined_names=frozenset({"EVP_Digest"}),
                defined_versions=frozenset({"OPENSSL_3.0.0"}),
                undefined=(),
                verneed={},
            )
            consumer_syms = wheel_libs.ElfSymbolInfo(
                defined=frozenset(),
                defined_names=frozenset(),
                defined_versions=frozenset(),
                undefined=(("SSL_new", "OPENSSL_3.0.0", "GLOBAL"), ("EVP_Digest", "OPENSSL_3.0.0", "GLOBAL")),
                verneed={
                    "libssl-5fe3ef2a.so.3": frozenset({"OPENSSL_3.0.0"}),
                    "libcrypto-51ce5003.so.3": frozenset({"OPENSSL_3.0.0"}),
                },
            )
            sym_map = {
                agent_crypto.resolve(): crypto_syms,
                agent_ssl.resolve(): ssl_syms,
                wheel_crypto_1.resolve(): crypto_syms,
                wheel_ssl_1.resolve(): ssl_syms,
                wheel_crypto_2.resolve(): crypto_syms,
                wheel_ssl_2.resolve(): ssl_syms,
                rust_ext.resolve(): consumer_syms,
                wheel_pq.resolve(): consumer_syms,
            }

            patched_cmds = []

            def fake_run(cmd, **_kwargs):
                patched_cmds.append(cmd)
                # Update dyn_map to simulate patchelf rewriting DT_NEEDED for post-patch check
                target = Path(cmd[-1])
                cur = dyn_map[target]
                new_needed = list(cur.needed)
                for i, tok in enumerate(cmd):
                    if tok == "--replace-needed":
                        old_n, new_n = cmd[i + 1], cmd[i + 2]
                        new_needed = [new_n if x == old_n else x for x in new_needed]
                dyn_map[target] = wheel_libs.ElfDynamicInfo(needed=tuple(new_needed), rpaths=cur.rpaths)
                return mock.MagicMock(returncode=0)

            with (
                mock.patch.object(wheel_libs, "read_dynamic_info", side_effect=lambda p, readelf="readelf": dyn_map[p]),
                mock.patch.object(
                    wheel_libs, "read_symbol_info", side_effect=lambda p, readelf="readelf": sym_map[p.resolve()]
                ),
                mock.patch("tasks.libs.package.wheel_libs.subprocess.run", side_effect=fake_run),
            ):
                res = wheel_libs.deduplicate_wheel_libs(
                    site_packages,
                    embedded_lib,
                    require_dedup={"libcrypto", "libssl"},
                )

            self.assertEqual(
                res.replaced,
                {
                    "libcrypto-51ce5003.so.3": "libcrypto.so.3",
                    "libssl-5fe3ef2a.so.3": "libssl.so.3",
                },
            )
            self.assertIn("libkrb5-04e0cbc2.so.3.3", res.skipped)
            self.assertIn("libpq-2691e7a0.so.5.18", res.skipped)
            self.assertEqual(set(res.patched_files), {rust_ext, wheel_pq})
            self.assertFalse(wheel_crypto_1.exists())
            self.assertFalse(wheel_ssl_1.exists())
            self.assertFalse(wheel_crypto_2.exists())
            self.assertFalse(wheel_ssl_2.exists())
            # cryptography.libs became empty and was removed; psycopg_c.libs still has libpq and libkrb5
            self.assertFalse((site_packages / "cryptography.libs").exists())
            self.assertTrue(wheel_krb5.exists())
            self.assertTrue(wheel_pq.exists())
            # Read-only permissions restored on _rust.abi3.so
            self.assertEqual(stat.S_IMODE(rust_ext.stat().st_mode), 0o444)
            # patchelf used --force-rpath --set-rpath preserving existing RPATH
            rust_cmd = next(c for c in patched_cmds if c[-1] == str(rust_ext))
            self.assertIn("--force-rpath", rust_cmd)
            rpath_idx = rust_cmd.index("--set-rpath") + 1
            self.assertEqual(rust_cmd[rpath_idx], f"$ORIGIN/../../../cryptography.libs:{embedded_lib}")

    def test_fixed_point_transitive_rejection_and_require_dedup_error(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            embedded_lib = root / "embedded" / "lib"
            site_packages = embedded_lib / "python3.13" / "site-packages"

            # liba depends on libb. liba fails ABI check against Agent's liba, so liba stays
            # on disk and becomes a remaining consumer of libb; libb also fails for liba, so
            # both must be kept!
            agent_a = self._make_elf(embedded_lib / "liba.so.1")
            agent_b = self._make_elf(embedded_lib / "libb.so.1")
            wheel_a = self._make_elf(site_packages / "pkg.libs" / "liba-11111111.so.1")
            wheel_b = self._make_elf(site_packages / "pkg.libs" / "libb-22222222.so.1")
            ext = self._make_elf(site_packages / "pkg" / "_ext.so")

            dyn_map = {
                ext: wheel_libs.ElfDynamicInfo(needed=("liba-11111111.so.1",), rpaths=("$ORIGIN/../pkg.libs",)),
                wheel_a: wheel_libs.ElfDynamicInfo(needed=("libb-22222222.so.1",), rpaths=("$ORIGIN",)),
                wheel_b: wheel_libs.ElfDynamicInfo(needed=(), rpaths=("$ORIGIN",)),
            }

            sym_map = {
                ext.resolve(): wheel_libs.ElfSymbolInfo(
                    frozenset(), frozenset(), frozenset(), (("sym_a", None, "GLOBAL"),), {}
                ),
                wheel_a.resolve(): wheel_libs.ElfSymbolInfo(
                    frozenset({("sym_a", None)}),
                    frozenset({"sym_a"}),
                    frozenset(),
                    (("sym_b_only_in_wheel", None, "GLOBAL"),),
                    {},
                ),
                agent_a.resolve(): wheel_libs.ElfSymbolInfo(frozenset(), frozenset(), frozenset(), (), {}),
                wheel_b.resolve(): wheel_libs.ElfSymbolInfo(
                    frozenset({("sym_b_only_in_wheel", None)}),
                    frozenset({"sym_b_only_in_wheel"}),
                    frozenset(),
                    (),
                    {},
                ),
                agent_b.resolve(): wheel_libs.ElfSymbolInfo(frozenset(), frozenset(), frozenset(), (), {}),
            }

            with (
                mock.patch.object(wheel_libs, "read_dynamic_info", side_effect=lambda p, readelf="readelf": dyn_map[p]),
                mock.patch.object(
                    wheel_libs, "read_symbol_info", side_effect=lambda p, readelf="readelf": sym_map[p.resolve()]
                ),
            ):
                res = wheel_libs.deduplicate_wheel_libs(site_packages, embedded_lib)
                self.assertEqual(res.replaced, {})
                self.assertIn("liba-11111111.so.1", res.skipped)
                self.assertIn("libb-22222222.so.1", res.skipped)
                self.assertTrue(wheel_a.exists())
                self.assertTrue(wheel_b.exists())

                with self.assertRaises(RuntimeError) as ctx:
                    wheel_libs.deduplicate_wheel_libs(site_packages, embedded_lib, require_dedup={"liba"})
                self.assertIn("liba-11111111.so.1", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
