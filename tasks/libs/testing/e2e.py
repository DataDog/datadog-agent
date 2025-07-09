from collections.abc import Iterable


def create_test_selection_regex(test_names: list[str]) -> str:
    """
    Create a regex to exact-match the tests in the targets list.
    Ex: ["TestFoo", "TestBar"] -> "^(?:TestFoo|TestBar)$"
    """
    if not test_names:
        return ""

    # Remove any whitespace and eventual ^$ surrounding the test names
    processed_names = [name.strip().strip("^$") for name in test_names]

    # Join them with a pipe to create an OR regex
    regex_body = "|".join(processed_names)
    if len(processed_names) > 1:
        # If we have more than one test, we need to group them with a non-capturing group
        regex_body = f"(?:{regex_body})"

    return f'"^{regex_body}$"'


def filter_only_leaf_tests(tests: Iterable[tuple[str, str]]) -> set[tuple[str, str]]:
    """
    Given some (package, test_name) tuples, return only the leaf tests.
    A test is a leaf if it is not a parent of any other test in the list (within the same package).
    """
    # Sort tests by depth (number of '/' in test name) - deepest tests first
    tests_sorted = sorted(tests, key=lambda t: len(t[1].split('/')), reverse=True)
    leaf_tests: set[tuple[str, str]] = set()
    for candidate_test in tests_sorted:
        # Check if candidate_test is a leaf test
        # candidate_test will itself be a leaf if no known leaf test is a child of it.
        # This works because we are iterating from deepest to shallowest test
        is_leaf = all(not _is_child(candidate_test, known_leaf_test) for known_leaf_test in leaf_tests)
        if is_leaf:
            leaf_tests.add(candidate_test)
    return leaf_tests


def _is_child(candidate_parent: tuple[str, str], candidate_child: tuple[str, str]) -> bool:
    """
    Returns True if candidate_child is a child of candidate_parent.
    Example: is_child(("pkg", "TestAgent"), ("pkg", "TestAgent/TestFeatureA")) == True
    """
    if candidate_parent[0] != candidate_child[0]:
        return False
    child_test_parts = candidate_child[1].split('/')
    parent_test_parts = candidate_parent[1].split('/')
    if len(parent_test_parts) > len(child_test_parts):
        return False

    for part1, part2 in zip(child_test_parts, parent_test_parts, strict=False):
        if part1 != part2:
            return False
    return True
