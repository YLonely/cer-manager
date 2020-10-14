package namespace

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
)

func NewPoolClient(config PoolClientConfig) (PoolClient, error) {
	var c net.Conn
	var err error
	if c, err = net.Dial("unix", config.socketPath); err != nil {
		return nil, err
	}
	return &poolClient{
		c: c,
	}, nil
}

type poolClient struct {
	c net.Conn
}

var _ PoolClient = &poolClient{}

func (client *poolClient) GetNamespace(ctx context.Context, t NamespaceType) (int, int, error) {
	if _, err := client.c.Write([]byte(typeGetNamespace)); err != nil {
		return -1, -1, err
	}
	req := getNamespaceRequest{
		T: t,
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return -1, -1, err
	}
	if _, err = client.c.Write(reqJSON); err != nil {
		return -1, -1, err
	}
	rspJSON, err := ioutil.ReadAll(client.c)
	if err != nil {
		return -1, -1, err
	}
	rsp := getNamespaceResponse{}
	if err := json.Unmarshal(rspJSON, &rsp); err != nil {
		return -1, -1, err
	}
	namespaceFdPath := fmt.Sprintf("/proc/%d/fd/%d", rsp.Pid, rsp.Fd)
	file, err := os.Open(namespaceFdPath)
	if err != nil {
		return -1, -1, err
	}
	return rsp.NSId, int(file.Fd()), nil
}

func (client *poolClient) PutNamespace(ctx context.Context, t NamespaceType, nsID int) error {

}

func (client *poolClient) Close(context.Context) error {
	return client.c.Close()
}
