import pip

DATADOG_CHECK_PREFIX = "datadog-"

def get_installed_packages():
    return pip.get_installed_distributions()

def get_datadog_wheels():
    packages = []
    dist = get_installed_packages()
    for package in dist:
        if package.project_name.startswith(DATADOG_CHECK_PREFIX):
            name = package.project_name[len(DATADOG_CHECK_PREFIX):].replace('-', '_')
            packages.append(name)

    return packages
