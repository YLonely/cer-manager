package client

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/YLonely/cr-daemon/namespace"
	"github.com/YLonely/cr-daemon/utils"
)

func (client *Client) GetNamespace(t namespace.NamespaceType, arg interface{}) (int, int, interface{}, error) {
	if err := utils.Send(client.c, []byte(namespace.MethodGetNamespace)); err != nil {
		return -1, -1, nil, err
	}
	req := namespace.GetNamespaceRequest{
		T:   t,
		Arg: arg,
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return -1, -1, nil, err
	}
	if err = utils.Send(client.c, reqJSON); err != nil {
		return -1, -1, nil, err
	}
	rspJSON, err := utils.Receive(client.c)
	if err != nil {
		return -1, -1, nil, err
	}
	rsp := namespace.GetNamespaceResponse{}
	if err := json.Unmarshal(rspJSON, &rsp); err != nil {
		return -1, -1, nil, err
	}
	namespaceFdPath := fmt.Sprintf("/proc/%d/fd/%d", rsp.Pid, rsp.Fd)
	file, err := os.Open(namespaceFdPath)
	if err != nil {
		return -1, -1, nil, err
	}
	return rsp.NSId, int(file.Fd()), rsp.Info, nil
}

func (client *Client) PutNamespace(t namespace.NamespaceType, nsID int) error {
	req := namespace.PutNamespaceRequest{
		T:  t,
		ID: nsID,
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err = utils.Send(client.c, reqJSON); err != nil {
		return err
	}
	return nil
}
