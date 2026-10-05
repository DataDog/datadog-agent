rule MAC_Scam_Launcher_JXA_GrokTradingAI
{
    meta:
        description = "JXA compile se faisant passer pour un installeur xGrok - redirection vers arnaque trading apps.grok.com/trading-ai"
        author = "Marjoriefort"
        yarahub_reference_md5 = "4d1441872f31bd14af12aec613d133e6"
        date = "2026-09-19"

        yarahub_uuid = "4c20e755-42fe-4532-b771-29b437b0cd27"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $m = "JsOsaDAS"
        $a = "xGrok AI Installation Configuration" wide ascii
        $b = "apps.grok.com/trading-ai" wide ascii

    condition:
        $m and ($a or $b)
}
