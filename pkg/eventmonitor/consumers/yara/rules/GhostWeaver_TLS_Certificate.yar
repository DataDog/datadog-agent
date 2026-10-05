rule GhostWeaver_TLS_Certificate
{
    meta:
        id = "2PS1wclvHTVqqWL0fNQQnE"
        fingerprint = "b9f0e5e0bd4159cf52830858f71e6ed144448e2260aea5437f59f9a64ca7c871"
        version = "1.0"
        date = "2026-03-08"
        modified = "2026-03-08"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://WWW.DERP.CA/BLOG/GHOSTWEAVER-TAG124-POWERSHELL-RAT"
        author = "derp.ca"
        description = "GhostWeaver/Pantera pinned self-signed TLS certificate (CN=GeoTrust LTD.)"
        category = "MALWARE"
        malware = "GHOSTWEAVER"
        mitre_att = "T1553.004"
        reference = "https://www.derp.ca/blog/ghostweaver-tag124-powershell-rat"
        triage_score = 10
        triage_description = "GhostWeaver/Pantera pinned TLS certificate detected."
        yarahub_uuid = "389f1cd7-386c-4e47-8b92-9bf24c2c7b75"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "78113996c94c5a1fcacba8da4ca86dde"

    strings:
        $cert_der = {
            30 82 04 ee 30 82 02 d6  // SEQUENCE headers
            a0 03 02 01 02 02 10 00  // version + serial prefix
            9e d7 a1 52 31 31 20 57  // serial number (unique)
            1a 4e 51 63 87 70 69 30  // serial + algo prefix
        }

        $cert_b64 = "MIIE7jCCAtagAwIBAgIQAJ7XoVIxMSBXGk5RY4dwa" ascii wide

        $cert_cn_str = "GeoTrust LTD." ascii wide

        $cert_sha1 = { 00 6f 5b b9 27 20 ac a3 3a ab 33 50 5e 17 c4 2c c2 f9 f2 36 }

    condition:
        any of them
}
