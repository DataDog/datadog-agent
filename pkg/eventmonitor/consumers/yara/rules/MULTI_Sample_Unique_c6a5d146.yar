rule MULTI_Sample_Unique_c6a5d146
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (inconnu, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "c6a5d146e87e80d5fe6fbc5c337841b22b1cd124819388716054289f60d7dac6"
        yarahub_uuid = "9df3a470-cc28-4360-ac0b-54c5e1c0ca98"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "inconnu"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6adb164474d1546d82c242352ba53f49"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "provided by Glazed. By accessing, downloading, installing, or otherwise using the Work, you (" ascii
        $s1 = "agree to the terms set forth in this Agreement." ascii
        $s2 = "# 1. Grant of License" ascii
        $s3 = "# 2. Restrictions" ascii
        $s4 = "Sell, sublicense, lease, lend, rent, redistribute, or otherwise make the Work available to any third party." ascii
        $s5 = "Use the Work, in whole or in part, in any product or project intended for profit or commercial benefit." ascii
        $s6 = "Modify, adapt, translate, reverse-engineer, decompile, or disassemble the Work for the purpose of distribution or sale." ascii
        $s7 = "Upload, publish, or share the Work publicly in any form, including on public code repositories, marketplaces, servers, or file-sharing platforms." ascii
        $s8 = "Claim authorship, ownership, or creative credit of the Work in any form." ascii
        $s9 = "Incorporate any part of the Work into other software, addons, or services intended for distribution or commercial use." ascii
        $s10 = "# 3. Ownership" ascii
        $s11 = "# 4. Termination" ascii
        $s12 = "This Agreement is effective until terminated. It will terminate automatically without notice if You breach any of the terms and conditions stated herein. Upon termination, You must immediately delete" ascii
        $s13 = "# 5. Enforcement and Legal Remedies" ascii
        $s14 = "# 6. Disclaimer of Warranty" ascii
        $s15 = "# 7. Limitation of Liability" ascii
        $s16 = "# 8. Contact and Inquiries" ascii
        $s17 = "[charis.giannitsaros@gmail.com](mailto:charis.giannitsaros@gmail.com)**" ascii
        $s18 = "All Rights Reserved**" ascii
        $s19 = "This License Agreement (" ascii
    condition:
        true and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
