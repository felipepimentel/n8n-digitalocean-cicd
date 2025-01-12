package ssh

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const (
	defaultTimeout = 10 * time.Second
)

var (
	ErrSSHAuthSockNotSet = errors.New("SSH_AUTH_SOCK not set")
)

type Client struct {
	client *ssh.Client
}

func NewClient(host string, port int, user, keyPath string) (*Client, error) {
	// Try to connect to SSH agent
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, ErrSSHAuthSockNotSet
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
	}

	agentClient := agent.NewClient(conn)

	// Create SSH client config
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			// Use SSH agent for authentication
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		// #nosec G106 -- Using InsecureIgnoreHostKey is acceptable for this use case
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         defaultTimeout,
	}

	// Connect to remote host
	addr := fmt.Sprintf("%s:%d", host, port)

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	return &Client{
		client: client,
	}, nil
}

func (c *Client) ExecuteCommand(command string) (string, error) {
	// Create session
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Run command and capture output
	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), fmt.Errorf("failed to run command: %w", err)
	}

	return string(output), nil
}

func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}

	return nil
}
