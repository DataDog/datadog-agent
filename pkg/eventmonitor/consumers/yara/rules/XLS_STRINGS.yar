rule XLS_STRINGS
{
    meta:
        description = "Detect Strings targeting Bangladesh"
        author = "somedieyoungZZ"
        date = "2024-10-24"
        yarahub_uuid = "2dd15342-3366-4134-89fd-1ded326d59e7"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "de83e25b881411e72d27b8c83c320997"

    strings:
        $string1 = "SUZHOU SHENGCHENG TEXTILE CO.,LTD."
        $string2 = "79 Xiushui Road,South Third Ring,Group 12"
        $string3 = "Shengtang Village, Shengze Town, Wujiang District"
        $string4 = "Microsoft Office Excel 2003 Worksheet"
        $string5 = "ABM TOWER, 671/1 SHOLAKBAHAR, BAHADDARHAT"
        $string6 = "Chittagong, Bangladesh"
        $string7 = "Vrai"
        $string8 = "Faux"

        $xls_magic = { D0 CF 11 E0 A1 B1 1A E1 }
        $xls_worksheet_identifier = "Workbook"

    condition:
        (uint32(0) == 0xE011CFD0) and
        $xls_magic and
        any of ($string*) and
        $xls_worksheet_identifier
}

