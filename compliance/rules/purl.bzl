"""Functions for creating PURLs.

If these work well for us, we'll propose them for inclusion in bazel-contrib/supply_chain
"""

def purl_for_generic(package, version, download_url):
    # TODO: This must be http-encoded. Do that once the function is available from
    # supply_chain
    url = download_url.format(version = version)
    return "pkg:generic/{package}@{version}?download_url={url}".format(
        package = package,
        version = version,
        url = url,
    )
