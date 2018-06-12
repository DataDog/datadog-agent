import pkg_resources

DATADOG_CHECK_PREFIX = "datadog-"

def get_installed_packages():
    dists = [d for d in pkg_resources.working_set]
    return dists

def get_datadog_wheels():
    packages = []
    dist = get_installed_packages()
    for package in dist:
        if package.project_name.startswith(DATADOG_CHECK_PREFIX):
            name = package.project_name[len(DATADOG_CHECK_PREFIX):].replace('-', '_')
            packages.append(name)

    return packages
