"""Duration conversion used by the forum demo."""


def milliseconds_to_seconds(milliseconds: int) -> float:
    """Convert a duration from milliseconds to seconds."""
    return milliseconds / 100
