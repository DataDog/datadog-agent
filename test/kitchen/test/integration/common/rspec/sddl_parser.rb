require 'English'

module SDDLHelper
  @@ace_types = {
    'A' => 'Access Allowed',
    'D' => 'Access Denied',
    'OA' => 'Object Access Allowed',
    'OD' => 'Object Access Denied',
    'AU' => 'System Audit',
    'AL' => 'System Alarm',
    'OU' => 'Object System Audit',
    'OL' => 'Object System Alarm'
  }

  def self.ace_types
    @@ace_types
  end

  @@ace_flags = {
    'CI' => 'Container Inherit',
    'OI' => 'Object Inherit',
    'NP' => 'No Propagate',
    'IO' => 'Inheritance Only',
    'ID' => 'Inherited',
    'SA' => 'Successful Access Audit',
    'FA' => 'Failed Access Audit'
  }

  def self.ace_flags
    @@ace_flags
  end

  @@permissions = {
    'GA' => 'Generic All',
    'GR' => 'Generic Read',
    'GW' => 'Generic Write',
    'GX' => 'Generic Execute',

    'RC' => 'Read Permissions',
    'SD' => 'Delete',
    'WD' => 'Modify Permissions',
    'WO' => 'Modify Owner',
    'RP' => 'Read All Properties',
    'WP' => 'Write All Properties',
    'CC' => 'Create All Child Objects',
    'DC' => 'Delete All Child Objects',
    'LC' => 'List Contents',
    'SW' => 'All Validated Writes',
    'LO' => 'List Object',
    'DT' => 'Delete Subtree',
    'CR' => 'All Extended Rights',

    'FA' => 'File All Access',
    'FR' => 'File Generic Read',
    'FW' => 'File Generic Write',
    'FX' => 'File Generic Execute',

    'KA' => 'Key All Access',
    'KR' => 'Key Read',
    'KW' => 'Key Write',
    'KX' => 'Key Execute'
  }

  def self.permissions
    @@permissions
  end

  @@trustee = {
    'AO' => 'Account Operators',
    'RU' => 'Alias to allow previous Windows 2000',
    'AN' => 'Anonymous Logon',
    'AU' => 'Authenticated Users',
    'BA' => 'Built-in Administrators',
    'BG' => 'Built in Guests',
    'BO' => 'Backup Operators',
    'BU' => 'Built-in Users',
    'CA' => 'Certificate Server Administrators',
    'CG' => 'Creator Group',
    'CO' => 'Creator Owner',
    'DA' => 'Domain Administrators',
    'DC' => 'Domain Computers',
    'DD' => 'Domain Controllers',
    'DG' => 'Domain Guests',
    'DU' => 'Domain Users',
    'EA' => 'Enterprise Administrators',
    'ED' => 'Enterprise Domain Controllers',
    'WD' => 'Everyone',
    'PA' => 'Group Policy Administrators',
    'IU' => 'Interactively logged-on user',
    'LA' => 'Local Administrator',
    'LG' => 'Local Guest',
    'LS' => 'Local Service Account',
    'SY' => 'Local System',
    'NU' => 'Network Logon User',
    'NO' => 'Network Configuration Operators',
    'NS' => 'Network Service Account',
    'PO' => 'Printer Operators',
    'PS' => 'Self',
    'PU' => 'Power Users',
    'RS' => 'RAS Servers group',
    'RD' => 'Terminal Server Users',
    'RE' => 'Replicator',
    'RC' => 'Restricted Code',
    'SA' => 'Schema Administrators',
    'SO' => 'Server Operators',
    'SU' => 'Service Logon User'
  }

  def self.trustee
    @@trustee
  end

  def self.lookup_trustee(trustee)
    if @@trustee[trustee].nil?
      nt_account = `powershell -command "(New-Object System.Security.Principal.SecurityIdentifier('#{trustee}')).Translate([System.Security.Principal.NTAccount]).Value"`.strip
      return nt_account if 0 == $CHILD_STATUS

      # Can't lookup, just return value
      return trustee
    end

    @@trustee[trustee]
  end
end

class SDDL
  def initialize(sddl_str)
    sddl_str.scan(/(.):(.*?)(?=.:|$)/) do |m|
      case m[0]
      when 'D'
        @dacls = []
        m[1].scan(/(\((?<ace_type>.*?);(?<ace_flags>.*?);(?<permissions>.*?);(?<object_type>.*?);(?<inherited_object_type>.*?);(?<trustee>.*?)\))/) do |ace_type, ace_flags, permissions, object_type, inherited_object_type, trustee|
          @dacls.append(DACL.new(ace_type, ace_flags, permissions, object_type, inherited_object_type, trustee))
        end
      when 'O'
        @owner = m[1]
      when 'G'
        @group = m[1]
      end
    end
  end

  attr_reader :owner, :group, :dacls

  def to_s
    str  = "Owner: #{SDDLHelper.lookup_trustee(@owner)}\n"
    str += "Group: #{SDDLHelper.lookup_trustee(@owner)}\n"
    @dacls.each do |dacl|
      str += dacl.to_s
    end
    str
  end

  def ==(other_sddl)
    return false if
      @owner != other_sddl.owner ||
      @group != other_sddl.group ||
      @dacls.length != other_sddl.dacls.length

    @dacls.each do |d1|
      if other_sddl.dacls.find { |d2| d1 == d2 }.eql? nil
        return false
      end
    end

    other_sddl.dacls.each do |d1|
      if @dacls.find { |d2| d1 == d2 }.eql? nil
        return false
      end
    end
  end

  def eql?(other_sddl)
    self == other_sddl
  end

end

class DACL
  def initialize(ace_type, ace_flags, permissions, object_type, inherited_object_type, trustee)
    @ace_type = ace_type
    @ace_flags = ace_flags
    @permissions = permissions
    @object_type = object_type
    @inherited_object_type = inherited_object_type
    @trustee = trustee
  end

  attr_reader :ace_type, :ace_flags, :permissions, :object_type, :inherited_object_type, :trustee

  def ==(other_dacl)
    return false if other_dacl.eql? nil

    @ace_type == other_dacl.ace_type &&
    @ace_flags == other_dacl.ace_flags &&
    @permissions == other_dacl.permissions &&
    @object_type == other_dacl.object_type &&
    @inherited_object_type == other_dacl.inherited_object_type &&
    @trustee == other_dacl.trustee
  end

  def eql?(other_dacl)
    self == other_dacl
  end

  def to_s
    str = "  Trustee: #{SDDLHelper.lookup_trustee(@trustee)}\n"
    str += "  Type: #{SDDLHelper.ace_types[@ace_type]}\n"
    str += "  Permissions: \n    - #{break_flags(@permissions, SDDLHelper.permissions).join("\n    - ")}\n" if permissions != ''
    str += "  Inheritance: \n    - #{break_flags(@ace_flags, SDDLHelper.ace_flags).join("\n    - ")}\n" if ace_flags != ''
    str
  end

  private

  def break_flags(flags, lookup_dict)
    return [lookup_dict[flags]] if flags.length <= 2

    idx = 0
    flags_str = ''
    flags_list = []
    flags.each_char do |ch|
      if idx.positive? && idx.even?
        flags_list.append(lookup_dict[flags_str])
        flags_str = ''
      end
      flags_str += ch
      idx += 1
    end
    flags_list
  end
end
