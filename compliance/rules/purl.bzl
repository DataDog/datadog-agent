"""Convenience wrappers around @package_metadata//purl:purl.bzl's purl.builder()."""

load("@package_metadata//purl:purl.bzl", "purl")

def purl_for_generic(package, version, download_url):
    url = download_url.format(version = version)
    return purl.builder().type("generic").name(package).version(version).add_qualifier("download_url", url).build()

def purl_for_golang(module):
    # split, since purl.builder() percent-encodes a "/" within the name, unlike rules_go's reader
    namespace, _, name = module.rpartition("/")
    return purl.builder().type("golang").namespace(namespace).name(name).build()
