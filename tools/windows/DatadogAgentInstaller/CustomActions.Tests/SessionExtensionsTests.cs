using Datadog.CustomActions.Extensions;
using FluentAssertions;
using Xunit;

namespace CustomActions.Tests
{
    public class SessionExtensionsTests
    {
        [Theory]
        // A path or an account name in a message must survive being formatted by the installer
        [InlineData(@"C:\ProgramData\Datadog", @"C:\ProgramData\Datadog")]
        [InlineData(@"C:\a[b]c\Datadog", @"C:\a[\[]b]c\Datadog")]
        [InlineData("[ProductName] is owned by [x", @"[\[]ProductName] is owned by [\[]x")]
        // Closing brackets and braces are not substitution markers
        [InlineData("a]b{c}", "a]b{c}")]
        [InlineData("", "")]
        public void EscapeMsiFormat_Escapes_Substitution_Markers(string input, string expected)
        {
            input.EscapeMsiFormat().Should().Be(expected);
        }

        [Fact]
        public void EscapeMsiFormat_Handles_Null()
        {
            ((string)null).EscapeMsiFormat().Should().BeNull();
        }
    }
}
