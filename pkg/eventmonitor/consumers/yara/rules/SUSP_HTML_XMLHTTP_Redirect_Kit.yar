rule SUSP_HTML_XMLHTTP_Redirect_Kit {
    meta:
        description = "HTML avec exfiltration XMLHTTP + redirection window.location - kit phishing frais"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / cluster HTML"
        confidence  = "medium"
        yarahub_uuid            = "4ac7b303-e9fd-4adf-9272-5ada3f70853f"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "cbf7cc2ec7f0221a48329441517add09"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "<script" ascii wide
        $b = "XMLHTTP" ascii wide nocase
        $c = "window.location" ascii wide nocase
    condition:
        all of them
}

