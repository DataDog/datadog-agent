import tempfile
import unittest
from pathlib import Path

from tasks.libs.package.auditwheel import (
    canonical_library_name,
    discover_duplicate_libraries,
    normalize_auditwheel_libraries,
)


class FakePatchelf:
    def __init__(self, needed_by_file, ignore_replacements=False):
        self.needed_by_file = {str(path): list(needed) for path, needed in needed_by_file.items()}
        self.ignore_replacements = ignore_replacements
        self.rpaths = {}
        self.calls = []
        self.on_print_needed = None

    def __call__(self, arguments):
        arguments = [str(argument) for argument in arguments]
        self.calls.append(arguments)
        operation = arguments[0]
        if operation == '--print-needed':
            path = arguments[1]
            if self.on_print_needed is not None:
                self.on_print_needed(Path(path))
            return '\n'.join(self.needed_by_file[path])
        if operation == '--replace-needed':
            old_name, new_name, path = arguments[1:]
            if not self.ignore_replacements:
                self.needed_by_file[path] = [
                    new_name if needed == old_name else needed for needed in self.needed_by_file[path]
                ]
            return ''
        if operation == '--print-rpath':
            return self.rpaths.get(arguments[1], '')
        if operation == '--add-rpath':
            rpath, path = arguments[1:]
            existing = self.rpaths.get(path, '')
            self.rpaths[path] = ':'.join(part for part in (existing, rpath) if part)
            return ''
        raise AssertionError(f'unexpected patchelf invocation: {arguments}')


def write_elf(path):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(b'\x7fELFtest fixture')
    return path


