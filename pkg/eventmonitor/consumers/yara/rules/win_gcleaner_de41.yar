rule win_gcleaner_de41 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-05-29"
        description               = "detects GCleaner"
        hash1_md5                 = "8151e61aec021fa04bce8a30ea052e9d"
        hash1_sha1                = "4b972d2e74a286e9663d25913610b409e713befd"
        hash1_sha256              = "868fceaa4c01c2e2ceee3a27ac24ec9c16c55401a7e5a7ca05f14463f88c180f"
        hash2_md5                 = "7526665a9d5d3d4b0cfffb2192c0c2b3"
        hash2_sha1                = "13bf754b44526a7a8b5b96cec0e482312c14838c"
        hash2_sha256              = "bb5cd698b03b3a47a2e55a6be3d62f3ee7c55630eb831b787e458f96aefe631b"
        hash3_md5                 = "a39e68ae37310b79c72025c6dfba0a2a"
        hash3_sha1                = "ae007e61c16514a182d21ee4e802b7fcb07f3871"
        hash3_sha256              = "c5395d24c0a1302d23f95c1f95de0f662dc457ef785138b0e58b0324965c8a84"
        malpedia_family           = "win.gcleaner"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "8151e61aec021fa04bce8a30ea052e9d"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "de41ff50-28a7-4a09-86dc-f737f8858354"

    strings:
        $accept = "Accept: text/html, application/xml;q=0.9, application/xhtml+xml, image/png, image/jpeg, image/gif, image/x-xbitmap, */*;q=0.1"
        $accept_lang = "Accept-Language: ru-RU,ru;q=0.9,en;q=0.8"
        $accept_charset = "Accept-Charset: iso-8859-1, utf-8, utf-16, *;q=0.1"
        $accept_encoding = "Accept-Encoding: deflate, gzip, x-gzip, identity, *;q=0"

        $unkown = "<unknown>"
        $cmd1 = "\" & exit"
        $cmd2 = "\" /f & erase "
        $cmd3 = "/c taskkill /im \""

        $anti1 = " Far "
        $anti2 = "roxifier"
        $anti3 = "HTTP Analyzer"
        $anti4 = "Wireshark"
        $anti5 = "NetworkMiner"

        $mix1 = "mixshop"
        $mix2 = "mixtwo"
        $mix3 = "mixnull"
        $mix4 = "mixazed"

    condition:
        uint16(0) == 0x5A4D and
        15 of them
}
