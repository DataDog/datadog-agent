import os
import pwd
import grp
import importlib.metadata
import pkg_resources

def create_python_installed_packages_file(filename):
    """
    Create a file listing the currently installed Python dependencies.
    """
    with open(filename, 'w', encoding='utf-8') as f:
        installed_packages = importlib.metadata.distributions()
        for dist in installed_packages:
            f.write(f"{dist.metadata['Name']}=={dist.version}\n")
    os.chown(filename, pwd.getpwnam('dd-agent').pw_uid, grp.getgrnam('dd-agent').gr_gid)

def load_requirements(filename):
    """
    Load requirements from a file.
    """
    with open(filename, 'r', encoding='utf-8') as f:
        return {req.name: req for req in pkg_resources.parse_requirements(f)}
