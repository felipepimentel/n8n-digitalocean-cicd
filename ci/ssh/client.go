package ssh

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	defaultTimeout = 30 * time.Second
)

var (
	ErrReadKey     = fmt.Errorf("unable to read private key")
	ErrParseKey    = fmt.Errorf("unable to parse private key")
	ErrDial        = fmt.Errorf("failed to dial")
	ErrSession     = fmt.Errorf("failed to create session")
	ErrExecCommand = fmt.Errorf("failed to execute command")
)

type Client struct {
	client *ssh.Client
}

func NewClient(host string, port int, user, privateKeyPath string) (*Client, error) {
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadKey, err)
	}

	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseKey, err)
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106
		Timeout:         defaultTimeout,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDial, err)
	}

	return &Client{client: client}, nil
}

func (c *Client) ExecuteCommand(command string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSession, err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), fmt.Errorf("%w: %v", ErrExecCommand, err)
	}

	return string(output), nil
}
