package client

import (
	"errors"
	"fmt"

	ns "github.com/YLonely/cer-manager/namespace"
	"github.com/YLonely/cer-manager/services"
	"github.com/YLonely/cer-manager/services/namespace"
	"github.com/YLonely/cer-manager/utils"
)

func (client *Client) GetNamespace(t ns.NamespaceType, arg interface{}) (namespaceID int, namespacePath string, info interface{}, err error) {
	req := namespace.GetNamespaceRequest{
		T:   t,
		Arg: arg,
	}
	var data []byte
	data, err = utils.Pack(services.NamespaceService, namespace.MethodGetNamespace, req)
	if err != nil {
		return
	}
	if err = utils.Send(client.c, data); err != nil {
		return
	}
	rsp := namespace.GetNamespaceResponse{}
	if err = utils.ReceiveData(client.c, &rsp); err != nil {
		return
	}
	if rsp.Fd == -1 {
		return namespaceID, namespacePath, info, errors.New(rsp.Info.(string))
	}
	namespacePath = fmt.Sprintf("/proc/%d/fd/%d", rsp.Pid, rsp.Fd)
	namespaceID = rsp.NSId
	info = rsp.Info
	return
}

func (client *Client) PutNamespace(t ns.NamespaceType, nsID int) error {
	req := namespace.PutNamespaceRequest{
		T:  t,
		ID: nsID,
	}
	data, err := utils.Pack(services.NamespaceService, namespace.MethodPutNamespace, req)
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
