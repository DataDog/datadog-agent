rule u42_crime_win_heartcrypt {
    meta:
        author = "Unit 42 Threat Intelligence"
        date = "2024-11-30"
        description = "HeartCrypt PaaS hunting rule."
        yarahub_uuid = "79c4c50f-c927-445f-8620-04c094e01c65"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "4b03aa6eb9a13c1d957fb75f13696579"
    strings:
        $a = {E8 08 00 00 00 00 ?? ?? ?? ?? ?? ?? ?? 83 C4 04 81}
        $b = {
            B8 4D 00 00 00
            66 89 85 ?? ?? ?? ??
            B9 42 00 00 00
            66 89 8D ?? ?? ?? ??
            BA 53 00 00 00
            66 89 95 ?? ?? ?? ??
            B8 65 00 00 00
            66 89 85 ?? ?? ?? ??
            B9 72 00 00 00
            66 89 8D ?? ?? ?? ??
            BA 76 00 00 00
            66 89 95 ?? ?? ?? ??
            B8 69 00 00 00
            66 89 85 ?? ?? ?? ??
            B9 63 00 00 00
            66 89 8D ?? ?? ?? ??
            BA 65 00 00 00
            66 89 95 ?? ?? ?? ??
            B8 2E 00 00 00
            66 89 85 ?? ?? ?? ??
            B9 65 00 00 00
            66 89 8D ?? ?? ?? ??
            BA 78 00 00 00
            66 89 95 ?? ?? ?? ??
            B8 65 00 00 00
            66 89 85 ?? ?? ?? ??
            33 C9
            66 89 8D ?? ?? ?? ??
            }
    condition:
        $a or $b
}