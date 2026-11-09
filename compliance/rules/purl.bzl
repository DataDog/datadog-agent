"""Convenience wrappers around @package_metadata//purl:purl.bzl's purl.builder()."""

load("@package_metadata//purl:purl.bzl", "purl")

def purl_for_generic(package, version, download_url):
    url = download_url.format(version = version)
    return purl.builder().type("generic").name(package).version(version).add_qualifier("download_url", url).build()

def purl_for_golang(module_path, subpath = None, version = None):
    """Returns the purl of a Go module, or of a package within it.

    Example: purl_for_golang("github.com/DataDog/datadog-agent", subpath = "cmd/foo")
    returns "pkg:golang/github.com/DataDog/datadog-agent#cmd/foo".

    Args:
      module_path: Go module path, e.g. "github.com/DataDog/datadog-agent".
      subpath: optional package directory within the module.
      version: optional module version.

    Returns:
      The purl string.
    """
    namespace, _, name = module_path.rpartition("/")
    b = purl.builder().type("golang").name(name)
    if namespace:
        b = b.namespace(namespace)
    if version:
        b = b.version(version)
    if subpath:
        b = b.subpath(subpath)
    return b.build()
