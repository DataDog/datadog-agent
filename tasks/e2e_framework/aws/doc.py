from .common import get_architectures, get_default_architecture, get_default_os_family, get_os_families

os_family: str = f"The operating system. Possible values are {get_os_families()}. Default '{get_default_os_family()}'"
ami_id: str = "A full Amazon Machine Image (AMI) id (e.g. ami-0123456789abcdef0)"
architecture: str = (
    f"The architecture to use. Possible values are {get_architectures()}. Default '{get_default_architecture()}'"
)
instance_type: str = "The instance type to use (default is t3.medium for aws)"
use_fargate: str = "Use Fargate (default True)"
