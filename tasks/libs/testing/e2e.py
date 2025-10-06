from collections.abc import Iterable


def create_test_selection_gotest_regex(test_names: list[str]) -> str:
    """
    Create a gotest-compatible regex to exact-match the tests in the targets list.
    Note that go test handles "/" quite specially:
    - The argument is first split by "/", with each part being its own regex
    - Each part then gets matched against the corresponding segment of the test name.
    - If everything matches, the test is selected for execution.
    See https://datadoghq.atlassian.net/wiki/x/rAX-0 for more details.

    Each part is a regex, so we need to add ^$ around "/"s to ensure every segment is exact-matched.

    Note that in some cases the produced regex might select tests that are not in the original list.
    For example, if the input is ["TestFoo", "TestBar/Ba", "TestBar/Baz"], the produced regex will be:
    `"^(?:TestFoo|TestBar)$/^(?:Ba|Baz)$"`
    But this regex would also match "TestFoo/Ba" for example, which is not in the original list.

    Ex: ["TestFoo", "TestBar/Ba", "TestBar/Baz"] -> "^(?:TestFoo|TestBar)$/^(?:Ba|Baz)$"
    """
    if not test_names:
        return ""

    # Split the test names into component lists (handle each segment around "/" separately)
    test_components = [name.split('/') for name in test_names]

    # Pad all component lists to the same length with empty strings
    max_length = max(len(components) for components in test_components)
    padded_components = [components + [""] * (max_length - len(components)) for components in test_components]

    # We now have a rectangular matrix of components, where each row corresponds to a test name
    # and each column corresponds to a same-level segment of the test name.
    # Going column by column, we will create a regex for each segment.
    regex_components = []
    for col in range(max_length):
        # Get all non-None components for the current column
        # Use a set to avoid duplicates
        column_components: set[str] = {components[col] for components in padded_components if components[col]}

        # Sort the components alphabetically to ensure consistent ordering
        column_components_sorted = sorted(column_components)

        component = f"^(?:{'|'.join(column_components_sorted)})$"
        regex_components.append(component)

    regex_body = '/'.join(regex_components)
    return f'"{regex_body}"'


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
