rule MULTI_Sample_CoinMiner_1ad8694c_Extrait
{
    meta:
        author = "Marjoriefort"
        description = "Detects CoinMiner (inconnu, etat extrait)"
        date = "2026-09-21"
        reference_sha256 = "1ad8694c8b94a923be6056a58c896f872c5bbfa3b8c26f3f31d601615f3fdc90"
        yarahub_uuid = "96e4fde9-388a-419d-a97b-965392fcd2e5"
        famille = "CoinMiner"
        classe = "inconnu"
        etat = "extrait"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.8.9"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "54e9f45448cfc5c6f2506cf89d771fd5"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "\"worker-id\": null" ascii
        $s1 = "\"access-token\": null," ascii
        $s2 = "\"init-avx2\": -1," ascii
        $s3 = "\"1gb-pages\": false," ascii
        $s4 = "\"cache_qos\": false," ascii
        $s5 = "\"scratchpad_prefetch_mode\": 1" ascii
        $s6 = "\"huge-pages\": true," ascii
        $s7 = "\"huge-pages-jit\": false," ascii
        $s8 = "\"hw-aes\": null," ascii
        $s9 = "\"memory-pool\": false," ascii
        $s10 = "\"max-threads-hint\": 100," ascii
        $s11 = "\"argon2-impl\": null," ascii
        $s12 = "\"cn/0\": false," ascii
        $s13 = "\"cn-lite/0\": false" ascii
        $s14 = "\"donate-level\": 1," ascii
        $s15 = "\"donate-over-proxy\": 1," ascii
        $s16 = "\"url\": \"donate.v2.xmrig.com:3333\"," ascii
        $s17 = "\"user\": \"YOUR_WALLET_ADDRESS\"," ascii
        $s18 = "\"rig-id\": null," ascii
        $s19 = "\"tls-fingerprint\": null," ascii
    condition:
        true and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
