package ssh

import (
	"fmt"
	"io/ioutil"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	config *ssh.ClientConfig
	addr   string
}

func NewClient(host string, port int, user string, keyPath string) (*Client, error) {
	key, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read private key: %v", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return &Client{
		config: config,
		addr:   fmt.Sprintf("%s:%d", host, port),
	}, nil
}

func (c *Client) ExecuteCommand(cmd string) (string, error) {
	client, err := ssh.Dial("tcp", c.addr, c.config)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(output), fmt.Errorf("failed to execute command: %v", err)
	}

	return string(output), nil
} 