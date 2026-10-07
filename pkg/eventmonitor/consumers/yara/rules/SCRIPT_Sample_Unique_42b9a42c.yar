rule SCRIPT_Sample_Unique_42b9a42c {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "42b9a42cb5793b4c8c38ce0332c0e22a784883457aae7a7c7309f8034e0bb672.js"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "edb126bb-88b0-495c-bef1-09ed0b266a13"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "6e31d5e66719d3d2148d7f4e4b18b9e0"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "	// Adjust the page count to 1 in case the initial bool-columns.clientHeight is less than the height of the screen. We only do this once.2" ascii wide
        $b = "	//To prevent 1 page chapters from not reflowing to additional pages when increasing the font size:" ascii wide
    condition:
        all of them
}
