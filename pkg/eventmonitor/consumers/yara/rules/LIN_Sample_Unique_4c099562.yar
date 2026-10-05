rule LIN_Sample_Unique_4c099562
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (elf, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "4c09956231aef80c4b49ca9cda029843641f0c453820d02ad1bef0f810d1a192"
        yarahub_uuid = "85cdc771-dbc1-4330-9869-852d5a4ba7fb"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "elf"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "1a09777f1ac9443b4f4271bdec58c188"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "knLOiYcaHXE4IxN9DgHe/CLhRnT0HPwbzDPt406pw/hiQMWXhQJoj90phDexd7/e2vW0hb01vWsqKv-xANV" ascii
        $s1 = "debugCalH9" ascii
        $s2 = "runtime." ascii
        $s3 = "dev/nulH" ascii
        $s4 = "runtime.H9" ascii
        $s5 = "reflect.H9" ascii
        $s6 = "UUUUUUUUH!" ascii
        $s7 = "P H9S uqH" ascii
        $s8 = "debugCal" ascii
        $s9 = "runtime L" ascii
        $s10 = "error: L" ascii
        $s11 = "wwwwwwwwH!" ascii
        $s12 = "wwwwwwwwH" ascii
        $s13 = "GODEBUG=H9" ascii
        $s14 = "memprofiL9" ascii
    condition:
        uint32(0) == 0x464C457F and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
