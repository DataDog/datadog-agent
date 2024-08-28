import os
import pwd
import grp
import importlib.metadata
import pkg_resources
from packaging import version

def extract_version(specifier):
    """
    Extract version from the specifier string.
    """
    try:
        # Get the first version specifier from the specifier string
        return str(next(iter(pkg_resources.Requirement.parse(f'{specifier}').specifier)))
    except Exception:
        return None
    
def create_python_installed_packages_file(filename):
    """
    Create a file listing the currently installed Python dependencies.
    """
    with open(filename, 'w', encoding='utf-8') as f:
        installed_packages = importlib.metadata.distributions()
        for dist in installed_packages:
            f.write(f"{dist.metadata['Name']}=={dist.version}\n")
    os.chown(filename, pwd.getpwnam('dd-agent').pw_uid, grp.getgrnam('dd-agent').gr_gid)

def create_diff_installed_packages_file(postinst_file, prerm_file, diff_file):
    """
    Create a file listing the new or upgraded Python dependencies.
    """
    postinst_python_installed_packages = load_requirements(postinst_file)
    prerm_python_installed_packages = load_requirements(prerm_file)
    with open(diff_file, 'w', encoding='utf-8') as f:
        for package_name, prerm_req in prerm_python_installed_packages.items():
            postinst_req = postinst_python_installed_packages.get(package_name)
            if postinst_req:
                # Extract and compare versions
                postinst_version_str = extract_version(str(postinst_req.specifier))
                prerm_version_str = extract_version(str(prerm_req.specifier))
                if postinst_version_str and prerm_version_str:
                    if version.parse(prerm_version_str) > version.parse(postinst_version_str):
                        f.write(f"{prerm_req}\n")
            else:
                # Package is new in the new file; include it
                f.write(f"str({prerm_req})\n")

def load_requirements(filename):
    """
    Load requirements from a file.
    """
    with open(filename, 'r', encoding='utf-8') as f:
        return {req.name: req for req in pkg_resources.parse_requirements(f)}
