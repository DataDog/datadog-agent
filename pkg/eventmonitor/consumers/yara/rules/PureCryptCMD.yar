rule PureCryptCMD
{
    meta:
        description = "Detects PureCrypters .cmd output"
        author = "01Xyris"
        date = "2024-10-15"
        yarahub_uuid = "2302a4a9-610d-424c-a67a-d0a021e08a17"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e895debe62e7a70683da3407a24990c5"

    strings:
        $chcp_cmd = "@chcp 65001"
        $var1 = "C:\\Win"
        $var2 = "erShel"
        $var3 = "\\Wind"
        $var4 = "owsPow"
        $var5 = "shell."
        $var6 = "/q /y"
        $var7 = "l\\v1.0"
        $var8 = "/h /i"
        $var9 = "exe %~0.Kkm"
        $var10 = "ysWOW6"
        $var11 = "| xco"
        $var12 = "echo F"
        $var13 = "py /d"
        $var14 = "attrib"
        $var15 = "+s +h"
        $var16 = "SET Yi"
        $var17 = "pdqois"
        $var18 = "m.Conv"
        $var19 = "ession"
        $var20 = "ject S"

    condition:
        $chcp_cmd and all of ($var1, $var2, $var3, $var4, $var5) and 5 of ($var6, $var7, $var8, $var9, $var10, $var11, $var12, $var13, $var14, $var15, $var16, $var17, $var18, $var19, $var20)
}
