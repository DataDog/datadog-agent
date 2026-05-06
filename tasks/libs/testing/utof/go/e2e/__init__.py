"""Go e2e test format — public API re-exports."""

from tasks.libs.testing.utof.go.e2e.converter import convert_e2e_test_results
from tasks.libs.testing.utof.metadata import generate_metadata

__all__ = [
    "convert_e2e_test_results",
    "generate_metadata",
]
