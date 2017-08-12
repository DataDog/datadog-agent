"""
Invoke entrypoint, import here all the tasks we want to make available
"""
from .go import fmt, lint, vet
from .test import test
