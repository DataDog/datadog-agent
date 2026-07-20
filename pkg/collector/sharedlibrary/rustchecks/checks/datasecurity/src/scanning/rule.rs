use dd_sds::{RegexRuleConfig, RootRuleConfig};
use serde::Deserialize;

#[derive(Debug, Deserialize)]
pub struct ScanningRule {
    pub id: String,
    #[serde(default)]
    #[allow(dead_code)]
    pub name: String,
    #[serde(flatten)]
    pub config: RootRuleConfig<RegexRuleConfig>,
}
