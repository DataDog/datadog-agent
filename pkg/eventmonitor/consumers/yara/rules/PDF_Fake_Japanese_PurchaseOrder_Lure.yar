rule PDF_Fake_Japanese_PurchaseOrder_Lure {
    meta:
        description = "Detects fake Japanese purchase order PDF lure files containing specific localized title bar strings and Adobe Acrobat update errors"
        author = "Serhii Kocherhan"
        date = "2026-09-03"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "23288f42-77da-4374-9d9b-2707714d505e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "356fa97f14f8863a6d1e0048fad45116"

    strings:
        // PDF Magic Header
        $pdf_magic = { 25 50 44 46 2D }

        // Specific Japanese Lure Term
        $jp_po = { E7 99 BA E2 81 84 E6 9B B8 }

        // Exact String Signatures
        $str_zoom     = "55.9%" ascii wide
        $str_dim      = "8.27 x 11.69 in" ascii wide
        $str_adobe    = "Adobe Acrobat" ascii wide
        $str_update   = "Optional update delivery is not working" ascii wide
        $str_title = { E6 97 A5 E6 9C AC E9 87 87 E8 B4 AD E8 AE A2 E5 8D 95 E6 A8 A1 E7 B3 A1 E6 8F 90 E7 A4 BA E7 89 88 20 2D 20 41 64 6F 62 65 20 41 63 72 6F 62 61 74 20 52 65 61 64 65 72 20 28 33 32 2D 62 69 74 29 }
        $str_telemetry = "acroipm2.adobe.com" ascii wide

    condition:
        // Check for PDF header within the first 1024 bytes and file size limit
        $pdf_magic at 0 and filesize < 10MB and
        (
            // Primary Match: High-confidence unique title bar string or update prompt
            $str_title or
            (
                $jp_po and 3 of ($str_zoom, $str_dim, $str_adobe, $str_update, $str_telemetry)
            )
            or
            // Broad Fallback Match: 4 or more of the specific artifact strings together
            4 of ($str_*)
        )
}