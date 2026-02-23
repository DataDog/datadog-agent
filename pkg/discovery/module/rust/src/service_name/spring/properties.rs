// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use nom::{
    IResult,
    branch::alt,
    bytes::complete::take_while1,
    character::complete::{char, one_of, space0},
    combinator::{eof, not, peek, rest, value},
};
use std::io::{BufRead, BufReader, Read};

/// Parser for Java .properties filesthat searches for a specific key.  It's a
/// minimal parser that only supports what should be needed for the Spring
/// application name.
pub fn parse_properties<R: Read>(reader: R, target_key: &str) -> Option<String> {
    let buf_reader = BufReader::new(reader);

    for line_result in buf_reader.lines() {
        let line = line_result.ok()?;
        if let Some((key, value)) = parse_property_line(&line)
            && key == target_key
        {
            return Some(value);
        }
    }

    None
}

fn parse_property_line(line: &str) -> Option<(String, String)> {
    property_parser(line).ok().map(|(_, (k, v))| (k, v))
}

fn property_parser(input: &str) -> IResult<&str, (String, String)> {
    let (input, _) = space0(input)?;
    // Fail if we're at EOF or at a comment character (# or !)
    let (input, _) = not(peek(alt((value((), eof), value((), one_of("#!"))))))(input)?;
    let (input, key) = take_while1(|c: char| !c.is_whitespace() && c != '=' && c != ':')(input)?;
    let (input, _) = space0(input)?;
    let (input, _) = alt((char('='), char(':')))(input)?;
    let (input, _) = space0(input)?;
    let (input, value) = rest(input)?;
    Ok((input, (key.to_string(), value.trim().to_string())))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_spring_application_name_first() {
        let props = "spring.application.name=myapp\nserver.port=8080\nother.key=value".as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, Some("myapp".to_string()));
    }

    #[test]
    fn test_spring_application_name_in_middle() {
        let props =
            "server.port=8080\nspring.application.name=myservice\nother.key=value".as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, Some("myservice".to_string()));
    }

    #[test]
    fn test_spring_application_name_at_end() {
        let props = "server.port=8080\nother.key=value\nspring.application.name=backend".as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, Some("backend".to_string()));
    }

    #[test]
    fn test_spring_application_name_after_comments() {
        let props = "# Configuration file\n# Author: Someone\nserver.port=8080\nspring.application.name=webapp\n".as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, Some("webapp".to_string()));
    }

    #[test]
    fn test_spring_application_name_with_many_properties() {
        let props = "database.url=jdbc:postgresql://localhost/db\n\
                     database.user=admin\n\
                     database.password=secret\n\
                     server.port=8080\n\
                     server.host=0.0.0.0\n\
                     spring.application.name=complex-app\n\
                     logging.level=INFO\n\
                     cache.enabled=true"
            .as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, Some("complex-app".to_string()));
    }

    #[test]
    fn test_property_not_found() {
        let props = "server.port=8080\nother.key=value".as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, None);
    }

    #[test]
    fn test_empty_file() {
        let props = "".as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, None);
    }

    #[test]
    fn test_only_comments() {
        let props = "# Comment 1\n# Comment 2\n# Comment 3".as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, None);
    }

    #[test]
    fn test_parse_property_line_with_equals() {
        assert_eq!(
            parse_property_line("key=value"),
            Some(("key".to_string(), "value".to_string()))
        );
    }

    #[test]
    fn test_parse_property_line_with_colon() {
        assert_eq!(
            parse_property_line("key:value"),
            Some(("key".to_string(), "value".to_string()))
        );
    }

    #[test]
    fn test_parse_property_line_with_whitespace() {
        assert_eq!(
            parse_property_line("  key = value  "),
            Some(("key".to_string(), "value".to_string()))
        );
        assert_eq!(
            parse_property_line("key   :   value"),
            Some(("key".to_string(), "value".to_string()))
        );
    }

    #[test]
    fn test_parse_property_line_with_dotted_key() {
        assert_eq!(
            parse_property_line("spring.application.name=myapp"),
            Some(("spring.application.name".to_string(), "myapp".to_string()))
        );
    }

    #[test]
    fn test_parse_property_line_with_hyphenated_value() {
        assert_eq!(
            parse_property_line("spring.application.name=my-service-app"),
            Some((
                "spring.application.name".to_string(),
                "my-service-app".to_string()
            ))
        );
    }

    #[test]
    fn test_parse_property_line_with_value_containing_spaces() {
        assert_eq!(
            parse_property_line("spring.application.name=My Service App"),
            Some((
                "spring.application.name".to_string(),
                "My Service App".to_string()
            ))
        );
    }

    #[test]
    fn test_parse_property_line_comments() {
        assert_eq!(parse_property_line("#comment"), None);
        assert_eq!(parse_property_line("# comment with space"), None);
        assert_eq!(parse_property_line("  # indented comment"), None);
        assert_eq!(parse_property_line("!comment"), None);
        assert_eq!(parse_property_line("! comment with space"), None);
        assert_eq!(parse_property_line("  ! indented comment"), None);
    }

    #[test]
    fn test_parse_property_line_empty() {
        assert_eq!(parse_property_line(""), None);
        assert_eq!(parse_property_line("   "), None);
    }

    #[test]
    fn test_value_with_equals_in_it() {
        assert_eq!(
            parse_property_line("key=value=with=equals"),
            Some(("key".to_string(), "value=with=equals".to_string()))
        );
    }

    #[test]
    fn test_value_with_colon_in_it() {
        assert_eq!(
            parse_property_line("database.url:jdbc:postgresql://localhost/db"),
            Some((
                "database.url".to_string(),
                "jdbc:postgresql://localhost/db".to_string()
            ))
        );
    }

    #[test]
    fn test_spring_application_name_with_comments_and_exclamation() {
        let props = "# Hash comment\n\
                     ! Exclamation comment\n\
                     server.port=8080\n\
                     # Another comment\n\
                     spring.application.name=test-app\n\
                     ! Final comment"
            .as_bytes();
        let result = parse_properties(props, "spring.application.name");
        assert_eq!(result, Some("test-app".to_string()));
    }
}
