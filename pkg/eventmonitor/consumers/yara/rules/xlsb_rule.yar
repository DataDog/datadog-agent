/* Realizado por Nerio Rodriguez */
rule xlsb_rule 
{
    meta:
        description = "Regla para correo malicioso"
        author = "Nerio Rodriguez"
        date = "2024-04-15"
	yarahub_uuid = "5ba6c7f5-1c25-46ef-9904-60a78716d140"
        yarahub_reference_md5 = "c2293ce082da26ff050854765bcd0870"
	yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $s1 = "d5f1edd399916227c8551ba8dcd2bd47a1302130db64f2526dbeaa58981dbf45" wide ascii
        $s2 = "c2293ce082da26ff050854765bcd0870" wide ascii
    condition:
        all of them
}

