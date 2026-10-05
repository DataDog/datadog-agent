rule Web_Malicious_JS_Web3_Contract_Includer {
    meta:
        description = "Detects malicious JavaScript injection loaders utilizing Web3 / Ethereum blockchain providers (Base network) to fetch, decrypt, and render dynamic phishing payloads or overlay frames."
        author = "Serhii Kocherhan"
        date = "2026-09-27"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "4507849d-931f-466b-bf45-f725b91c8a0b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b3a6765cba5bfc46b81adb9f291b8f37"

    strings:
        // Web3 / RPC Provider Endpoint Constants
        $s_base_rpc1 = "https://mainnet.base.org" ascii wide
        $s_base_rpc2 = "https://base.publicnode.com" ascii wide
        $s_base_rpc3 = "https://base.drpc.org" ascii wide

        // Smart Contract Function Selectors / Methods
        $s_fn1 = "scriptCount" ascii wide
        $s_fn2 = "getScript" ascii wide
        $s_fn3 = "hasDemoPage" ascii wide
        $s_fn4 = "getDemoPage" ascii wide

        // Blob/Obfuscation Handling Artifacts
        $s_blob = "nc-blob:" ascii wide
        $s_obfuscated = "DecompressionStream" ascii wide

        // Shadow DOM / Overlay Injection Container ID
        $s_shadow = "ui-aa7066ac" ascii wide

    condition:
        filesize < 50KB and (
            ($s_shadow) or
            (any of ($s_base_rpc*) and 2 of ($s_fn*)) or
            ($s_blob and $s_obfuscated)
        )
}