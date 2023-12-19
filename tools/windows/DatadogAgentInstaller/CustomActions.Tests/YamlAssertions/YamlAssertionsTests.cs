using CustomActions.Tests.Helpers;
using Xunit;

namespace CustomActions.Tests.YamlAssertions
{
    /// <summary>
    /// Tests for the custom YAML assertions
    /// </summary>
    public class YamlAssertionsTests
    {
        [Fact]
        public void Should_Succeed_With_NonNestedKey()
        {
            "not_nested_at_all:".ToYaml().Should().HaveKey("not_nested_at_all");
        }

        [Fact]
        public void Should_Succeed_With_NestedKey()
        {
            var yaml = @"
deeply:
    nested:
        property:
            test: 42
            ".ToYaml();

            yaml.Should()
                .HaveKey("deeply.nested.property.test")
                .And.HaveValue("42");

            // The above is equivalent to
            yaml.Should()
                .HaveKey("deeply")
                .And.HaveKey("nested")
                .And.HaveKey("property")
                .And.HaveKey("test")
                .And.HaveValue("42");
        }

        [Fact]
        public void Should_Not_HaveValue()
        {
            "no_value:".ToYaml().Should().HaveKey("no_value").And.NoValue();
        }

        [Fact]
        public void Should_Not_HaveKey()
        {
            "test: 42".ToYaml().Should().HaveKey("test").And.NotHaveValue("32");
        }

        [Fact]
        public void Should_HaveValues_Correctly_Evaluate_List()
        {
            @"test_list:
                - elementA
                - anotherElementCalledB"
                .ToYaml()
                .Should()
                .HaveKey("test_list")
                .And.HaveValues(new[] { "elementA", "anotherElementCalledB" });
        }
    }
}
