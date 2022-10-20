using System.Collections.Generic;
using System.Linq;
using FluentAssertions;
using FluentAssertions.Execution;
using FluentAssertions.Primitives;
using YamlDotNet.RepresentationModel;

namespace CustomActions.Tests.Helpers
{
    public class YamlAssertions : ReferenceTypeAssertions<YamlNode, YamlAssertions>
    {
        public YamlAssertions(YamlNode instance)
            : base(instance)
        {
        }

        protected override string Identifier => "yaml";

        private YamlMappingNode FindLastParentNode(YamlMappingNode currentNode, string[] keys)
        {
            foreach (var key in keys)
            {
                if (currentNode.Children.ContainsKey(key))
                {
                    if (currentNode.Children[key] is YamlMappingNode)
                    {
                        currentNode = (YamlMappingNode)currentNode.Children[key];
                        continue;
                    }
                }

                break;
            }

            return currentNode;
        }

        public AndConstraint<YamlAssertions> HaveKey(
            string key, string because = "", params object[] becauseArgs)
        {
            var keys = key.Split('.');
            var mappingNode = (YamlMappingNode)Subject;
            if (keys.Length > 1)
            {
                // For nested keys, find the last parent node.
                mappingNode = FindLastParentNode(mappingNode, keys);
            }

            Execute.Assertion
                .BecauseOf(because, becauseArgs)
                .Given(() => mappingNode.Children)
                .ForCondition(nodes => nodes.ContainsKey(keys.Last()))
                .FailWith("Expected {context:YAML} to contain key {0}{reason}.", key);

            var child = mappingNode.Children[keys.Last()];
            // Change assertion scope to child node
            return new AndConstraint<YamlAssertions>(new YamlAssertions(child));
        }

        public AndConstraint<YamlAssertions> NotHaveKey(
            string key, string because = "", params object[] becauseArgs)
        {
            var keys = key.Split('.');
            var mappingNode = (YamlMappingNode)Subject;
            if (keys.Length > 1)
            {
                // For nested keys, find the last parent node.
                mappingNode = FindLastParentNode(mappingNode, keys);
            }

            Execute.Assertion
                .BecauseOf(because, becauseArgs)
                .Given(() => mappingNode.Children)
                .ForCondition(nodes => !nodes.ContainsKey(keys.Last()))
                .FailWith("Expected {context:YAML} to NOT contain key {0}{reason}.", key);

            // Don't change assertion scope
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

        public AndConstraint<YamlAssertions> NoValue(
            string because = "", params object[] becauseArgs)
        {
            Execute.Assertion
                .BecauseOf(because, becauseArgs)
                .Given(() => (YamlScalarNode)Subject)
                .ForCondition(node => string.IsNullOrEmpty(node.Value))
                .FailWith("Expected {context} to not have a value {reason}.");

            return new AndConstraint<YamlAssertions>(this);
        }

        public AndConstraint<YamlAssertions> HaveValues(
            IList<string> values, string because = "", params object[] becauseArgs)
        {
            Execute.Assertion
                .BecauseOf(because, becauseArgs)
                .Given(() => ((YamlSequenceNode)Subject))
                .ForCondition(subject => subject.Children.Cast<YamlScalarNode>().Select(k => k.Value).SequenceEqual(values))
                .FailWith("Expected {context} to equal {0}{reason}", values);

            return new AndConstraint<YamlAssertions>(this);
        }
    }
}
