package client

import (
	"context"
	"net"
)

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

type Config struct {
	SocketPath string
}

type Client struct {
	c net.Conn
}

func (client *Client) Close(context.Context) error {
	return client.c.Close()
}
