package client

import (
	"net"
	"path/filepath"

	"github.com/YLonely/cer-manager/cermanager"
)

func NewCERManagerClient(config Config) (*Client, error) {
	var c net.Conn
	var err error
	if c, err = net.Dial("unix", config.SocketPath); err != nil {
		return nil, err
	}
	return &Client{
		c: c,
	}, nil
}

func NewDefaultClient() (*Client, error) {
	return NewCERManagerClient(Config{
		SocketPath: filepath.Join(cermanager.DefautlBundlePath, cermanager.DefaultSocketName),
	})
}

type Config struct {
	SocketPath string
}

type Client struct {
	c net.Conn
}

func (client *Client) Close() error {
	return client.c.Close()
}
