rule SUS_UNC_InEmail
{
	meta:
		author = "Nicholas Dhaeyer - @DhaeyerWolf"
		date = "2023-05-15"
		description = "Looks for a suspicious UNC string in .eml files & .ole files"
		yarahub_uuid = "7df969ed-49f8-4c52-be25-6511d6dcc37f"
		yarahub_license = "CC BY-SA 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "1ac728095ebedb5d25bea43e69014bc4"
	  
	strings:
		$MAGIC_MSG = {D0 CF 11 E0 A1 B1 1A E1} // sadly the .msg message byte is the same as the one for other OLE files
		$MAGIC_EML = {52 65 63 65 69 76 65 64 3A} // Magic byte for .eml files: "Received:"
		$MAGIC_ICS = {42 45 47 49 4E 3A 56 43 41 4C 45 4E 44 41 52} // "BEGIN:VCALENDAR"
		
		$Appointment = "IPM.Appointment"
		
		$UNC = {00 5C 5C} 
	  
	condition:
		$UNC and ($MAGIC_MSG at 0 or $MAGIC_EML at 0 or $MAGIC_ICS at 0) and $Appointment
}