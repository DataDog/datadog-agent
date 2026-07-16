use dd_sds::{RegexRuleConfig, RootRuleConfig};
use serde::Deserialize;

/// A scanning rule, shared by every sub task. `id` (used to map matches back to
/// the rule) and `name` are ours; everything else is the flattened dd-sds
/// `RootRuleConfig<RegexRuleConfig>`, so the full rule schema — pattern,
/// proximity keywords, suppressions, precedence and secondary validators (Luhn,
/// JWT, ...) — comes straight from dd-sds with no duplication.
#[derive(Debug, Deserialize)]
pub struct ScanningRule {
    pub id: String,
    #[serde(default)]
    #[allow(dead_code)]
    pub name: String,
    #[serde(flatten)]
    pub config: RootRuleConfig<RegexRuleConfig>,
}
