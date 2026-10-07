/*
 * MIRAI_OHSHIT -- ELF payloads, all architectures
 * Drosera honeypot capture, 2026-08-07
 *
 * SCOPE: the 7 captured ELF builds (ARM, ARM7, SH4, MIPS, i686, PowerPC,
 * x86-64). The shell loader chain is covered separately by
 * MIRAI_OHSHIT_loader.yar.
 *
 * Architecture-independent by design. The C2 domain set is compiled
 * identically into all seven builds, so matching on strings rather than code
 * also covers builds we never captured -- 8 of the 15 the loader fetches.
 *
 * This rule also carries XOR-0x22 strings. Mirai's table.c XORs its config
 * with the four bytes of TABLE_KEY; the leaked default 0xdeadbeef collapses
 * to one effective byte, 0xef^0xbe^0xad^0xde == 0x22. These builds hide a
 * Layer-7 HTTP flood kit there, invisible to plaintext extraction.
 */

rule MIRAI_OHSHIT_payload
{
    meta:
        description  = "MIRAI_OHSHIT ELF payload -- compiled-in C2 set, all architectures"
        author       = "AfterPacket"
        date         = "2026-08-07"
        hash_sha256  = "81ea2a392b6d71c7cd381f48ebffb992457a1d3897d1484a3fd7823f2e9f69a3"
        hash_sha256_2 = "9d7cd4948a1fcbaeadc425752fce9a933bd6fc41eeede030dffd7b99b3bc51d5"
        family       = "MIRAI_OHSHIT"
        tlp          = "TLP:WHITE"
        reference    = "https://github.com/Afterpacket/drosera-threat-intel"
        status       = "PRODUCTION"

        /* YARAhub / YARAify submission metadata.
         * reference_md5 is //bot.arm7 (sha256 81ea2a39...), an ELF carrying the
         * compiled-in C2 domain set this rule matches on. */
        yarahub_uuid              = "c81b4e07-5a29-4d3f-b7e6-1f04a9c2d85b"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "ac1b52b71229469fa2a87ea8a11db607"
        yarahub_reference_link    = "https://github.com/Afterpacket/drosera-threat-intel"
        yarahub_author_twitter    = "@AfterPacket"
        yarahub_author_email      = "AfterPacketTru@protonmail.com"
        /* malpedia_family intentionally omitted. "elf.mirai" is a real
         * Malpedia slug (malpedia.caad.fkie.fraunhofer.de/details/elf.mirai
         * resolves) but YARAify rejects it with "Illegal value for field
         * malpedia_family", and its guidelines do not document what the
         * validator accepts. The field is optional, so it is dropped rather
         * than guessed at. Do not re-add without testing against YARAify. */

    strings:
        /* C2 domains -- identical across all 7 captured builds.
         * Each is individually rare enough to stand alone. */
        $d1 = "api-relay-3.metrics-collector.io" ascii
        $d2 = "cdn-edge-updates.hostcloud-eu.net" ascii
        $d3 = "mgmt-panel.serverstats-daemon.com" ascii
        $d4 = "sync.softwaremirror.workers.dev" ascii
        $d5 = "glibc.malloc.top" ascii
        $d6 = "control.tor2web-relay-fast.onion" ascii

        /* C2 IPs, compiled in alongside the domains */
        $ip1 = "45.61.161.207" ascii
        $ip2 = "45.83.140.28" ascii
        $ip3 = "5.101.221.87" ascii
        $ip4 = "51.15.68.114" ascii
        $ip5 = "94.130.53.201" ascii
        $ip6 = "195.201.24.6" ascii

        /* Retrieval template used by the payload itself */
        $cap1 = "/bins.sh" ascii

        /* XOR-0x22 obfuscated Layer-7 flood config. Present in all 7 builds
         * and invisible to plaintext string extraction. The UA-scrape URL is
         * the highest-confidence member -- it has no benign reason to appear
         * inside an IoT binary, obfuscated or otherwise. */
        $x1 = "www.useragentstring.com" xor(0x22)
        $x2 = "Accept-Language:" xor(0x22)
        $x3 = "Accept-Encoding:" xor(0x22)
        $x4 = "https://news.ycombinator.com/" xor(0x22)

        /* Deliberately NOT matched on: bugs.launchpad.net (glibc
         * boilerplate), 185.199.108.153 (GitHub Pages) and 104.21.234.17
         * (Cloudflare) all appear in these binaries and would drag in
         * unrelated software. Nor on the 20 Referer domains recovered from
         * the XOR region -- they are google.com, facebook.com and similar,
         * and matching them would fire on ordinary software. */

    condition:
        uint32be(0) == 0x7F454C46 and
        (
            /* HIGH: any single C2 domain -- none occur in benign software */
            any of ($d*) or

            /* HIGH: the obfuscated UA-scrape URL is unambiguous on its own */
            $x1 or

            /* HIGH: obfuscated flood scaffolding, two or more members.
             * Plaintext HTTP headers are everywhere; XOR-0x22 ones are not. */
            2 of ($x*) or

            /* MEDIUM: two hardcoded C2 IPs together */
            2 of ($ip*) or

            /* MEDIUM: one C2 IP plus the payload's own fetch template */
            (any of ($ip*) and $cap1)
        )
}