class TestAuditwheelLibrary(unittest.TestCase):
    def test_maps_precise_auditwheel_hashes_with_version_suffixes(self):
        valid_names = {
            'libssl-0123abcd.so.3': 'libssl.so.3',
            'libcrypto-deadbeef.so.3.2.1': 'libcrypto.so.3',
            'libz-a1b2c3d4.so.1.2.7': 'libz.so.1',
            'libcurl-abcdef01.so.4.8.0': 'libcurl.so.4',
            'libkrb5-04e0cbc2.so.3.3': 'libkrb5.so.3',
            'libk5crypto-37a76880.so.3.1': 'libk5crypto.so.3',
            'libkrb5support-c059b95f.so.0.1': 'libkrb5support.so.0',
            'libcom_err-c2c4a5b1.so.3.0': 'libcom_err.so.3',
            'libgssapi_krb5-8da44e5f.so.2.2': 'libgssapi_krb5.so.2',
            'libodbc-0febc3ca.so.2.0.0': 'libodbc.so.2',
        }
        for name, canonical_name in valid_names.items():
            with self.subTest(name=name):
                self.assertEqual(canonical_library_name(name), canonical_name)

        invalid_names = (
            'libssl-1234567.so.3',
            'libssl-0123ABCDE.so.3',
            'libssl-notahash.so.3',
            'libssl-0123abcd.so.1',
            'libssl.so.3',
            'libpq-0123abcd.so.5.17',
        )
        for name in invalid_names:
            with self.subTest(name=name):
                self.assertIsNone(canonical_library_name(name))

    def test_discovers_candidates_recursively_and_preserves_unique_wheel_libraries(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            site_packages = Path(temporary_directory)
            candidates = (
                'cryptography.libs/libssl-0123abcd.so.3',
                'confluent_kafka.libs/libz-a1b2c3d4.so.1.2.7',
                'pyodbc.libs/libodbc-0febc3ca.so.2.0.0',
                'linked.libs/libssl-0123abcd.so.3',
            )
            preserved = (
                'psycopg_c.libs/libpq-0123abcd.so.5.17',
                'confluent_kafka.libs/librdkafka-0123abcd.so.1',
                'zstandard.libs/libzstd-0123abcd.so.1.5.7',
                'lmdb.libs/liblmdb-0123abcd.so.0.0.0',
            )
            for relative_path in candidates[:-1] + preserved:
                write_elf(site_packages / relative_path)
            linked_library = site_packages / candidates[-1]
            linked_library.parent.mkdir(parents=True)
            linked_library.symlink_to('../cryptography.libs/libssl-0123abcd.so.3')

            discovered = discover_duplicate_libraries(site_packages)

            self.assertEqual(
                {path.relative_to(site_packages).as_posix() for path in discovered},
                set(candidates),
            )
            for relative_path in preserved:
                self.assertTrue((site_packages / relative_path).exists())

    def test_rewrites_every_elf_consumer_and_preserves_unique_libraries(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            site_packages = root / 'site-packages'
            embedded_lib = root / 'embedded' / 'lib'
            embedded_lib.mkdir(parents=True)
            duplicate_names = {
                'libssl-0123abcd.so.3': 'libssl.so.3',
                'libcrypto-deadbeef.so.3': 'libcrypto.so.3',
                'libcurl-abcdef01.so.4.8.0': 'libcurl.so.4',
            }
            for duplicate_name, canonical_name in duplicate_names.items():
                write_elf(site_packages / 'wheel.libs' / duplicate_name)
                write_elf(embedded_lib / canonical_name)

            extension = write_elf(site_packages / 'package' / 'extension.abi3.so')
            libpq = write_elf(site_packages / 'psycopg_c.libs' / 'libpq-0123abcd.so.5.17')
            librdkafka = write_elf(site_packages / 'confluent_kafka.libs' / 'librdkafka-0123abcd.so.1')
            zstd = write_elf(site_packages / 'zstandard.libs' / 'libzstd-0123abcd.so.1.5.7')
            lmdb = write_elf(site_packages / 'lmdb.libs' / 'liblmdb-0123abcd.so.0.0.0')
            (site_packages / 'package' / 'not-an-elf.so').write_text('not an ELF')
            runner = FakePatchelf(
                {
                    extension: ['libssl-0123abcd.so.3'],
                    libpq: ['libcrypto-deadbeef.so.3', 'libssl-0123abcd.so.3'],
                    librdkafka: ['libcurl-abcdef01.so.4.8.0', 'libssl-0123abcd.so.3'],
                    zstd: [],
                    lmdb: [],
                    **{site_packages / 'wheel.libs' / duplicate_name: [] for duplicate_name in duplicate_names},
                }
            )

            result = normalize_auditwheel_libraries(site_packages, embedded_lib, runner=runner)

            self.assertEqual(runner.needed_by_file[str(extension)], ['libssl.so.3'])
            self.assertEqual(runner.needed_by_file[str(libpq)], ['libcrypto.so.3', 'libssl.so.3'])
            self.assertEqual(runner.needed_by_file[str(librdkafka)], ['libcurl.so.4', 'libssl.so.3'])
            for consumer in (extension, libpq, librdkafka):
                self.assertIn(str(embedded_lib), runner.rpaths[str(consumer)].split(':'))
            for duplicate_name in duplicate_names:
                self.assertFalse((site_packages / 'wheel.libs' / duplicate_name).exists())
            for unique_library in (libpq, librdkafka, zstd, lmdb):
                self.assertTrue(unique_library.exists())
            self.assertEqual(result.removed_library_count, 3)
            self.assertEqual(result.patched_consumer_count, 3)

    def test_reports_pre_patch_logical_bytes_when_patchelf_grows_a_duplicate(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            site_packages = root / 'site-packages'
            embedded_lib = root / 'embedded' / 'lib'
            embedded_lib.mkdir(parents=True)
            ssl_duplicate = write_elf(site_packages / 'wheel.libs' / 'libssl-0123abcd.so.3')
            crypto_duplicate = write_elf(site_packages / 'wheel.libs' / 'libcrypto-deadbeef.so.3')
            write_elf(embedded_lib / 'libssl.so.3')
            write_elf(embedded_lib / 'libcrypto.so.3')
            fake_patchelf = FakePatchelf(
                {
                    ssl_duplicate: ['libcrypto-deadbeef.so.3'],
                    crypto_duplicate: [],
                }
            )
            original_size = ssl_duplicate.stat().st_size + crypto_duplicate.stat().st_size

            def growing_patchelf(arguments):
                output = fake_patchelf(arguments)
                if str(arguments[0]) == '--replace-needed':
                    consumer = Path(arguments[-1])
                    consumer.write_bytes(consumer.read_bytes() + b'patchelf growth')
                return output

            result = normalize_auditwheel_libraries(site_packages, embedded_lib, runner=growing_patchelf)

            self.assertEqual(result.removed_logical_bytes, original_size)

    def test_verifies_all_consumers_before_deleting_duplicates(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            site_packages = root / 'site-packages'
            embedded_lib = root / 'embedded' / 'lib'
            embedded_lib.mkdir(parents=True)
            duplicate = write_elf(site_packages / 'wheel.libs' / 'libssl-0123abcd.so.3')
            write_elf(embedded_lib / 'libssl.so.3')
            consumer = write_elf(site_packages / 'package' / 'extension.so')
            runner = FakePatchelf({duplicate: [], consumer: ['libssl-0123abcd.so.3']})
            print_count = 0

            def assert_duplicate_still_exists(_):
                nonlocal print_count
                print_count += 1
                if print_count > 2:
                    self.assertTrue(duplicate.exists())

            runner.on_print_needed = assert_duplicate_still_exists

            normalize_auditwheel_libraries(site_packages, embedded_lib, runner=runner)

            self.assertGreater(print_count, 2)
            self.assertFalse(duplicate.exists())

    def test_fails_before_mutation_when_a_canonical_library_is_missing(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            site_packages = root / 'site-packages'
            embedded_lib = root / 'embedded' / 'lib'
            embedded_lib.mkdir(parents=True)
            duplicate = write_elf(site_packages / 'wheel.libs' / 'libssl-0123abcd.so.3')
            consumer = write_elf(site_packages / 'package' / 'extension.so')
            runner = FakePatchelf({duplicate: [], consumer: ['libssl-0123abcd.so.3']})

            with self.assertRaisesRegex(RuntimeError, 'canonical library is missing.*libssl.so.3'):
                normalize_auditwheel_libraries(site_packages, embedded_lib, runner=runner)

            self.assertTrue(duplicate.exists())
            self.assertFalse(any(call[0] == '--replace-needed' for call in runner.calls))

    def test_keeps_duplicates_when_post_patch_verification_finds_a_stale_consumer(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            site_packages = root / 'site-packages'
            embedded_lib = root / 'embedded' / 'lib'
            embedded_lib.mkdir(parents=True)
            duplicate = write_elf(site_packages / 'wheel.libs' / 'libssl-0123abcd.so.3')
            write_elf(embedded_lib / 'libssl.so.3')
            consumer = write_elf(site_packages / 'package' / 'extension.so')
            runner = FakePatchelf(
                {duplicate: [], consumer: ['libssl-0123abcd.so.3']},
                ignore_replacements=True,
            )

            with self.assertRaisesRegex(RuntimeError, 'stale auditwheel DT_NEEDED.*extension.so'):
                normalize_auditwheel_libraries(site_packages, embedded_lib, runner=runner)

            self.assertTrue(duplicate.exists())

    def test_fails_before_mutation_for_a_hashed_needed_library_that_was_not_discovered(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            root = Path(temporary_directory)
            site_packages = root / 'site-packages'
            embedded_lib = root / 'embedded' / 'lib'
            embedded_lib.mkdir(parents=True)
            write_elf(embedded_lib / 'libssl.so.3')
            consumer = write_elf(site_packages / 'package' / 'extension.so')
            runner = FakePatchelf({consumer: ['libssl-0123abcd.so.3']})

            with self.assertRaisesRegex(RuntimeError, 'has no matching bundled library'):
                normalize_auditwheel_libraries(site_packages, embedded_lib, runner=runner)

            self.assertFalse(any(call[0] == '--replace-needed' for call in runner.calls))


class TestAuditwheelRecipe(unittest.TestCase):
    def test_runs_normalization_after_installing_canonical_dependencies_on_standard_linux(self):
        repository_root = Path(__file__).resolve().parents[4]
        integrations_recipe = (
            repository_root / 'omnibus/config/software/datadog-agent-integrations-py3.rb'
        ).read_text()
        dependencies_recipe = (repository_root / 'omnibus/config/software/datadog-agent-dependencies.rb').read_text()

        self.assertNotIn("tasks/libs/package/auditwheel.py", integrations_recipe)
        self.assertIn("if linux_target? && !fips_mode? && !heroku_target?", dependencies_recipe)
        self.assertLess(
            dependencies_recipe.index("//packages/agent/dependencies:install"),
            dependencies_recipe.index("tasks/libs/package/auditwheel.py"),
        )


if __name__ == '__main__':
    unittest.main()
