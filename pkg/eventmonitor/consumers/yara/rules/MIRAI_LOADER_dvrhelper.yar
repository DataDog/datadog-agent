/*
 * MIRAI_LOADER -- unobfuscated Mirai loader, dvrHelper payload
 * Drosera honeypot capture, 2026-08-03
 *
 * This family is self-identifying: it fetches a path literally named
 * mirai.<arch> and writes it to dvrHelper, the classic Mirai binary name.
 *
 * Note "dvrHelper" alone is deliberately NOT sole-condition -- it is a
 * widely reused Mirai artefact name and appears in unrelated samples,
 * detection content, and in this same capture inside a rival operator's
 * kill script (see BOTKILL_PROCWIPE). Matching it alone would misattribute.
 */

rule MIRAI_LOADER_dvrhelper
{
    meta:
        description  = "Mirai loader fetching mirai.<arch> from 77.90.185.66, writes dvrHelper"
        author       = "AfterPacket"
        date         = "2026-08-07"
        hash_sha256  = "246c3c37a0de2987d9352411f22ea4805f7e75287f2782d5ee56770219a096a4"
        family       = "MIRAI_LOADER"
        tlp          = "TLP:WHITE"
        reference    = "https://github.com/Afterpacket/drosera-threat-intel"
        status       = "PRODUCTION"

        /* YARAhub / YARAify submission metadata.
         * reference_md5 is /wget (sha256 246c3c37...), carrying $uniq1
         * SERVER="77.90.185.66". */
        yarahub_uuid              = "52a9f4c6-7b38-4e1d-b204-9c6a1d8e35f7"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "3d39989e0700b86e57f81ef0183b0049"
        yarahub_reference_link    = "https://github.com/Afterpacket/drosera-threat-intel"
        yarahub_author_twitter    = "@AfterPacket"
        yarahub_author_email      = "AfterPacketTru@protonmail.com"

    strings:
        /* High-confidence unique strings */
        $uniq1 = "SERVER=\"77.90.185.66\"" ascii
        $uniq2 = "http://$SERVER/mirai.$arch" ascii
        /* Exec tag, this family's equivalent of MIRAI_TELNETCURL's
         * "telnet.curl" -- dvrHelper alone is a shared Mirai artefact name,
         * but "dvrHelper tscan" together is this loader specifically. */
        $uniq3 = "./dvrHelper tscan" ascii

        /* C2 / staging indicators */
        $c2_1 = "77.90.185.66" ascii

        /* Capability markers */
        $cap1 = "dvrHelper" ascii
        $cap2 = "chmod +x dvrHelper" ascii
        $cap3 = "mirai." ascii
        $cap4 = "chmod 777" ascii
        /* Android staging path -- this loader targets Android as well as
         * Linux, which the directory list makes explicit. */
        $cap5 = "/data/local/tmp" ascii
        $cap6 = "ARCHS=\"arm arm5 arm7 mips mpsl\"" ascii

    condition:
        filesize < 64KB and
        (
            /* HIGH: config assignment or the literal fetch template */
            any of ($uniq*) or

            /* MEDIUM: staging host plus two capability markers.
             * dvrHelper alone is intentionally insufficient -- it is a
             * shared Mirai artefact name, not a family discriminator. */
            ($c2_1 and 2 of ($cap*))
        )
}
