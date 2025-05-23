"""Module regrouping all invoke tasks used for linting the `datadog-agent` repo"""

from .go import go, update_go  # noqa: F401
from .misc import copyrights, filenames  # noqa: F401
from .old import *  # noqa: F403
from .python import python  # noqa: F401
