use dd_sds::{RegexRuleConfig, RootRuleConfig};
use serde::Deserialize;

#[derive(Debug, Deserialize, PartialEq)]
pub struct ScanningRule {
    pub id: String,
    #[serde(flatten)]
    pub config: RootRuleConfig<RegexRuleConfig>,
}
