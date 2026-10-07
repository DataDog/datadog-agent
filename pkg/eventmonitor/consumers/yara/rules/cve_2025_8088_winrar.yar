rule cve_2025_8088_winrar
{
    meta:
        description = "Detects RAR archives with CVE-2025-8088"
        author = "t3ft3lb"
        date = "2026-01-02"
        reference = "https://nvd.nist.gov/vuln/detail/CVE-2025-8088"
        yarahub_uuid = "ebd6e272-b827-4e6f-b847-5da699702f23"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "2321fcff65ad6a40cc10dab947390cd2"

    strings:
        $s0 = "\x03STM" ascii
        $s1 = "\\\\..\\\\" ascii fullword
        $s2 = "\\..\\..\\" ascii fullword

    condition:
        uint32(0) == 0x21726152 and
        $s0 and (#s1 > 2 or #s2 > 2)
}