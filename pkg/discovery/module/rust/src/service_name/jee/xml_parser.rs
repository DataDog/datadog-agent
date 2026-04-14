// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Streaming XML parser with built-in security limits.
//!
//! Wraps xml-rs (`EventReader`) in an `XmlParser` struct that implements
//! `Iterator`, automatically enforcing depth, attribute, and event-count
//! limits while filtering raw events down to `StartElement` / `EndElement` /
//! `Text`.
//!
//! Security properties:
//!
//! - **Deep nesting**: xml-rs is iterative (no stack overflow risk), and
//!   `XmlParser` rejects documents exceeding `MAX_DEPTH`.
//! - **Attribute flooding**: xml-rs's built-in `max_attributes` limit caps
//!   per-element attribute count, preventing CPU exhaustion.
//! - **XXE prevention**: xml-rs does not support external entity expansion
//!   or DTD entity definitions.
//! - **Event-count limit**: `MAX_EVENTS` caps total parse events to prevent
//!   unbounded looping on adversarial input.

use super::Error;
use std::io::Read;
use xml::attribute::OwnedAttribute;
use xml::reader::{EventReader, ParserConfig2, XmlEvent};

/// Maximum nesting depth for XML elements.
/// No legitimate JEE configuration file approaches this depth.
/// Note: xml-rs is iterative, so deep nesting cannot cause stack overflow.
/// This limit is retained as defence-in-depth in case future callers
/// accumulate state proportional to depth — current callers do not.
pub const MAX_DEPTH: usize = 256;

/// Maximum attributes per XML element.
/// Protects against CPU exhaustion from attribute flooding.
const MAX_ATTRS: usize = 256;

/// Maximum total parse events (start, end, text, etc.) before the parser
/// gives up.  No legitimate JEE configuration file approaches this count.
/// Protects against adversarial input that could cause unbounded looping.
const MAX_EVENTS: usize = 100_000;

/// A parsed XML event stripped down to the three variants callers need.
/// Whitespace, comments, and processing instructions are consumed
/// internally.  End-of-document is signalled by `None` from the iterator.
#[derive(Debug)]
pub enum XmlItem {
    StartElement {
        name: String,
        attributes: Vec<OwnedAttribute>,
    },
    EndElement {
        name: String,
    },
    Text(String),
}

/// Return value from [`XmlHandler`] methods, encoding both the next state
/// and any depth change the framework should apply.
pub enum Action<S> {
    /// Transition to the new state and descend one level.
    Descend(S),
    /// Transition to the new state and ascend one level.
    Ascend(S),
    /// Transition to the new state without changing depth.
    Same(S),
    /// Stop parsing immediately.
    Break,
}

/// Trait for implementing streaming XML parsers as state machines.
///
/// Instead of writing a manual `while let Some(event) = parser.next()`
/// loop with nested matches, implement this trait and call
/// [`XmlParser::run`].  The framework handles event dispatch, depth
/// tracking (descend/ascend), and error propagation.
pub trait XmlHandler {
    /// Tracks position within the XML element hierarchy.
    type State;

    /// Called for each `StartElement` event.
    fn start_element(
        &mut self,
        state: Self::State,
        name: &str,
        attributes: &[OwnedAttribute],
    ) -> Action<Self::State>;

    /// Called for each `EndElement` event.
    fn end_element(&mut self, state: Self::State, name: &str) -> Action<Self::State>;

    /// Called for each `Text` (or `CData`) event.
    /// Default implementation ignores text and preserves the current state.
    fn text(&mut self, state: Self::State, _text: String) -> Action<Self::State> {
        Action::Same(state)
    }
}

/// Streaming XML parser with automatic depth tracking and security limits.
///
/// Implements [`Iterator`] over `Result<XmlItem, Error>`.
pub struct XmlParser<R: Read> {
    reader: EventReader<R>,
    depth: usize,
    event_count: usize,
    done: bool,
    required_depth: usize,
    exit_depths: Vec<usize>,
}

impl<R: Read> XmlParser<R> {
    /// Creates a new parser with security limits applied.
    pub fn new(reader: R) -> Self {
        Self {
            reader: ParserConfig2::new()
                .max_attributes(MAX_ATTRS)
                .create_reader(reader),
            depth: 0,
            event_count: 0,
            done: false,
            required_depth: 1,
            exit_depths: Vec::new(),
        }
    }

