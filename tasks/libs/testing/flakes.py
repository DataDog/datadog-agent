def get_tests_family_if_failing_tests(test_name_list, failing_tests: set):
    """Get the parent tests of a list of tests only if the marked test is failing

    For example with the test ["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestCPU"]
    this method should return the set{"TestEKSSuite/TestCPU/TestCPUUtilization", "TestEKSSuite/TestCPU", "TestEKSSuite", "TestKindSuite/TestCPU", "TestKindSuite"}
    if TestKindSuite/TestCPU and TestEKSSuite/TestCPU/TestCPUUtilization are failing
    Another example, with the test ["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestCPU"]
    if only TestKindSuite/TestCPU is failing, the method should return the set{"TestKindSuite/TestCPU", "TestKindSuite"}

    Args:
        test_name_list (list): List of test names to get the parent tests from
        failing_tests (set): Set of tests that are failing
    """
    test_name_set = set(test_name_list)
    marked_tests_failing = failing_tests.intersection(test_name_set)
    return get_tests_family(list(marked_tests_failing))


def get_tests_family(test_name_list):
    """Get the parent tests of a list of tests

    Get the parent tests of a list of tests
    For example with the test ["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestCPU"]
    this method should return the set{"TestEKSSuite/TestCPU/TestCPUUtilization", "TestEKSSuite/TestCPU", "TestEKSSuite", "TestKindSuite/TestCPU", "TestKindSuite"}

    Args:
        test_name_list (list): List of test names to get the parent tests from

    """
    test_family = set(test_name_list)
    for test_name in test_name_list:
        while test_name.count('/') > 0:
            test_name = test_name.rsplit('/', 1)[0]
            test_family.add(test_name)
    return test_family


def is_known_flaky_test(failing_test, known_flaky_tests, known_flaky_tests_parents):
    """Check if a test is known to be flaky

     If a test is a parent of a test that is known to be flaky, the test should be considered flaky
    For example:
    - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite/TestCPU/TestCPUUtilization should be considered flaky
    - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite should be considered flaky unless TestEKSSuite/TestCPU is not failing
    - if TestEKSSuite/TestCPU is known to be flaky, TestEKSSuite/TestMemory should not be considered flaky

    Args:
        failing_test (str): The test that is failing
        known_flaky_tests (set): Set of tests that are known to be flaky
        known_flaky_tests_parents (set): Set of tests that are parent of a test that is known to be flaky
    """

    failing_test_parents = get_tests_family([failing_test])

    if any(parent in known_flaky_tests for parent in failing_test_parents):
        return True

    return failing_test in known_flaky_tests_parents


def consolidate_flaky_failures(flaky_failures: set, failing_tests: set) -> set:
    """
    Consolidate flaky failures.
    Return a set with all the failures that are due to flakiness.
    If a failing test has only children that are failing because of flakiness, the failing test should be considered flaky as well.
    If a failing has two children failing because of flakiness and one child that is failing because of a real issue, the failing test should not be considered flaky.
    """
    new_flaky_failures = set(flaky_failures)
    list_failing_tests = list(failing_tests)
    list_failing_tests.sort(key=lambda x: len(x.split('/')), reverse=True)
    for test in list_failing_tests:
        if test in flaky_failures:
            continue
        for flaky_failure in flaky_failures:
            if is_strict_child(flaky_failure, test):
                new_flaky_failures.add(test)
                break
        children = get_child_test_in_list(test, failing_tests)
        if not children:  # If the test has no children it cannot be failed because of a flaky child
            continue
        has_non_flaky_failing_child = False
        for child in children:
            if child not in new_flaky_failures:
                has_non_flaky_failing_child = True
        if not has_non_flaky_failing_child:
            new_flaky_failures.add(test)

    return new_flaky_failures


def get_child_test_in_list(test_name, test_list):
    """Get the child test in a list of tests

    Get the child test in a list of tests
    For example with the test "TestEKSSuite/TestCPU" and the list ["TestEKSSuite/TestCPU/TestCPUUtilization", "TestKindSuite/TestCPU", "TestEKSSuite/TestCPU"]
    this method should return the test "TestEKSSuite/TestCPU/TestCPUUtilization"

    Args:
        test_name (str): Test name to get the child test from
        test_list (list): List of test names to get the child test from

    """
    children = []
    for test in test_list:
        if is_strict_child(test_name, test):
            children.append(test)
    return children


def is_strict_child(parent, child):
    """Check if a test is a child of another test

    Check if a test is a child of another test
    For example with the test "TestEKSSuite/TestCPU" and the test "TestEKSSuite/TestCPU/TestCPUUtilization"
    this method should return True

    Args:
        parent (str): Test name to check if it is the parent
        child (str): Test name to check if it is the child

    """

    splitted_parent = parent.split('/')
    splitted_child = child.split('/')
    if len(splitted_parent) >= len(splitted_child):
        return False
    for i in range(len(splitted_parent)):
        if splitted_parent[i] != splitted_child[i]:
            return False
    return True
