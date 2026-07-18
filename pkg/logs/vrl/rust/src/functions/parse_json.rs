// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Adapted from the `vrl` crate (vectordotdev/vrl, crates.io "vrl" v0.33.1,
// src/stdlib/parse_json.rs), which is MPL-2.0 licensed. See
// ../../LICENSE-vrl for the full text. The `max_depth` parameter present
// upstream is intentionally dropped here to keep this v1 minimal; it can be
// ported back verbatim if a concrete need arises.

use vrl::prelude::*;

use crate::functions::json_utils::{StripBomFromUTF8, json_type_def};

static DEFAULT_LOSSY: Value = Value::Boolean(true);

const PARAMETERS: &[Parameter] = &[
    Parameter::required(
        "value",
        kind::BYTES,
        "The string representation of the JSON to parse.",
    ),
    Parameter::optional(
        "lossy",
        kind::BOOLEAN,
        "Whether to parse the JSON in a lossy manner. Replaces invalid UTF-8 characters
with the Unicode character \u{fffd} (U+FFFD) if set to true, otherwise returns an error
if there are any invalid UTF-8 characters present.",
    )
    .default(&DEFAULT_LOSSY),
];

fn parse_json(value: Value, lossy: Value) -> Resolved {
    let lossy = lossy.try_boolean()?;
    Ok(if lossy {
        serde_json::from_str(value.try_bytes_utf8_lossy()?.strip_bom())
    } else {
        serde_json::from_slice(value.try_bytes()?.strip_bom())
    }
    .map_err(|e| format!("unable to parse json: {e}"))?)
}

#[derive(Clone, Copy, Debug)]
pub struct ParseJson;

impl Function for ParseJson {
    fn identifier(&self) -> &'static str {
        "parse_json"
    }

    fn summary(&self) -> &'static str {
        "parse a string to a JSON type"
    }

    fn usage(&self) -> &'static str {
        indoc! {"
            Parses the provided `value` as JSON.

            Only JSON types are returned.
        "}
    }

    fn category(&self) -> &'static str {
        "parse"
    }

    fn internal_failure_reasons(&self) -> &'static [&'static str] {
        &["`value` is not a valid JSON-formatted payload."]
    }

    fn return_kind(&self) -> u16 {
        kind::BOOLEAN
            | kind::INTEGER
            | kind::FLOAT
            | kind::BYTES
            | kind::OBJECT
            | kind::ARRAY
            | kind::NULL
    }

    fn parameters(&self) -> &'static [Parameter] {
        PARAMETERS
    }

    fn examples(&self) -> &'static [Example] {
        &[]
    }

    fn compile(
        &self,
        _state: &state::TypeState,
        _ctx: &mut FunctionCompileContext,
        arguments: ArgumentList,
    ) -> Compiled {
        let value = arguments.required("value");
        let lossy = arguments.optional("lossy");

        Ok(ParseJsonFn { value, lossy }.as_expr())
    }
}

#[derive(Debug, Clone)]
struct ParseJsonFn {
    value: Box<dyn Expression>,
    lossy: Option<Box<dyn Expression>>,
}

impl FunctionExpression for ParseJsonFn {
    fn resolve(&self, ctx: &mut Context) -> Resolved {
        let value = self.value.resolve(ctx)?;
        let lossy = self
            .lossy
            .map_resolve_with_default(ctx, || DEFAULT_LOSSY.clone())?;
        parse_json(value, lossy)
    }

    fn type_def(&self, _: &state::TypeState) -> TypeDef {
        json_type_def()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use vrl::compiler::runtime::Runtime;
    use vrl::compiler::{TargetValue, TimeZone};
    use vrl::value::Secrets;

    fn resolve(source: &str) -> Result<Value, String> {
        let functions: Vec<Box<dyn Function>> = vec![Box::new(ParseJson)];
        let compiled = vrl::compiler::compile(source, &functions).map_err(|e| format!("{e:?}"))?;

        let mut target = TargetValue {
            value: Value::Object(ObjectMap::new()),
            metadata: Value::Object(ObjectMap::new()),
            secrets: Secrets::default(),
        };
        let mut runtime = Runtime::default();
        runtime
            .resolve(&mut target, &compiled.program, &TimeZone::default())
            .map_err(|e| format!("{e:?}"))
    }

    #[test]
    fn parses_an_object() {
        let result = resolve(r#"parse_json!("{\"field\": \"value\"}")"#).unwrap();
        assert_eq!(
            result
                .as_object()
                .unwrap()
                .get("field")
                .unwrap()
                .as_bytes()
                .unwrap(),
            "value"
        );
    }

    #[test]
    fn errors_on_invalid_json() {
        assert!(resolve(r#"parse_json!("{ invalid")"#).is_err());
    }
}
