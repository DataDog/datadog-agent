// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Ported with minimal changes from the `vrl` crate (vectordotdev/vrl,
// crates.io "vrl" v0.33.1, src/stdlib/json_utils/{bom,json_type_def}.rs),
// which is MPL-2.0 licensed. See ../../LICENSE-vrl for the full text.

use vrl::prelude::{Collection, TypeDef};
use vrl::value::Kind;

/// Helper trait to strip BOM from UTF-8.
pub trait StripBomFromUTF8 {
    #[must_use]
    fn strip_bom(self) -> Self;
}

// \u{feff} and [0xef, 0xbb, 0xbf] are the same
static BOM_MARKER_BYTES: &[u8] = &[0xef, 0xbb, 0xbf];
static BOM_MARKER: char = '\u{feff}';

impl StripBomFromUTF8 for &str {
    fn strip_bom(self) -> Self {
        self.trim_start_matches(BOM_MARKER)
    }
}

impl StripBomFromUTF8 for &[u8] {
    fn strip_bom(self) -> Self {
        self.strip_prefix(BOM_MARKER_BYTES).unwrap_or(self)
    }
}

pub(crate) fn json_inner_kind() -> Kind {
    Kind::null()
        | Kind::bytes()
        | Kind::integer()
        | Kind::float()
        | Kind::boolean()
        | Kind::array(Collection::any())
        | Kind::object(Collection::any())
}

pub(crate) fn json_type_def() -> TypeDef {
    TypeDef::bytes()
        .fallible()
        .or_boolean()
        .or_integer()
        .or_float()
        .add_null()
        .or_null()
        .or_array(Collection::from_unknown(json_inner_kind()))
        .or_object(Collection::from_unknown(json_inner_kind()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn strips_bom_from_str() {
        let raw = &[BOM_MARKER_BYTES, &[0x7b, 0x7d]].concat();
        let raw: &str = std::str::from_utf8(raw).unwrap();
        assert_eq!(raw.strip_bom(), "{}");
    }

    #[test]
    fn strips_bom_from_bytes() {
        let raw = &[BOM_MARKER_BYTES, &[0x7b, 0x7d]].concat();
        assert_eq!(raw.as_slice().strip_bom(), &[0x7b, 0x7d]);
    }

    #[test]
    fn leaves_input_without_bom_unchanged() {
        assert_eq!("{}".strip_bom(), "{}");
        assert_eq!(b"{}".as_slice().strip_bom(), b"{}".as_slice());
    }
}
