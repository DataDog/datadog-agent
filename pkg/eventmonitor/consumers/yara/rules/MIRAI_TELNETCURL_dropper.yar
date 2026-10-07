/*
 * MIRAI_TELNETCURL -- telnet-propagated Mirai variant, 5 architectures
 * Drosera honeypot capture, 2026-08-01 .. 2026-08-07
 *
 * SCOPE: the two shell droppers (/curl.sh and /wget.sh), which are the curl
 * and busybox-wget variants of one script. The 5 ELF payloads are covered
 * separately by MIRAI_TELNETCURL_payload.yar.
 *
 * The per-architecture output filenames are non-dictionary six-character
 * tokens, stable across both dropper variants. They are the strongest
 * available signal for this family.
 */

rule MIRAI_TELNETCURL_dropper
{
    meta:
        description  = "Mirai telnet dropper staging from 205.237.110.232, exec tag telnet.curl"
        author       = "AfterPacket"
        date         = "2026-08-07"
        hash_sha256  = "3801a288c16a19c57c7a8a7b0f139cf630d2cd0c4bbcb26876e3593c492ffc5d"
        hash_sha256_2 = "e1568cae97252fa9350ef2d2d381975c8bd29e11f126fb06bd64e92a73d7beb9"
        family       = "MIRAI_TELNETCURL"
        tlp          = "TLP:WHITE"
        reference    = "https://github.com/Afterpacket/drosera-threat-intel"
        status       = "PRODUCTION"

        /* YARAhub / YARAify submission metadata.
         * reference_md5 is /curl.sh (sha256 3801a288...), carrying $uniq1
         * "telnet.curl" plus the staging host. */
        yarahub_uuid              = "6d2f9a45-b13c-4e78-8f05-7a9d3b6c1e24"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "688c6311320aa416a88cf6eed3d5f8f5"
        yarahub_reference_link    = "https://github.com/Afterpacket/drosera-threat-intel"
        yarahub_author_twitter    = "@AfterPacket"
        yarahub_author_email      = "AfterPacketTru@protonmail.com"

    strings:
        /* High-confidence unique strings */
        $uniq1 = "telnet.curl" ascii

        /* C2 / staging indicators */
        $c2_1 = "205.237.110.232" ascii
        $c2_2 = "http://205.237.110.232/" ascii

        /* Fixed per-architecture output filenames.
         * name1-5 are fetched. name6-7 appear ONLY in the dropper's rm -rf
         * preamble and are never downloaded -- they are cleanup targets for
         * architecture builds this kit ships but these two hosts did not
         * serve. They are hunt strings for builds we have not captured. */
        $name1 = "VFASXC" ascii
        $name2 = "WQZRTY" ascii
        $name3 = "YUIOXC" ascii
        $name4 = "GHJKLB" ascii
        $name5 = "MNCXOP" ascii
        $name6 = "PLXMKJ" ascii
        $name7 = "KFGDFG" ascii

        /* Capability markers */
        $cap1 = "chmod 777" ascii
        $cap2 = "busybox wget" ascii

    condition:
        filesize < 64KB and
        (
            /* HIGH: three of the fixed filenames together cannot be chance */
            3 of ($name*) or

            /* HIGH: exec tag plus staging host */
            ($uniq1 and any of ($c2_*)) or

            /* MEDIUM: staging host plus a filename plus dropper behaviour */
            (any of ($c2_*) and any of ($name*) and any of ($cap*))
        )
}