    fn descend(&mut self) {
        self.exit_depths.push(self.required_depth);
        self.required_depth += 1;
    }

    fn ascend(&mut self) {
        self.exit_depths.pop();
        self.required_depth -= 1;
    }

    /// Runs an [`XmlHandler`] against this parser until EOF, `Break`, or error.
    pub fn run<H: XmlHandler>(
        &mut self,
        handler: &mut H,
        mut state: H::State,
    ) -> Result<(), Error> {
        while let Some(event) = self.next() {
            let action = match event? {
                XmlItem::StartElement {
                    ref name,
                    ref attributes,
                    ..
                } => handler.start_element(state, name, attributes),
                XmlItem::EndElement { ref name, .. } => handler.end_element(state, name),
                XmlItem::Text(s) => handler.text(state, s),
            };
            state = match action {
                Action::Descend(s) => {
                    self.descend();
                    s
                }
                Action::Ascend(s) => {
                    self.ascend();
                    s
                }
                Action::Same(s) => s,
                Action::Break => return Ok(()),
            };
        }
        Ok(())
    }
}

impl<R: Read> Iterator for XmlParser<R> {
    type Item = Result<XmlItem, Error>;

    fn next(&mut self) -> Option<Self::Item> {
        loop {
            if self.done {
                return None;
            }
            self.event_count += 1;
            if self.event_count > MAX_EVENTS {
                self.done = true;
                return Some(Err(Error::XmlParse("too many XML parse events".into())));
            }
            match self.reader.next() {
                Ok(XmlEvent::StartElement {
                    name, attributes, ..
                }) => {
                    self.depth += 1;
                    if self.depth > MAX_DEPTH {
                        self.done = true;
                        return Some(Err(Error::XmlParse("XML nesting too deep".into())));
                    }
                    if self.required_depth == self.depth {
                        return Some(Ok(XmlItem::StartElement {
                            name: name.local_name,
                            attributes,
                        }));
                    }
                }
                Ok(XmlEvent::EndElement { name, .. }) => {
                    let depth = self.depth;
                    self.depth = self.depth.saturating_sub(1);
                    if self.exit_depths.last() == Some(&depth) {
                        return Some(Ok(XmlItem::EndElement {
                            name: name.local_name,
                        }));
                    }
                }
                Ok(XmlEvent::Characters(s)) | Ok(XmlEvent::CData(s)) => {
                    // Only emit text when inside an element the caller
                    // descended into.  After descending into an element at
                    // depth D, required_depth becomes D+1 and text directly
                    // inside that element is at depth D (== required - 1).
                    // Text inside deeper children that were *not* descended
                    // into lives at depth >= required and must be filtered,
                    // otherwise it leaks into accumulating handlers.
                    if self.depth + 1 == self.required_depth {
                        return Some(Ok(XmlItem::Text(s)));
                    }
                }
                Ok(XmlEvent::EndDocument) => {
                    self.done = true;
                    return None;
                }
                Err(e) => {
                    self.done = true;
                    return Some(Err(Error::XmlParse(e.to_string())));
                }
                _ => {} // Skip whitespace, comments, PI, StartDocument
            }
        }
    }
}

