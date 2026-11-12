"""Shared helpers for aspects that walk the CC dependency graph.

Both the dependency-collection aspect (dd_collect_dependencies) and the
debug-symbol-collection aspect (dd_strip) traverse the same CC dependency
graph. The constants and helpers below capture the parts of that traversal
that are common to all collectors; collectors with additional needs extend
CC_GRAPH_WALKED_ATTRS with their own extras.
"""

# Attributes describing the edges of the CC dependency graph.  Aspects
# walking that graph should propagate along all of these:
# - dynamic_deps: cc_shared_library -> cc_shared_library edges
# - input: _dd_cc_packaged_rule -> cc_shared_library edges (bridges a
#   packaged target back to its underlying cc_shared_library)
# - shared_library: cc_import -> _dd_cc_packaged_rule edges (a cc_import
#   bridge pointed at an already-packaged shared library)
# - embed, deps: go_library/go_binary edges, so a walk can start from a
#   real Go binary
# - cdeps: go_library/go_binary -> cc_library edges (the cgo boundary)
# - data: cc_library -> _dd_cc_packaged_rule edges (a runtime, dlopen'd
#   dependency rather than a link-time one)
CC_GRAPH_WALKED_ATTRS = [
    "dynamic_deps",
    "input",
    "shared_library",
    "embed",
    "deps",
    "cdeps",
    "data",
]

def get_deps(rule_attributes, attr_names):
    """Return the targets reachable through the given attr_names.

    Handles both scalar and list attributes, and silently skips attributes
    the visited rule does not define.

    Args:
      rule_attributes: struct containing the attributes for the target rule
      attr_names: attribute names from which to extract dependencies

    Returns:
      reachable targets
    """
    deps = []
    for attr_name in attr_names:
        val = getattr(rule_attributes, attr_name, None)
        if val == None:
            continue
        if type(val) == "list":
            deps.extend(val)
        else:
            deps.append(val)
    return deps
