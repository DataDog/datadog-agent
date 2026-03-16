use std::collections::HashMap;

/// A simple string interner that deduplicates strings and returns u32 codes.
///
/// On flush the interner yields the dictionary (unique values) and the codes
/// vector, ready to be turned into a Vortex `DictArray`.
pub struct StringInterner {
    map: HashMap<String, u32>,
    values: Vec<String>,
    codes: Vec<u32>,
}

impl StringInterner {
    pub fn new() -> Self {
        Self {
            map: HashMap::new(),
            values: Vec::new(),
            codes: Vec::new(),
        }
    }

    pub fn with_capacity(cap: usize) -> Self {
        Self {
            map: HashMap::with_capacity(cap / 4), // rough estimate of distinct values
            values: Vec::new(),
            codes: Vec::with_capacity(cap),
        }
    }

    /// Intern a string reference, returning the code assigned to it.
    #[inline]
    pub fn intern(&mut self, s: &str) -> u32 {
        let code = if let Some(&c) = self.map.get(s) {
            c
        } else {
            let c = self.values.len() as u32;
            self.values.push(s.to_string());
            self.map.insert(s.to_string(), c);
            c
        };
        self.codes.push(code);
        code
    }

    /// Intern an owned string, avoiding an extra allocation when the value is new.
    #[inline]
    pub fn intern_owned(&mut self, s: String) -> u32 {
        let code = if let Some(&c) = self.map.get(&s) {
            c
        } else {
            let c = self.values.len() as u32;
            self.map.insert(s.clone(), c);
            self.values.push(s);
            c
        };
        self.codes.push(code);
        code
    }

    /// Number of rows interned so far.
    #[inline]
    pub fn len(&self) -> usize {
        self.codes.len()
    }

    /// Take the dictionary values and codes, resetting the interner.
    pub fn take(&mut self) -> (Vec<String>, Vec<u32>) {
        self.map.clear();
        (
            std::mem::take(&mut self.values),
            std::mem::take(&mut self.codes),
        )
    }

    /// Clear all state.
    pub fn clear(&mut self) {
        self.map.clear();
        self.values.clear();
        self.codes.clear();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_basic_interning() {
        let mut interner = StringInterner::new();
        assert_eq!(interner.intern("hello"), 0);
        assert_eq!(interner.intern("world"), 1);
        assert_eq!(interner.intern("hello"), 0);
        assert_eq!(interner.len(), 3);

        let (values, codes) = interner.take();
        assert_eq!(values, vec!["hello", "world"]);
        assert_eq!(codes, vec![0, 1, 0]);
    }

    #[test]
    fn test_intern_owned() {
        let mut interner = StringInterner::new();
        interner.intern_owned("a".to_string());
        interner.intern_owned("b".to_string());
        interner.intern_owned("a".to_string());

        let (values, codes) = interner.take();
        assert_eq!(values, vec!["a", "b"]);
        assert_eq!(codes, vec![0, 1, 0]);
    }

    #[test]
    fn test_take_resets() {
        let mut interner = StringInterner::new();
        interner.intern("x");
        let _ = interner.take();
        assert_eq!(interner.len(), 0);
        assert_eq!(interner.intern("x"), 0); // re-assigns code 0
    }
}
