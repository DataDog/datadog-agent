package remote

import (
	"github.com/DataDog/test-infra-definitions/common"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type connectionArgs struct {
	host pulumi.StringInput
	user string

	// ==== Optional ====
	privateKeyPath     string
	privateKeyPassword string
	sshAgentPath       string
	port               int
}

type ConnectionOption = func(*connectionArgs) error

func buildConnectionArgs(host pulumi.StringInput, user string, options ...ConnectionOption) (*connectionArgs, error) {
	args := &connectionArgs{
		host: host,
		user: user,
		port: 22,
	}
	return common.ApplyOption(args, options)
}

// WithPrivateKeyPath [optional] sets the path to the private key to use for the connection
func WithPrivateKeyPath(path string) ConnectionOption {
	return func(args *connectionArgs) error {
		args.privateKeyPath = path
		return nil
	}
}

// WithPrivateKeyPassword [optional] sets the password to use in case the private key is encrypted
func WithPrivateKeyPassword(password string) ConnectionOption {
	return func(args *connectionArgs) error {
		args.privateKeyPassword = password
		return nil
	}
}

// WithSSHAgentPath [optional] sets the path to the SSH Agent socket. Default to environment variable SSH_AUTH_SOCK if present.
func WithSSHAgentPath(path string) ConnectionOption {
	return func(args *connectionArgs) error {
		args.sshAgentPath = path
		return nil
	}
}

// WithPort [optional] sets the port to use for the connection. Default to 22.
func WithPort(port int) ConnectionOption {
	return func(args *connectionArgs) error {
		args.port = port
		return nil
	}
}
