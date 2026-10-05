rule Web_Generic_Binary_URL_Download {
    meta:
        description = "Detects files containing HTTP or HTTPS links pointing directly to .bin payload extensions."
        author = "Serhii Kocherhan"
        date = "2026-09-20"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "9e9960fa-1f18-463f-8ed6-9c26cd8622d9"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "849e5a552bdc668eb9ff741a9a9ded08"

    strings:
        // PCRE pattern matching HTTP/HTTPS URLs ending with .bin extension
        $re_bin_url = /https?:\/\/[a-zA-Z0-9\.\-_%+=?&\/]+\.bin/ ascii wide nocase

    condition:
        filesize < 10MB and $re_bin_url
}