using FluentAssertions;
using FluentAssertions.Execution;
using FluentAssertions.Primitives;
using YamlDotNet.RepresentationModel;

namespace CustomActions.Tests
{
    public class YamlAssertions : ReferenceTypeAssertions<YamlNode, YamlAssertions>
    {
        public YamlAssertions(YamlNode instance)
            : base(instance)
        {
        }

        protected override string Identifier => "yaml";

        public AndConstraint<YamlAssertions> HaveKey(
            string key, string because = "", params object[] becauseArgs)
        {
            Execute.Assertion
                .BecauseOf(because, becauseArgs)
                .Given(() => ((YamlMappingNode)Subject).Children)
                .ForCondition(nodes => nodes.ContainsKey(key))
                .FailWith("Expected {context:YAML} to contain key {0}{reason}.", key);

            var child = ((YamlMappingNode)Subject).Children[key];
            return new AndConstraint<YamlAssertions>(new YamlAssertions(child));
        }

        public AndConstraint<YamlAssertions> NotHaveKey(
            string key, string because = "", params object[] becauseArgs)
        {
            Execute.Assertion
                .BecauseOf(because, becauseArgs)
                .Given(() => ((YamlMappingNode)Subject).Children)
                .ForCondition(nodes => !nodes.ContainsKey(key))
                .FailWith("Expected {context:YAML} to NOT contain key {0}{reason}.", key);

            return new AndConstraint<YamlAssertions>(this);
        }

        public AndConstraint<YamlAssertions> HaveValue(
            string value, string because = "", params object[] becauseArgs)
        {
            Execute.Assertion
                .BecauseOf(because, becauseArgs)
                .Given(() => ((YamlScalarNode)Subject).Value)
                .ForCondition(subjectValue => subjectValue == value)
                .FailWith("Expected {context} to equal {0}{reason} but was {1}", value, ((YamlScalarNode)Subject).Value);

            return new AndConstraint<YamlAssertions>(this);
        }

        public AndConstraint<YamlAssertions> NotHaveValue(
            string value, string because = "", params object[] becauseArgs)
        {
            Execute.Assertion
                .BecauseOf(because, becauseArgs)
                .Given(() => ((YamlScalarNode)Subject).Value)
                .ForCondition(nodes => nodes != value)
                .FailWith("Expected {context} to NOT be equal to {0}{reason}.", value);

            return new AndConstraint<YamlAssertions>(this);
        }
    }
}
