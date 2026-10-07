rule gafgyt_langflow_sbox_cipher
{
    meta:
        author = "Nokia Deepfield ERT"
        description = "Customized Gafgyt/BASHLITE DDoS bot delivered via Langflow CVE-2025-3248; identifies the actor's embedded affine S-box, 16-byte magic key, and LCG-driven stream cipher used for the encrypted C2 protocol"
        date = "2026-07-15"
        family = "gafgyt"
        severity = "high"
        reference = "e00d92ca28a2cfd75e96f71fc0408747f04942657fcab0f2a25ce79bc3ad23a8"
        reference_url = "https://www.akamai.com/blog/security-research/2026/jul/langflow-exploited-build-custom-ddos-gafgyt-botnets"
        yarahub_uuid = "3fc6d7a0-c14d-425a-9af5-f2992632320f"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "85e2c18dbb454c7651b76dd52259ddbd"

    strings:
        // Embedded 256-byte affine S-box: sbox[i] = (0x0d + 0xa7*i) & 0xff.
        // First 24 entries; actor-unique, present in every arch build (stored in .data).
        $sbox = { 0d b4 5b 02 a9 50 f7 9e 45 ec 93 3a e1 88 2f d6 7d 24 cb 72 19 c0 67 0e }

        // 16-byte cipher key: DEADBEEF CAFEBABE E0A4CBD6 BADC0DE5 (little-endian dwords).
        // 0xE0A4CBD6 is also the PRGA/LCG init seed. Actor-unique, cross-arch.
        $key = { ef be ad de be ba fe ca d6 cb a4 e0 e5 0d dc ba }

        // sockaddr_in for the C2: AF_INET (0x0002) + port 1337 (htons -> 39 05).
        // Supporting only; port/address may change across builds.
        $sockaddr_c2 = { 02 00 39 05 }
        $c2_ip = "184.174.96.191"

    condition:
        uint32(0) == 0x464c457f and
        (
            $sbox or
            $key or
            ($sockaddr_c2 and $c2_ip)
        )
}
