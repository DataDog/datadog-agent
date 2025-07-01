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
    # Sort tests by descending depth (number of '/' in test name)
    tests_sorted = sorted(tests, key=lambda t: len(t[1].split('/')))
    leaf_tests: set[tuple[str, str]] = set()
    for candidate_test in tests_sorted:
        # If none of the known leaf tests is a child of the candidate test, then the candidate test is a leaf
        is_leaf = all(not _is_child(candidate_test, known_leaf_test) for known_leaf_test in leaf_tests)
        if is_leaf:
            leaf_tests.add(candidate_test)
    return leaf_tests


def _is_child(test1: tuple[str, str], test2: tuple[str, str]) -> bool:
    """
    Returns True if test1 is a child of test2.
    Example: is_child(("pkg", "TestAgent/TestFeatureA"), ("pkg", "TestAgent")) == True
    """
    if test1[0] != test2[0]:
        return False
    splitted_test1 = test1[1].split('/')
    splitted_test2 = test2[1].split('/')
    if len(splitted_test2) > len(splitted_test1):
        return False

    for part1, part2 in zip(splitted_test1, splitted_test2, strict=False):
        if part1 != part2:
            return False
    return True