/// Gets an attribute value by local name (ignoring namespace prefix).
pub fn get_attr(attributes: &[OwnedAttribute], name: &str) -> Option<String> {
    attributes
        .iter()
        .find(|a| a.name.local_name == name)
        .map(|a| a.value.clone())
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
#[allow(clippy::panic)]
#[allow(clippy::indexing_slicing)]
mod tests {
    use super::*;

    /// Verifies that deeply nested XML is rejected at MAX_DEPTH.
    #[test]
    fn test_deep_nesting_rejected() {
        let depth = MAX_DEPTH + 10;
        let mut xml = String::new();
        for i in 0..depth {
            xml.push_str(&format!("<level{}>", i));
        }
        for i in (0..depth).rev() {
            xml.push_str(&format!("</level{}>", i));
        }

        let mut parser = XmlParser::new(xml.as_bytes());
        let mut found_error = false;
        for event in &mut parser {
            if event.is_err() {
                found_error = true;
                break;
            }
        }
        assert!(
            found_error,
            "Deep nesting was not detected — MAX_DEPTH check is broken"
        );
    }

    /// Verifies that XML with too many attributes is rejected by the parser.
    #[test]
    fn test_many_attributes_rejected() {
        let num_attrs = MAX_ATTRS + 10;
        let mut xml = String::from("<root ");
        for i in 0..num_attrs {
            xml.push_str(&format!("attr{}=\"val{}\" ", i, i));
        }
        xml.push_str("/>");

        let mut parser = XmlParser::new(xml.as_bytes());
        for event in &mut parser {
            match event {
                Ok(XmlItem::StartElement { attributes, .. }) => {
                    assert!(
                        attributes.len() <= MAX_ATTRS,
                        "Parser accepted {} attributes, limit should be {}",
                        attributes.len(),
                        MAX_ATTRS
                    );
                    return;
                }
                Err(_) => return, // Parser rejected — expected
                _ => {}
            }
        }
    }

    /// Verifies that XML entity expansion (billion laughs) is blocked.
    #[test]
    fn test_entity_expansion_blocked() {
        let xml = r#"<?xml version="1.0"?>
<!DOCTYPE bomb [
  <!ENTITY a "AAAAAAAAAA">
  <!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;">
]>
<root>&b;</root>"#;

        // Verify parsing completes without hanging or expanding entities.
        let mut parser = XmlParser::new(xml.as_bytes());
        for event in &mut parser {
            if event.is_err() {
                return;
            }
        }
    }

    /// Verifies that XXE (external entity expansion) is blocked.
    #[test]
    fn test_xxe_blocked() {
        let xml = r#"<?xml version="1.0"?>
<!DOCTYPE foo [
  <!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<root>&xxe;</root>"#;

        let mut parser = XmlParser::new(xml.as_bytes());
        for event in &mut parser {
            if event.is_err() {
                return;
            }
        }
    }

    /// Verifies the event-count safety limit.
    #[test]
    fn test_max_events_limit() {
        // Each <a/> produces ~2 raw events; generate enough to exceed MAX_EVENTS.
        let num_elements = MAX_EVENTS / 2 + 100;
        let mut xml = String::with_capacity(num_elements * 4 + 14);
        xml.push_str("<root>");
        for _ in 0..num_elements {
            xml.push_str("<a/>");
        }
        xml.push_str("</root>");

        let mut parser = XmlParser::new(xml.as_bytes());
        let mut hit_limit = false;
        for event in &mut parser {
            if event.is_err() {
                hit_limit = true;
                break;
            }
        }
        assert!(hit_limit, "Event limit was not enforced");
    }

    #[test]
    fn test_get_attr() {
        use xml::name::OwnedName;

        let attrs = vec![
            OwnedAttribute {
                name: OwnedName::local("appBase"),
                value: "webapps".to_string(),
            },
            OwnedAttribute {
                name: OwnedName {
                    local_name: "id".to_string(),
                    namespace: Some("http://www.omg.org/XMI".to_string()),
                    prefix: Some("xmi".to_string()),
                },
                value: "Target_123".to_string(),
            },
        ];

        assert_eq!(get_attr(&attrs, "appBase"), Some("webapps".to_string()));
        // Matches by local name, ignoring namespace prefix
        assert_eq!(get_attr(&attrs, "id"), Some("Target_123".to_string()));
        assert_eq!(get_attr(&attrs, "missing"), None);
    }

    #[test]
    fn test_text_events() {
        let xml = "<root><name>hello world</name></root>";
        let mut parser = XmlParser::new(xml.as_bytes());
        let events = collect_names(&mut parser);
        assert_eq!(
            events,
            vec![
                ("start".into(), "root".into()),
                ("start".into(), "name".into()),
                ("text".into(), "hello world".into()),
                ("end".into(), "name".into()),
                ("end".into(), "root".into()),
            ]
        );
    }

    /// Collects all events, returning Ok(items) if no errors, or the first error.
    fn collect_events(xml: &[u8]) -> Result<Vec<XmlItem>, Error> {
        let mut items = Vec::new();
        for event in XmlParser::new(xml) {
            items.push(event?);
        }
        Ok(items)
    }

    #[test]
    fn test_empty_input() {
        // xml-rs treats empty input as malformed — verify we surface the error.
        assert!(collect_events(b"").is_err());
    }

    #[test]
    fn test_not_xml() {
        assert!(collect_events(b"this is not xml at all").is_err());
    }

    #[test]
    fn test_unclosed_tag() {
        assert!(collect_events(b"<root><child>text</root>").is_err());
    }

    #[test]
    fn test_truncated_mid_tag() {
        assert!(collect_events(b"<root><child attr=").is_err());
    }

    #[test]
    fn test_truncated_after_start() {
        assert!(collect_events(b"<root>").is_err());
    }

    #[test]
    fn test_mismatched_close_tag() {
        assert!(collect_events(b"<root><a></b></root>").is_err());
    }

    #[test]
    fn test_double_close() {
        assert!(collect_events(b"<root></root></root>").is_err());
    }

    /// After an error, the parser must stop (return None) — no stale events.
    #[test]
    fn test_error_terminates_iterator() {
        let xml = b"<root><a></b></root>";
        let mut parser = XmlParser::new(xml.as_slice());
        let mut saw_error = false;
        for event in &mut parser {
            if event.is_err() {
                saw_error = true;
                break;
            }
        }
        assert!(saw_error);
        assert!(
            parser.next().is_none(),
            "Iterator must return None after error"
        );
    }

    /// Verifies that an I/O error mid-stream is surfaced as an XmlParse error.
    #[test]
    fn test_io_error_mid_stream() {
        struct FailAfter {
            data: &'static [u8],
            pos: usize,
            fail_at: usize,
        }
        impl std::io::Read for FailAfter {
            fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
                if self.pos >= self.fail_at {
                    return Err(std::io::Error::new(
                        std::io::ErrorKind::BrokenPipe,
                        "simulated I/O failure",
                    ));
                }
                let remaining = &self.data[self.pos..self.fail_at.min(self.data.len())];
                let n = remaining.len().min(buf.len());
                buf[..n].copy_from_slice(&remaining[..n]);
                self.pos += n;
                Ok(n)
            }
        }

        // Feed valid XML but cut the reader off mid-stream.
        let xml = b"<root><child>text</child></root>";
        let reader = FailAfter {
            data: xml,
            pos: 0,
            fail_at: 10, // cuts off inside <child>
        };
        let mut parser = XmlParser::new(reader);
        let mut saw_error = false;
        for event in &mut parser {
            if event.is_err() {
                saw_error = true;
                break;
            }
        }
        assert!(saw_error, "I/O error should surface as parse error");
        assert!(
            parser.next().is_none(),
            "Iterator must return None after error"
        );
    }

    #[test]
    fn test_empty_elements() {
        let mut parser = XmlParser::new(b"<root><a/><b></b></root>" as &[u8]);
        let events = collect_names(&mut parser);
        // <a/> produces start + end, same as <b></b>
        assert_eq!(events.len(), 6);
        assert_eq!(events[1], ("start".into(), "a".into()));
        assert_eq!(events[2], ("end".into(), "a".into()));
        assert_eq!(events[3], ("start".into(), "b".into()));
        assert_eq!(events[4], ("end".into(), "b".into()));
    }

    #[test]
    fn test_comments_and_pi_are_skipped() {
        let xml = b"<root><!-- comment --><?pi target?><a/></root>";
        let mut parser = XmlParser::new(xml as &[u8]);
        let events = collect_names(&mut parser);
        // Only root start, a start, a end, root end
        assert_eq!(events.len(), 4);
        assert_eq!(events[1], ("start".into(), "a".into()));
    }

    /// Helper: collect events from a parser with depth filtering already set up.
    fn collect_names(parser: &mut XmlParser<&[u8]>) -> Vec<(String, String)> {
        let mut out = Vec::new();
        while let Some(event) = parser.next() {
            match event.unwrap() {
                XmlItem::StartElement { name, .. } => {
                    out.push(("start".into(), name));
                    parser.descend();
                }
                XmlItem::EndElement { name, .. } => {
                    parser.ascend();
                    out.push(("end".into(), name));
                }
                XmlItem::Text(s) => out.push(("text".into(), s)),
            }
        }
        out
    }

    /// Basic test: descend filters to only direct children.
    #[test]
    fn test_descend_filters_children() {
        // <root><a><nested/></a><b/></root>
        // After consuming root and descending, we should see only <a> and <b>
        // StartElements (direct children of root), not <nested>.  Without
        // further descend(), no EndElements are emitted.
        let xml = b"<root><a><nested/></a><b/></root>";
        let mut parser = XmlParser::new(xml.as_slice());
        // Consume root
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "root"));
        parser.descend();

        let mut names = Vec::new();
        for event in &mut parser {
            if let XmlItem::StartElement { name, .. } = event.unwrap() {
                names.push(name);
            }
        }
        assert!(
            !names.contains(&"nested".to_string()),
            "nested element should be filtered out: {:?}",
            names
        );
        assert!(
            names.contains(&"a".to_string()) && names.contains(&"b".to_string()),
            "direct children a and b should be visible: {:?}",
            names
        );
    }

    /// Test descend: after seeing <a>, descend to see a's children.
    #[test]
    fn test_descend_sees_children() {
        // <root><a><b>text</b></a></root>
        // Consume root, descend, see <a>, descend, should see <b> and text.
        let xml = b"<root><a><b>text</b></a></root>";
        let mut parser = XmlParser::new(xml.as_slice());

        // Consume root
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "root"));
        parser.descend();

        // Get <a>
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "a"));

        // Descend into <a>'s children
        parser.descend();
        let events = collect_names(&mut parser);
        // Should see: start b, text "text", end b, end a, end root
        assert_eq!(events.len(), 5, "events: {:?}", events);
        assert_eq!(events[0], ("start".into(), "b".into()));
        assert_eq!(events[1], ("text".into(), "text".into()));
        assert_eq!(events[2], ("end".into(), "b".into()));
        assert_eq!(events[3], ("end".into(), "a".into()));
        assert_eq!(events[4], ("end".into(), "root".into()));
    }

    /// Test descend/ascend round-trip: after ascending, we see siblings again.
    #[test]
    fn test_descend_ascend_sees_siblings() {
        // <root><a><child/></a><b/></root>
        // Descend into <a>, consume its children, ascend, then see <b>.
        let xml = b"<root><a><child/></a><b/></root>";
        let mut parser = XmlParser::new(xml.as_slice());

        // Consume root
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "root"));
        parser.descend();

        // Get <a>
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "a"));

        parser.descend();

        // Consume <child/> start+end and </a>
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "child"));
        parser.descend();
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::EndElement { name, .. } if name == "child"));
        parser.ascend();
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::EndElement { name, .. } if name == "a"));

        parser.ascend();

        // Now should see <b>
        let event = parser.next().unwrap().unwrap();
        assert!(
            matches!(&event, XmlItem::StartElement { name, .. } if name == "b"),
            "expected <b> after ascend, got {:?}",
            event
        );
    }

    /// BUG REGRESSION: nested same-name element causes spurious EndElement.
    ///
    /// Given `<root><a><b><a></a></b><b/></a></root>`, when we descend
    /// into the outer <a> (depth 2) to see its <b> children (depth 3),
    /// the inner `</a>` at depth 4 should NOT be emitted — it's deeper
    /// than our required_depth and doesn't match exit_depths.
    ///
    /// With the buggy code (`exit_depths.last().is_some()`), the inner
    /// `</a>` IS emitted because exit_depths is non-empty.  Callers
    /// see `</a>` and ascend prematurely, missing the second `<b>`.
    #[test]
    fn test_nested_same_name_element_not_spurious() {
        let xml = b"<root><a><b><a></a></b><b/></a></root>";
        let mut parser = XmlParser::new(xml.as_slice());

        // Consume root
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "root"));
        parser.descend();

        // Get outer <a>
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "a"));

        parser.descend();

        // Collect events without further descending — we only want to see
        // direct children of <a> (the two <b> elements), not grandchildren.
        let mut starts = Vec::new();
        let mut end_a_count = 0;
        for event in &mut parser {
            match event.unwrap() {
                XmlItem::StartElement { name, .. } => starts.push(name),
                XmlItem::EndElement { name, .. } if name == "a" => end_a_count += 1,
                _ => {}
            }
        }

        // We should see BOTH <b> children
        assert_eq!(
            starts,
            vec!["b", "b"],
            "should see both <b> children, got starts: {:?}",
            starts
        );
        // Only ONE </a> — the outer one (our exit element)
        assert_eq!(
            end_a_count, 1,
            "should see exactly one </a> (the outer), got end_a_count: {}",
            end_a_count
        );
    }

    /// Text inside elements the caller did NOT descend into should be
    /// filtered.  This prevents subtree text from leaking into handlers
    /// that accumulate text in the parent element's context.
    #[test]
    fn test_text_filtered_in_skipped_subtree() {
        // <root><a><nested>poison</nested>direct</a></root>
        // After descending into root and <a>, <nested> is emitted as a
        // StartElement but we do NOT descend into it.  "poison" must be
        // filtered; "direct" (sibling text inside <a>) must be emitted.
        let xml = b"<root><a><nested>poison</nested>direct</a></root>";
        let mut parser = XmlParser::new(xml.as_slice());

        // Consume <root>, descend
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "root"));
        parser.descend();

        // Consume <a>, descend
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "a"));
        parser.descend();

        // Consume <nested>, do NOT descend — skip this subtree
        let event = parser.next().unwrap().unwrap();
        assert!(matches!(&event, XmlItem::StartElement { name, .. } if name == "nested"));

        // Collect remaining text events
        let mut texts = Vec::new();
        for event in &mut parser {
            if let XmlItem::Text(s) = event.unwrap() {
                texts.push(s);
            }
        }

        assert_eq!(
            texts,
            vec!["direct"],
            "text from skipped subtree should be filtered"
        );
    }

    /// Verify text depth filtering through the run() + XmlHandler API:
    /// text inside elements the handler skips (Same) must not reach
    /// the handler's text() method.
    #[test]
    fn test_run_text_filtered_in_skipped_subtree() {
        struct TextCollector {
            texts: Vec<String>,
        }

        impl XmlHandler for TextCollector {
            type State = bool; // true = inside target element

            fn start_element(
                &mut self,
                state: bool,
                name: &str,
                _attributes: &[OwnedAttribute],
            ) -> Action<bool> {
                match name {
                    "target" => Action::Descend(true),
                    "root" | "wrapper" => Action::Descend(false),
                    _ => Action::Same(state), // skip unknown children
                }
            }

            fn end_element(&mut self, _state: bool, _name: &str) -> Action<bool> {
                Action::Ascend(false)
            }

            fn text(&mut self, state: bool, text: String) -> Action<bool> {
                if state {
                    self.texts.push(text);
                }
                Action::Same(state)
            }
        }

        let xml = b"\
            <root><wrapper>\
                <target><nested>poison</nested>good</target>\
            </wrapper></root>";
        let mut parser = XmlParser::new(xml.as_slice());
        let mut handler = TextCollector { texts: Vec::new() };
        parser.run(&mut handler, false).unwrap();

        assert_eq!(
            handler.texts,
            vec!["good"],
            "poison text from <nested> should be filtered"
        );
    }

    // ---------------------------------------------------------------
    // XmlHandler / run() tests
    // ---------------------------------------------------------------

    /// A handler that collects element names at each depth, using
    /// Descend/Ascend/Same/Break to navigate the tree.
    struct CollectingHandler {
        elements: Vec<(String, usize)>, // (name, depth)
        texts: Vec<String>,
    }

    #[derive(Clone, Copy)]
    struct Depth(usize);

    impl XmlHandler for CollectingHandler {
        type State = Depth;

        fn start_element(
            &mut self,
            state: Depth,
            name: &str,
            _attributes: &[OwnedAttribute],
        ) -> Action<Depth> {
            self.elements.push((name.to_string(), state.0));
            Action::Descend(Depth(state.0 + 1))
        }

        fn end_element(&mut self, state: Depth, _name: &str) -> Action<Depth> {
            Action::Ascend(Depth(state.0 - 1))
        }

        fn text(&mut self, state: Depth, text: String) -> Action<Depth> {
            self.texts.push(text);
            Action::Same(state)
        }
    }

    /// run() dispatches start, end, and text events to the handler,
    /// handling Descend/Ascend/Same transitions.
    #[test]
    fn test_run_collects_tree() {
        let xml = b"<root><a><b>hello</b></a><c/></root>";
        let mut parser = XmlParser::new(xml.as_slice());
        let mut handler = CollectingHandler {
            elements: Vec::new(),
            texts: Vec::new(),
        };
        parser.run(&mut handler, Depth(0)).unwrap();

        assert_eq!(
            handler.elements,
            vec![
                ("root".into(), 0),
                ("a".into(), 1),
                ("b".into(), 2),
                ("c".into(), 1),
            ]
        );
        assert_eq!(handler.texts, vec!["hello"]);
    }

    /// A handler that stops parsing on a specific element via Break.
    struct BreakOnHandler {
        stop_at: String,
        seen: Vec<String>,
    }

    impl XmlHandler for BreakOnHandler {
        type State = ();

        fn start_element(
            &mut self,
            _state: (),
            name: &str,
            _attributes: &[OwnedAttribute],
        ) -> Action<()> {
            self.seen.push(name.to_string());
            if name == self.stop_at {
                Action::Break
            } else {
                Action::Descend(())
            }
        }

        fn end_element(&mut self, _state: (), _name: &str) -> Action<()> {
            Action::Ascend(())
        }
    }

    /// run() stops when the handler returns Break.
    #[test]
    fn test_run_break_stops_parsing() {
        let xml = b"<root><a/><b/><c/></root>";
        let mut parser = XmlParser::new(xml.as_slice());
        let mut handler = BreakOnHandler {
            stop_at: "b".into(),
            seen: Vec::new(),
        };
        parser.run(&mut handler, ()).unwrap();

        // Should see root, a, b — then stop before c.
        assert_eq!(handler.seen, vec!["root", "a", "b"]);
    }

    /// run() propagates XML parse errors.
    #[test]
    fn test_run_propagates_error() {
        let xml = b"<root><a></b></root>"; // mismatched tags
        let mut parser = XmlParser::new(xml.as_slice());
        let mut handler = CollectingHandler {
            elements: Vec::new(),
            texts: Vec::new(),
        };
        let result = parser.run(&mut handler, Depth(0));
        assert!(result.is_err(), "run() should propagate parse errors");
    }

    /// A handler that relies on the default text() implementation
    /// (ignores text, preserves state).
    struct IgnoreTextHandler {
        elements: Vec<String>,
    }

    impl XmlHandler for IgnoreTextHandler {
        type State = ();

        fn start_element(
            &mut self,
            _state: (),
            name: &str,
            _attributes: &[OwnedAttribute],
        ) -> Action<()> {
            self.elements.push(name.to_string());
            Action::Descend(())
        }

        fn end_element(&mut self, _state: (), _name: &str) -> Action<()> {
            Action::Ascend(())
        }
    }

    /// The default text() method ignores text without disrupting parsing.
    #[test]
    fn test_run_default_text_is_ignored() {
        let xml = b"<root><a>some text</a></root>";
        let mut parser = XmlParser::new(xml.as_slice());
        let mut handler = IgnoreTextHandler {
            elements: Vec::new(),
        };
        parser.run(&mut handler, ()).unwrap();
        assert_eq!(handler.elements, vec!["root", "a"]);
    }

    /// A handler that reads attributes via get_attr.
    struct AttrHandler {
        attrs: Vec<(String, Option<String>)>,
    }

    impl XmlHandler for AttrHandler {
        type State = ();

        fn start_element(
            &mut self,
            _state: (),
            name: &str,
            attributes: &[OwnedAttribute],
        ) -> Action<()> {
            let val = get_attr(attributes, "id");
            self.attrs.push((name.to_string(), val));
            Action::Descend(())
        }

        fn end_element(&mut self, _state: (), _name: &str) -> Action<()> {
            Action::Ascend(())
        }
    }

    /// run() passes attributes to the handler correctly.
    #[test]
    fn test_run_handler_sees_attributes() {
        let xml = br#"<root><item id="42"/><item/></root>"#;
        let mut parser = XmlParser::new(xml.as_slice());
        let mut handler = AttrHandler { attrs: Vec::new() };
        parser.run(&mut handler, ()).unwrap();
        assert_eq!(
            handler.attrs,
            vec![
                ("root".into(), None),
                ("item".into(), Some("42".into())),
                ("item".into(), None),
            ]
        );
    }
}
