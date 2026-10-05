rule Web_Phishing_JavaScript_Anchor_Redirector {
    meta:
        description = "Detects compact phishing redirector pages using JavaScript window.location anchor fragment splitting to pass payload URLs dynamically."
        author = "Serhii Kocherhan"
        date = "2026-09-19"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "b821e857-f538-4845-ab42-cf1823ee0b9a"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "972ce3b4c808ebed9dc2637104dbe6b2"

    strings:
        // Regular expression matching document.location.href assignment with any HTTP/HTTPS URL concatenated with split('#')[1]
        $re_redirect = /<script>\s*document\.location\.href\s*=\s*['"]https?:\/\/[^'"]+['"]\s*\+\s*window\.location\.href\.split\('#'\)\[1\];\s*<\/script>/ ascii wide nocase

        // Fallback string matching for anchor-fragment URL extraction
        $js_loc_href = "document.location.href" ascii wide nocase
        $js_split_hash = ".split('#')[1]" ascii wide nocase

    condition:
        filesize < 150 and (
            $re_redirect or
            ($js_loc_href and $js_split_hash)
        )
}