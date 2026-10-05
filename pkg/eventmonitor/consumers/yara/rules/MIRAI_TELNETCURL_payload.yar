/*
 * MIRAI_TELNETCURL -- ELF payloads, 5 architectures
 * Drosera honeypot capture, 2026-08-01 .. 2026-08-07
 *
 * SCOPE: the 5 captured ELF builds (arm, arm5, arm7, mips, mipsel). The two
 * shell droppers are covered separately by MIRAI_TELNETCURL_dropper.yar.
 *
 * These payloads carry NO obfuscated configuration. Verified 2026-08-08 by
 * sweeping the full single-byte XOR keyspace, including 0x22 -- the effective
 * key of Mirai's table.c under the leaked-source TABLE_KEY 0xdeadbeef. Zero
 * meaningful strings recovered at any key. Contrast MIRAI_OHSHIT, whose
 * payloads DO hide an HTTP flood kit at 0x22.
 */

rule MIRAI_TELNETCURL_payload
{
    meta:
        description  = "Mirai-lineage ELF payload carrying the leaked-source decoy domain"
        author       = "AfterPacket"
        date         = "2026-08-07"
        hash_sha256  = "9cbe35b1d10f55d712738644c60c2cc47eac13e06f23ba849abb1bbdbdfdc5f2"
        hash_sha256_2 = "3afa3a117915cf21b105ea9c8aae346af4f733556e6a6fe62e1352901d2b4831"
        family       = "MIRAI_TELNETCURL"
        tlp          = "TLP:WHITE"
        reference    = "https://github.com/Afterpacket/drosera-threat-intel"
        status       = "PRODUCTION"
        note         = "Lineage marker, not a family discriminator -- see condition"

        /* YARAhub / YARAify submission metadata.
         * reference_md5 is /arm5 (sha256 9cbe35b1...), an ELF carrying the
         * leaked-source decoy domain this rule matches on. */
        yarahub_uuid              = "a45e8b71-2f6d-4c09-b83a-5d1e7f4a9c60"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "dfbbf96df412f2d3d1358aaae94e1123"
        yarahub_reference_link    = "https://github.com/Afterpacket/drosera-threat-intel"
        yarahub_author_twitter    = "@AfterPacket"
        yarahub_author_email      = "AfterPacketTru@protonmail.com"
        /* malpedia_family intentionally omitted -- see the note in
         * MIRAI_OHSHIT_payload.yar. "elf.mirai" is a valid Malpedia slug but
         * YARAify rejects it as an illegal value. The field is optional. */

    strings:
        /* The decoy domain from the leaked Mirai source, present in all five
         * captured builds. It is a LINEAGE marker: any Mirai descendant that
         * kept the original resolver code carries it, so this rule identifies
         * Mirai-derived payloads generally, NOT this campaign specifically.
         * Attribute to the family only with corroborating infrastructure. */
        $decoy = "www.ikindalikemenbutonlyontuesday.com" ascii

        /* Scanner range constants that accompany it in these builds */
        $r1 = "119.0.0.0" ascii
        $r2 = "120.0.0.0" ascii
        $r3 = "121.0.0.0" ascii

    condition:
        uint32be(0) == 0x7F454C46 and
        (
            $decoy or
            2 of ($r*)
        )
}
