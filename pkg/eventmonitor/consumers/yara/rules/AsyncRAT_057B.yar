rule AsyncRAT_057B
{
    meta:
        id = "4j2WP1mJD3cNE60ZyHsOgC"
        fingerprint = "efcb736b80c4dfc515395045c34a26a46274fec546dd80d571b8dbc090180f71"
        version = "1.0"
        date = "2026-03-31"
        modified = "2026-03-31"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "AsyncRAT 0.5.7B -- minimal .NET RAT with PBKDF2 key derivation and MessagePack serialization"
        category = "MALWARE"
        malware = "ASYNCRAT"
        mitre_att = "T1219"

        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "AsyncRAT 0.5.7B .NET payload detected."
        yarahub_uuid = "2f0773d9-650c-4444-b237-0bd105d94994"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "addb2f9bc9ffad336cbee648fdfcf138"

    strings:
        // Canonical AsyncRAT PBKDF2 salt -- 32 raw bytes embedded in .NET resource
        // bfeb1e56fbcd973bb219022430a57843003d5644d21e62b9d4f180e7e6c33941
        $salt = { bf eb 1e 56 fb cd 97 3b b2 19 02 24 30 a5 78 43 00 3d 56 44 d2 1e 62 b9 d4 f1 80 e7 e6 c3 39 41 }

        // Client namespace
        $ns_handle = "Handle_Packet" ascii

        // Command strings (no underscore, unlike VenomRAT)
        $cmd_save = "savePlugin" ascii wide
        $cmd_pong = "pong" ascii wide

        // TLS cert field name (differs from VenomRAT's Server_Certificate)
        $cert_field = "ServerCertificate" ascii

        // MessagePack serialization library
        $msgpack = "MessagePackLib" ascii

    condition:
        uint16(0) == 0x5A4D
        and filesize > 20KB and filesize < 500KB
        and $salt
        and $ns_handle
        and ($cmd_save or $cmd_pong)
        and ($cert_field or $msgpack)
}
