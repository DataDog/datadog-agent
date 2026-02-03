from .common import get_architectures, get_default_architecture, get_default_os_family, get_os_families

instance_type: str = "The instance type to use (default is Standard_B4ms for azure)"
os_family: str = f"The operating system. Possible values are {get_os_families()}. Default '{get_default_os_family()}'"
architecture: str = (
    f"The architecture to use. Possible values are {get_architectures()}. Default '{get_default_architecture()}'"
)
