rule stealc_ioc_hifi {
    meta:
        author = "manb4t"
        description = "Simple string rule to identify current stealc samples"
        date = "2024-04-28"
        yarahub_author_twitter = "@knappresearchlb"
        yarahub_reference_md5 = "fe1fa198626701a72893c05b5e3c7d0c"
        sha256 = "93f357d221fc7f72bec7195e11c8a00b9e128448850a88ca66c8cc95fa47272f"
        yarahub_uuid = "f695b517-b316-4f57-9254-dbe90d4c5215"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.stealc"
    strings:
        $av1 = ".?AVexception@std@@" nocase ascii wide
        $av2 = ".?AVtype_info@@" nocase  ascii wide
        $av3 = ".?AVout_of_range@std@@" nocase  ascii wide
        $av4 = ".?AVlength_error@std@@"  nocase  ascii wide
        $av5 = ".?AVlogic_error@std@@" nocase ascii wide
        $av6 = ".?AVbad_alloc@std@@" nocase ascii wide
        $av7 = ".?AV_Iostream_error_category@std@@" nocase ascii wide
        $av8 = ".?AV_System_error_category@std@@" nocase ascii wide
        $av9 = ".?AVbad_exception@std@@" nocase ascii wide
        $av10 = ".?AVerror_category@std@@" nocase ascii wide
        $av11 = ".?AV_Generic_error_category@std@@" nocase ascii wide
        $genstr1 = "kernel32.dll" nocase ascii wide
        $genstr2 = "1#SNAN" nocase ascii wide
        $genstr3 = "1#QNAN" nocase ascii wide
        $stru1 = "msimg32.dll" nocase wide
        $stru2 = "mscoree.dll" nocase wide
        $stru3 = "USER32.DLL" nocase wide
        $stru4 = "Copyright (C) 2022, Cry" nocase wide
        $pdb = ".pdb" nocase ascii wide         
    condition:
        uint16(0) == 0x5a4d and
        all of ($av*) and
        2 of ($genstr*) and
        4 of ($stru*) and $pdb
}