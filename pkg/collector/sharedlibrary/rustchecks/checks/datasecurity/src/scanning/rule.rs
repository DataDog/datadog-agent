use dd_sds::{RegexRuleConfig, RootRuleConfig};
use serde::Deserialize;

#[derive(Debug, Deserialize, PartialEq)]
pub struct ScanningRule {
    pub id: String,
    /// Optional license/copyright notice attached to the rule. Defaults to empty
    /// when the config omits it.
    #[serde(default)]
    pub license: String,
    #[serde(flatten)]
    pub config: RootRuleConfig<RegexRuleConfig>,
}
