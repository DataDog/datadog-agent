/*
 * MIRAI_OHSHIT -- multi-architecture IoT botnet loader chain
 * Drosera honeypot capture, 2026-08-07
 *
 * SCOPE: the shell loader chain -- /tmp/.p (104 B stage-1 fetcher) and
 * /ohshit.sh (stage-2 multi-arch loop). The 7 ELF payloads are covered
 * separately by MIRAI_OHSHIT_payload.yar.
 *
 * ohshit.sh loops 15 architectures, one fetch per line (lines 4-18), cats
 * each payload into a file named WTF, chmods and executes. 7 of the 15 builds
 * were captured. (Corrected 2026-08-08: earlier revisions said 16.)
 */

rule MIRAI_OHSHIT_loader
{
    meta:
        description  = "Mirai-derived multi-arch loader chain staged from 94.154.43.123"
        author       = "AfterPacket"
        date         = "2026-08-07"
        hash_sha256  = "8b1a2fb6b358484b7769aeeb63209f2b277d91b5015cf28ce471a67e0ef83d28"
        hash_sha256_2 = "de9cfdf7d1330534731e8354ec5927db99e06365850a8fe1d07c6bf8dec97ad0"
        family       = "MIRAI_OHSHIT"
        tlp          = "TLP:WHITE"
        reference    = "https://github.com/Afterpacket/drosera-threat-intel"
        status       = "PRODUCTION"

        /* YARAhub / YARAify submission metadata.
         * reference_md5 is /ohshit.sh (sha256 8b1a2fb6...), which carries
         * $uniq2 "94.154.43.123//bot." -- this rule matches it. */
        yarahub_uuid              = "3f7a1c92-8d4e-4b61-9a3f-2c5e8d017b46"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "ffed68702b8dccbccbd3634bb647f603"
        yarahub_reference_link    = "https://github.com/Afterpacket/drosera-threat-intel"
        yarahub_author_twitter    = "@AfterPacket"
        yarahub_author_email      = "AfterPacketTru@protonmail.com"

    strings:
        /* High-confidence unique strings */
        $uniq1 = "ohshit.sh" ascii
        /* The doubled slash after the host is characteristic and low-noise */
        $uniq2 = "94.154.43.123//bot." ascii

        /* C2 / staging indicators */
        $c2_1 = "94.154.43.123" ascii
        $c2_2 = "http://94.154.43.123/ohshit.sh" ascii

        /* Capability markers -- the loader's staging idiom */
        $cap1 = ">WTF" ascii
        $cap2 = "chmod +x *;./WTF" ascii
        $cap3 = "cat bot." ascii
        $cap4 = "busybox /tmp/" ascii

    condition:
        /* Shell scripts -- keep the rule off large binaries entirely */
        filesize < 64KB and
        (
            /* HIGH: a unique string alone is sufficient */
            any of ($uniq*) or

            /* MEDIUM: staging host plus loader behaviour */
            (any of ($c2_*) and 2 of ($cap*))
        )
}
