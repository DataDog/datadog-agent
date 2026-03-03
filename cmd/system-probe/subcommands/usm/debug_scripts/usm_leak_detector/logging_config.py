"""
Logging configuration for the USM leak detector.
"""

import logging

logger = logging.getLogger("usm_leak_detector")


def configure_logging(verbose: bool = False):
    """Configure logging level based on verbosity.

    Args:
        verbose: If True, set logging level to DEBUG. Otherwise, WARNING.
    """
    level = logging.DEBUG if verbose else logging.WARNING
    logging.basicConfig(
        level=level,
        format="%(message)s"
    )
    logger.setLevel(level)
