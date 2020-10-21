package client

import (
	"context"
	"net"
)

const defaultSocketPath = "/var/lib/crdaemon/daemon.socket"

func NewDaemonClient(config Config) (*Client, error) {
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
	return NewDaemonClient(Config{
		SocketPath: defaultSocketPath,
	})
}

type Config struct {
	SocketPath string
}

type Client struct {
	c net.Conn
}

func (client *Client) Close(context.Context) error {
	return client.c.Close()
}
