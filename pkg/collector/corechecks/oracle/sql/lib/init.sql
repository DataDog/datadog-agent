SET VERIFY OFF
SET FEEDBACK OFF

VAR connection_type VARCHAR2(3);
VAR connection_type_cdb VARCHAR2(3);
exec :connection_type_cdb := 'CDB';

BEGIN
  SELECT DECODE(sys_context('USERENV','CON_ID'),1,:connection_type_cdb,'PDB') INTO :connection_type from DUAL;
END;
/

var hostingType varchar2(30);
var hostingTypeSelfManaged varchar2(30);
var hostingTypeRDS varchar2(30);
var hostingTypeOCI varchar2(30);
exec :hostingTypeSelfManaged := 'selfManaged';
exec :hostingTypeRDS := 'rds';
exec :hostingTypeOCI := 'oci';

exec :hostingType := :hostingTypeSelfManaged;

declare
  path_dir varchar2(30);
	oci_entries integer;
begin
  SELECT SUBSTR(name, 1, 10) path INTO path_dir FROM v$datafile WHERE rownum = 1 ;
  if path_dir = '/rdsdbdata' then
    :hostingType := :hostingTypeRDS;
  end if;

	if :hostingType = :hostingTypeSelfManaged and :connection_type = :connection_type_cdb then
		begin
			execute immediate 'SELECT count(*) INTO oci_entries FROM v$pdbs WHERE cloud_identity like ''%oraclecloud%'' and rownum = 1' into oci_entries;
			if oci_entries = 1 then
				:hostingType := :hostingTypeOCI;
			end if;
		exception
			when others then
				null;
		end;
	end if;
end;
/
