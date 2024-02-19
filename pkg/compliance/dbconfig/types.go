// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dbconfig

// DBResource holds a database configuration data and the resource type
// associated with it.
type DBResource struct {
	Type        string   `json:"type"`
	ContainerID string   `json:"container_id,omitempty"`
	Config      DBConfig `json:"config"`
}

// DBConfig represents a database application configuration metadata that we
// were able to scan.
type DBConfig struct {
	ProcessName     string      `json:"process_name,omitempty"`
	ProcessUser     string      `json:"process_user,omitempty"`
	ConfigFilePath  string      `json:"config_file_path"`
	ConfigFileUser  string      `json:"config_file_user"`
	ConfigFileGroup string      `json:"config_file_group"`
	ConfigFileMode  uint32      `json:"config_file_mode"`
	ConfigData      interface{} `json:"config_data"`
}

type mongoDBConfig struct {
	// Ignored fields:
	//     net.compression.compressors string
	//     net.ssl.clusterPassword string
	//     net.ssl.PEMKeyPassword string
	//     net.tls.certificateKeyFilePassword string
	//     net.tls.clusterPassword string

	//     operationProfiling.filter string
	//     operationProfiling.mode string
	//     operationProfiling.slowOpSampleRate float64
	//     operationProfiling.slowOpThresholdMs int

	//     processManagement.fork boolean
	//     processManagement.pidFilePath string
	//     processManagement.timeZoneInfo string
	//     processManagement.windowsService.description string
	//     processManagement.windowsService.displayName string
	//     processManagement.windowsService.serviceName string
	//     processManagement.windowsService.servicePassword string
	//     processManagement.windowsService.serviceUser string

	//     security.kmip.clientCertificatePassword string
	//     security.ldap.bind.queryPassword string

	//     storage.inMemory.engineConfig.inMemorySizeGB float64
	//     storage.journal.commitIntervalMs int
	//     storage.wiredTiger.collectionConfig.blockCompressor string
	//     storage.wiredTiger.engineConfig.cacheSizeGB float64
	//     storage.wiredTiger.engineConfig.directoryForIndexes bool
	//     storage.wiredTiger.engineConfig.journalCompressor string
	//     storage.wiredTiger.engineConfig.maxCacheOverflowFileSizeGB float64
	//     storage.wiredTiger.engineConfig.zstdCompressionLevel int
	//     storage.wiredTiger.indexConfig.prefixCompression bool

	SystemLog *struct {
		Verbosity          *int    `yaml:"verbosity,omitempty" json:"verbosity,omitempty"`
		Quiet              *bool   `yaml:"quiet,omitempty" json:"quiet,omitempty"`
		TraceAllExceptions *bool   `yaml:"traceAllExceptions,omitempty" json:"traceAllExceptions,omitempty"`
		SyslogFacility     *string `yaml:"syslogFacility,omitempty" json:"syslogFacility,omitempty"`
		Path               *string `yaml:"path,omitempty" json:"path,omitempty"`
		LogAppend          *bool   `yaml:"logAppend,omitempty" json:"logAppend,omitempty"`
		LogRotate          *string `yaml:"logRotate,omitempty" json:"logRotate,omitempty"`
		Destination        *string `yaml:"destination,omitempty" json:"destination,omitempty"`
		TimeStampFormat    *string `yaml:"timeStampFormat,omitempty" json:"timeStampFormat,omitempty"`
	} `yaml:"systemLog,omitempty" json:"systemLog,omitempty"`

	Net *struct {
		Port                   *int    `yaml:"port,omitempty" json:"port,omitempty"`
		BindIP                 *string `yaml:"bindIp,omitempty" json:"bindIp,omitempty"`
		BindIPAll              *bool   `yaml:"bindIpAll,omitempty" json:"bindIpAll,omitempty"`
		MaxIncomingConnections *int    `yaml:"maxIncomingConnections,omitempty" json:"maxIncomingConnections,omitempty"`
		WireObjectCheck        *bool   `yaml:"wireObjectCheck,omitempty" json:"wireObjectCheck,omitempty"`
		Ipv6                   *bool   `yaml:"ipv6,omitempty" json:"ipv6,omitempty"`

		UnixDomainSocket *struct {
			Enabled         *bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
			FilePermissions *int    `yaml:"filePermissions,omitempty" json:"filePermissions,omitempty"`
			PathPrefix      *string `yaml:"pathPrefix,omitempty" json:"pathPrefix,omitempty"`
		} `yaml:"unixDomainSocket,omitempty" json:"unixDomainSocket,omitempty"`

		TLS *struct {
			AllowConnectionsWithoutCertificates *bool   `yaml:"allowConnectionsWithoutCertificates,omitempty" json:"allowConnectionsWithoutCertificates,omitempty"`
			AllowInvalidCertificates            *bool   `yaml:"allowInvalidCertificates,omitempty" json:"allowInvalidCertificates,omitempty"`
			AllowInvalidHostnames               *bool   `yaml:"allowInvalidHostnames,omitempty" json:"allowInvalidHostnames,omitempty"`
			CAFile                              *string `yaml:"CAFile,omitempty" json:"CAFile,omitempty"`
			CertificateKeyFile                  *string `yaml:"certificateKeyFile,omitempty" json:"certificateKeyFile,omitempty"`
			CertificateSelector                 *string `yaml:"certificateSelector,omitempty" json:"certificateSelector,omitempty"`
			ClusterCAFile                       *string `yaml:"clusterCAFile,omitempty" json:"clusterCAFile,omitempty"`
			ClusterCertificateSelector          *string `yaml:"clusterCertificateSelector,omitempty" json:"clusterCertificateSelector,omitempty"`
			ClusterFile                         *string `yaml:"clusterFile,omitempty" json:"clusterFile,omitempty"`
			CRLFile                             *string `yaml:"CRLFile,omitempty" json:"CRLFile,omitempty"`
			DisabledProtocols                   *string `yaml:"disabledProtocols,omitempty" json:"disabledProtocols,omitempty"`
			FIPSMode                            *bool   `yaml:"FIPSMode,omitempty" json:"FIPSMode,omitempty"`
			LogVersions                         *string `yaml:"logVersions,omitempty" json:"logVersions,omitempty"`
			Mode                                *string `yaml:"mode,omitempty" json:"mode,omitempty"`
			ClusterAuthX509                     *struct {
				Attributes     *string `yaml:"attributes,omitempty" json:"attributes,omitempty"`
				ExtensionValue *string `yaml:"extensionValue,omitempty" json:"extensionValue,omitempty"`
			} `yaml:"clusterAuthX509,omitempty" json:"clusterAuthX509,omitempty"`
		} `yaml:"tls,omitempty" json:"tls,omitempty"`

		SSL struct {
			AllowConnectionsWithoutCertificates *bool   `yaml:"allowConnectionsWithoutCertificates,omitempty" json:"allowConnectionsWithoutCertificates,omitempty"`
			AllowInvalidCertificates            *bool   `yaml:"allowInvalidCertificates,omitempty" json:"allowInvalidCertificates,omitempty"`
			AllowInvalidHostnames               *bool   `yaml:"allowInvalidHostnames,omitempty" json:"allowInvalidHostnames,omitempty"`
			CAFile                              *string `yaml:"CAFile,omitempty" json:"CAFile,omitempty"`
			CertificateSelector                 *string `yaml:"certificateSelector,omitempty" json:"certificateSelector,omitempty"`
			ClusterCAFile                       *string `yaml:"clusterCAFile,omitempty" json:"clusterCAFile,omitempty"`
			ClusterCertificateSelector          *string `yaml:"clusterCertificateSelector,omitempty" json:"clusterCertificateSelector,omitempty"`
			ClusterFile                         *string `yaml:"clusterFile,omitempty" json:"clusterFile,omitempty"`
			CRLFile                             *string `yaml:"CRLFile,omitempty" json:"CRLFile,omitempty"`
			DisabledProtocols                   *string `yaml:"disabledProtocols,omitempty" json:"disabledProtocols,omitempty"`
			FIPSMode                            *bool   `yaml:"FIPSMode,omitempty" json:"FIPSMode,omitempty"`
			Mode                                *string `yaml:"mode,omitempty" json:"mode,omitempty"`
			PEMKeyFile                          *string `yaml:"PEMKeyFile,omitempty" json:"PEMKeyFile,omitempty"`
			SslOnNormalPorts                    *bool   `yaml:"sslOnNormalPorts,omitempty" json:"sslOnNormalPorts,omitempty"`
		} `yaml:"ssl,omitempty" json:"ssl,omitempty"`
	} `yaml:"net,omitempty" json:"net,omitempty"`

	Security *struct {
		Authorization            *string   `yaml:"authorization,omitempty" json:"authorization,omitempty"`
		ClusterAuthMode          *string   `yaml:"clusterAuthMode,omitempty" json:"clusterAuthMode,omitempty"`
		ClusterIPSourceAllowlist *[]string `yaml:"clusterIpSourceAllowlist,omitempty" json:"clusterIpSourceAllowlist,omitempty"`
		ClusterIPSourceWhitelist *[]string `yaml:"clusterIpSourceWhitelist,omitempty" json:"clusterIpSourceWhitelist,omitempty"`
		EnableEncryption         *bool     `yaml:"enableEncryption,omitempty" json:"enableEncryption,omitempty"`
		EncryptionCipherMode     *string   `yaml:"encryptionCipherMode,omitempty" json:"encryptionCipherMode,omitempty"`
		EncryptionKeyFile        *string   `yaml:"encryptionKeyFile,omitempty" json:"encryptionKeyFile,omitempty"`
		JavascriptEnabled        *bool     `yaml:"javascriptEnabled,omitempty" json:"javascriptEnabled,omitempty"`
		KeyFile                  *string   `yaml:"keyFile,omitempty" json:"keyFile,omitempty"`
		RedactClientLogData      *bool     `yaml:"redactClientLogData,omitempty" json:"redactClientLogData,omitempty"`
		TransitionToAuth         *bool     `yaml:"transitionToAuth,omitempty" json:"transitionToAuth,omitempty"`

		KMIP *struct {
			KeyIdentifier             *string `yaml:"keyIdentifier,omitempty" json:"keyIdentifier,omitempty"`
			RotateMasterKey           *bool   `yaml:"rotateMasterKey,omitempty" json:"rotateMasterKey,omitempty"`
			ServerName                *string `yaml:"serverName,omitempty" json:"serverName,omitempty"`
			Port                      *string `yaml:"port,omitempty" json:"port,omitempty"`
			ClientCertificateFile     *string `yaml:"clientCertificateFile,omitempty" json:"clientCertificateFile,omitempty"`
			ClientCertificateSelector *string `yaml:"clientCertificateSelector,omitempty" json:"clientCertificateSelector,omitempty"`
			ServerCAFile              *string `yaml:"serverCAFile,omitempty" json:"serverCAFile,omitempty"`
			ConnectRetries            *int    `yaml:"connectRetries,omitempty" json:"connectRetries,omitempty"`
			ConnectTimeoutMS          *int    `yaml:"connectTimeoutMS,omitempty" json:"connectTimeoutMS,omitempty"`
			ActivateKeys              *bool   `yaml:"activateKeys,omitempty" json:"activateKeys,omitempty"`
			KeyStatePollingSeconds    *int    `yaml:"keyStatePollingSeconds,omitempty" json:"keyStatePollingSeconds,omitempty"`
			UseLegacyProtocol         *bool   `yaml:"useLegacyProtocol,omitempty" json:"useLegacyProtocol,omitempty"`
		} `yaml:"kmip,omitempty" json:"kmip,omitempty"`

		SASL *struct {
			HostName            *string `yaml:"hostName,omitempty" json:"hostName,omitempty"`
			ServiceName         *string `yaml:"serviceName,omitempty" json:"serviceName,omitempty"`
			SaslauthdSocketPath *string `yaml:"saslauthdSocketPath,omitempty" json:"saslauthdSocketPath,omitempty"`
		} `yaml:"sasl,omitempty" json:"sasl,omitempty"`

		LDAP *struct {
			Servers                  *string `yaml:"servers,omitempty" json:"servers,omitempty"`
			TransportSecurity        *string `yaml:"transportSecurity,omitempty" json:"transportSecurity,omitempty"`
			TimeoutMS                *int    `yaml:"timeoutMS,omitempty" json:"timeoutMS,omitempty"`
			RetryCount               *int    `yaml:"retryCount,omitempty" json:"retryCount,omitempty"`
			UserToDNMapping          *string `yaml:"userToDNMapping,omitempty" json:"userToDNMapping,omitempty"`
			ValidateLDAPServerConfig *bool   `yaml:"validateLDAPServerConfig,omitempty" json:"validateLDAPServerConfig,omitempty"`
			Authz                    *struct {
				QueryTemplate *string `yaml:"queryTemplate,omitempty" json:"queryTemplate,omitempty"`
			} `yaml:"authz,omitempty" json:"authz,omitempty"`
			Bind *struct {
				Method         *string `yaml:"method,omitempty" json:"method,omitempty"`
				QueryUser      *string `yaml:"queryUser,omitempty" json:"queryUser,omitempty"`
				SaslMechanisms *string `yaml:"saslMechanisms,omitempty" json:"saslMechanisms,omitempty"`
				UseOSDefaults  *bool   `yaml:"useOSDefaults,omitempty" json:"useOSDefaults,omitempty"`
			} `yaml:"bind,omitempty" json:"bind,omitempty"`
		} `yaml:"ldap,omitempty" json:"ldap,omitempty"`
	} `yaml:"security,omitempty" json:"security,omitempty"`

	Sharding *struct {
		ArchiveMovedChunks *bool   `yaml:"archiveMovedChunks,omitempty" json:"archiveMovedChunks,omitempty"`
		ClusterRole        *string `yaml:"clusterRole,omitempty" json:"clusterRole,omitempty"`
		ConfigDB           *string `yaml:"configDB,omitempty" json:"configDB,omitempty"`
	} `yaml:"sharding,omitempty" json:"sharding,omitempty"`

	Replication *struct {
		EnableMajorityReadConcern *bool   `yaml:"enableMajorityReadConcern,omitempty" json:"enableMajorityReadConcern,omitempty"`
		LocalPingThresholdMs      *int    `yaml:"localPingThresholdMs,omitempty" json:"localPingThresholdMs,omitempty"`
		OplogSizeMB               *int    `yaml:"oplogSizeMB,omitempty" json:"oplogSizeMB,omitempty"`
		ReplSetName               *string `yaml:"replSetName,omitempty" json:"replSetName,omitempty"`
	} `yaml:"replication,omitempty" json:"replication,omitempty"`

	AuditLog *struct {
		AuditEncryptionKeyIdentifier *string `yaml:"auditEncryptionKeyIdentifier,omitempty" json:"auditEncryptionKeyIdentifier,omitempty"`
		CompressionMode              *string `yaml:"compressionMode,omitempty" json:"compressionMode,omitempty"`
		Destination                  *string `yaml:"destination,omitempty" json:"destination,omitempty"`
		Filter                       *string `yaml:"filter,omitempty" json:"filter,omitempty"`
		Format                       *string `yaml:"format,omitempty" json:"format,omitempty"`
		LocalAuditKeyFile            *string `yaml:"localAuditKeyFile,omitempty" json:"localAuditKeyFile,omitempty"`
		Path                         *string `yaml:"path,omitempty" json:"path,omitempty"`
		RuntimeConfiguration         *bool   `yaml:"runtimeConfiguration,omitempty" json:"runtimeConfiguration,omitempty"`
	} `yaml:"auditLog,omitempty" json:"auditLog,omitempty"`

	Storage *struct {
		DbPath                 *string  `yaml:"dbPath,omitempty" json:"dbPath,omitempty"`
		DirectoryPerDB         *bool    `yaml:"directoryPerDB,omitempty" json:"directoryPerDB,omitempty"`
		SyncPeriodSecs         *int     `yaml:"syncPeriodSecs,omitempty" json:"syncPeriodSecs,omitempty"`
		Engine                 *string  `yaml:"engine,omitempty" json:"engine,omitempty"`
		OplogMinRetentionHours *float64 `yaml:"oplogMinRetentionHours,omitempty" json:"oplogMinRetentionHours,omitempty"`
	} `yaml:"storage,omitempty" json:"storage,omitempty"`

	SetParameter *struct {
		EnableLocalhostAuthBypass *bool   `yaml:"enableLocalhostAuthBypass,omitempty" json:"enableLocalhostAuthBypass,omitempty"`
		AuthenticationMechanisms  *string `yaml:"authenticationMechanisms,omitempty" json:"authenticationMechanisms,omitempty"`
	} `yaml:"setParameter,omitempty" json:"setParameter,omitempty"`
}

type cassandraDBConfig struct {
	Authenticator           string `yaml:"authenticator" json:"authenticator"`
	LogbackFilePath         string `yaml:"logback_file_path" json:"logback_file_path"`
	LogbackFileContent      string `yaml:"logback_file_content" json:"logback_file_content"`
	Authorizer              string `yaml:"authorizer" json:"authorizer"`
	ListenAddress           string `yaml:"listen_address" json:"listen_address"`
	ClientEncryptionOptions struct {
		Enabled  bool `yaml:"enabled" json:"enabled"`
		Optional bool `yaml:"optional" json:"optional"`
	} `yaml:"client_encryption_options" json:"client_encryption_options"`
	ServerEncryptionOptions struct {
		InternodeEncryption string `yaml:"internode_encryption" json:"internode_encryption"`
	} `yaml:"server_encryption_options" json:"server_encryption_options"`
}
