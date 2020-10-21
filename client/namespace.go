package client

import (
	"errors"
	"fmt"
	"os"

	"github.com/YLonely/cr-daemon/namespace"
	"github.com/YLonely/cr-daemon/service"
	"github.com/YLonely/cr-daemon/utils"
)

func (client *Client) GetNamespace(t namespace.NamespaceType, arg interface{}) (int, int, interface{}, error) {
	req := namespace.GetNamespaceRequest{
		T:   t,
		Arg: arg,
	}
	data, err := utils.Pack(service.NamespaceService, namespace.MethodGetNamespace, req)
	if err != nil {
		return -1, -1, nil, err
	}
	if err = utils.Send(client.c, data); err != nil {
		return -1, -1, nil, err
	}
	rsp := namespace.GetNamespaceResponse{}
	if err = utils.ReceiveData(client.c, &rsp); err != nil {
		return -1, -1, nil, err
	}
	if rsp.Fd == -1 {
		return -1, -1, nil, errors.New(rsp.Info.(string))
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
	data, err := utils.Pack(service.NamespaceService, namespace.MethodPutNamespace, req)
	if err != nil {
		return err
	}
	if err = utils.Send(client.c, data); err != nil {
		return err
	}
	rsp := namespace.PutNamespaceResponse{}
	if err = utils.ReceiveData(client.c, &rsp); err != nil {
		return err
	}
	if rsp.Error != "" {
		return errors.New(rsp.Error)
	}
	return nil
}
