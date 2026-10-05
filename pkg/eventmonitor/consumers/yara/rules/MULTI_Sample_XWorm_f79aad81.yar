rule MULTI_Sample_XWorm_f79aad81
{
    meta:
        author = "Marjoriefort"
        description = "Detects XWorm (inconnu, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "f79aad8142292a24c23dfec0baafa9eda348b9b4325430006f39f6f36c03a2bb"
        yarahub_uuid = "373d5134-6a48-4332-98a3-22734c1fef86"
        famille = "XWorm"
        famille_source = "reputation"
        classe = "inconnu"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d972b9c4b2e0b89680dd48965c112c48"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "trimly.Global = True" wide ascii
        $s1 = "trimly.Pattern = organotherapy" wide ascii
        $s2 = "bepotastine = trimly.Replace(asectarian, melchizedekite)" wide ascii
        $s3 = "Set kebob = CreateObject(\"WScript.Shell\")" wide ascii
        $s4 = "Set cerotene = kebob.Environment(\"USER\")" wide ascii
        $s5 = "For always = 1 To Len(uncontroverted) Step pailful" wide ascii
        $s6 = "sepiophora = Left(sepiophora, Len(sepiophora) - 1)" wide ascii
        $s7 = "($colonoscopy)|Out-Null;[" wide ascii
        $s8 = "RMP.Cls_LJahnPKDWM]::Met_LJahnPKDWMPq('V" wide ascii
        $s9 = "Set dispensed = ruralist.Get(scoticismt).SpawnInstance_" wide ascii
        $s10 = "dispensed.ShowWindow = 0" wide ascii
        $s11 = "Set pyebald = ruralist.Get(poststimulus)" wide ascii
        $s12 = "chipmaking = pyebald.Create(microatolls, Null, dispensed, showgoer)" wide ascii
        $s13 = "kebob.Run microatolls, 0, True" wide ascii
        $s14 = "Function pleaseth()" wide ascii
        $s15 = "uncontroverted = \"T" wide ascii
        $s16 = "uncontroverted = uncontroverted & \"" wide ascii
        $s17 = "uncontroverted = uncontroverted & \"A" wide ascii
        $s18 = "uncontroverted = uncontroverted & \"s" wide ascii
        $s19 = "uncontroverted = uncontroverted & \"X" wide ascii
    condition:
        true and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
