import unittest
from unittest.mock import MagicMock, patch

from invoke.context import MockContext, Result

from tasks.issue import add_reviewers
from tasks.libs.issue.assign import guess_from_keywords, guess_from_labels


# We must define this class as we cannot override the name attribute in MagicMock
class Label:
    def __init__(self, name):
        self.name = name


class TestGuessFromLabels(unittest.TestCase):
    def test_with_team(self):
        issue = MagicMock(labels=[Label(name="team/triage"), Label(name="team/core")])

        self.assertEqual(guess_from_labels(issue), "core")

    def test_without_team(self):
        issue = MagicMock(labels=[Label(name="team/triage"), Label(name="team:burton")])

        self.assertEqual(guess_from_labels(issue), "triage")


class TestGuessFromKeywords(unittest.TestCase):
    def test_from_simple_match(self):
        issue = MagicMock(title="I have an issue", body="I can't get any logs from the agent.")
        self.assertEqual(guess_from_keywords(issue), "agent-log-pipelines")

    def test_with_a_file(self):
        issue = MagicMock(title="fix bug", body="It comes from the file pkg/agent/build.py")
        self.assertEqual(guess_from_keywords(issue), "agent-runtimes")

    def test_no_match(self):
        issue = MagicMock(title="fix bug", body="It comes from the file... hm I don't know.")
        self.assertEqual(guess_from_keywords(issue), "triage")


class TestAddReviewers(unittest.TestCase):
    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_dependabot_only(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "InvisibleMan"
        gh_mock.repo.get_pull.return_value = pr_mock
        c = MockContext()
        add_reviewers(c, 1234)
        print_mock.assert_called_once_with("This is not a (dependabot) bump PR, this action should not be run on it.")

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_single_dependency_one_reviewer(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump github.com/redis/go-redis/v9 from 9.1.0 to 9.7.0"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result("""pkg/network/protocols/redis/client.go
pkg/network/usm/tests/tracer_usm_linux_test.go
""")
            }
        )
        add_reviewers(c, 1234)
        print_mock.assert_not_called()
        pr_mock.create_review_request.assert_called_once_with(team_reviewers=['universal-service-monitoring'])

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_single_dependency_several_reviewers(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump github.com/go-delve/delve from 1.6.0 to 1.7.0"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result("""generate_tools.go
pkg/dynamicinstrumentation/diconfig/dwarf.go
pkg/network/go/asmscan/scan.go
pkg/network/go/bininspect/dwarf.go
pkg/network/go/bininspect/newproc.go
pkg/network/go/bininspect/types.go
pkg/network/go/bininspect/utils.go
pkg/network/go/dwarfutils/compile_unit.go
pkg/network/go/dwarfutils/locexpr/exec.go
pkg/network/go/dwarfutils/type_finder.go
pkg/network/go/goid/goid_offset.go
pkg/network/go/goversion/version.go
pkg/network/go/lutgen/run.go
pkg/network/protocols/http/gotls/lookup/luts.go""")
            }
        )
        add_reviewers(c, 1234)
        print_mock.assert_not_called()
        self.assertCountEqual(
            pr_mock.create_review_request.call_args[1]['team_reviewers'],
            ['universal-service-monitoring', 'debugger', 'agent-devx-infra'],
        )

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_group_dependency(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump the aws-sdk-go-v2 group with 5 updates"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result("""pkg/databasemonitoring/aws/aurora.go
pkg/databasemonitoring/aws/aurora_test.go
pkg/databasemonitoring/aws/client.go
pkg/databasemonitoring/aws/rdsclient_mockgen.go
pkg/serverless/apikey/api_key.go
pkg/serverless/apikey/api_key_test.go
pkg/serverless/trace/inferredspan/propagation_test.go
pkg/serverless/trace/propagation/carriers_test.go
pkg/serverless/trace/propagation/extractor_test.go
pkg/serverless/trigger/extractor.go
pkg/util/ec2/ec2_tags.go
test/new-e2e/examples/ecs_test.go
test/new-e2e/go.mod
test/new-e2e/pkg/provisioners/aws/kubernetes/kubernetes_dump.go
test/new-e2e/pkg/runner/parameters/store_aws.go
test/new-e2e/pkg/utils/clients/aws.go
test/new-e2e/pkg/utils/e2e/client/ecs/ecs.go
test/new-e2e/pkg/utils/e2e/client/ecs/session-manager-plugin.go
test/new-e2e/tests/containers/ecs_test.go
test/new-e2e/tests/windows/common/pipeline/pipeline.go""")
            }
        )
        add_reviewers(c, 1234)
        print_mock.assert_not_called()
        self.assertCountEqual(
            pr_mock.create_review_request.call_args[1]['team_reviewers'],
            [
                'windows-agent',
                'database-monitoring',
                'container-integrations',
                'agent-devx-loops',
                'serverless',
                'container-platform',
                'windows-kernel-integrations',
                'agent-runtimes',
                'agent-e2e-testing',
                'serverless-aws',
            ],
        )

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_group_dependency_scoped(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump the aws-sdk-go-v2 group in /test/new-e2e with 5 updates"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result("""pkg/databasemonitoring/aws/aurora.go
pkg/databasemonitoring/aws/aurora_test.go
pkg/databasemonitoring/aws/client.go
pkg/databasemonitoring/aws/rdsclient_mockgen.go
pkg/serverless/apikey/api_key.go
pkg/serverless/apikey/api_key_test.go
pkg/serverless/trace/inferredspan/propagation_test.go
pkg/serverless/trace/propagation/carriers_test.go
pkg/serverless/trace/propagation/extractor_test.go
pkg/serverless/trigger/extractor.go
pkg/util/ec2/ec2_tags.go
test/new-e2e/examples/ecs_test.go
test/new-e2e/go.mod
test/new-e2e/pkg/provisioners/aws/kubernetes/kubernetes_dump.go
test/new-e2e/pkg/runner/parameters/store_aws.go
test/new-e2e/pkg/utils/clients/aws.go
test/new-e2e/pkg/utils/e2e/client/ecs/ecs.go
test/new-e2e/pkg/utils/e2e/client/ecs/session-manager-plugin.go
test/new-e2e/tests/containers/ecs_test.go
test/new-e2e/tests/windows/common/pipeline/pipeline.go""")
            }
        )
        add_reviewers(c, 1234)
        print_mock.assert_not_called()
        self.assertCountEqual(
            pr_mock.create_review_request.call_args[1]['team_reviewers'],
            [
                'windows-agent',
                'container-integrations',
                'agent-devx-loops',
                'container-platform',
                'windows-kernel-integrations',
                'agent-e2e-testing',
            ],
        )
