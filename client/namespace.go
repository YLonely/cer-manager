package client

import (
	"errors"
	"fmt"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/api/services/namespace"
	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/utils"
)

// GetNamespace get a namespace of type t of ref from cer-manager
// if more than one reference is provided, the most fitting namespace among those references will be returned
func (client *Client) GetNamespace(t types.NamespaceType, ref types.Reference, extraRefs ...types.Reference) (namespaceID int, namespacePath string, info interface{}, err error) {
	req := namespace.GetNamespaceRequest{
		T:         t,
		Ref:       ref,
		ExtraRefs: extraRefs,
	}
	var data []byte
	data, err = utils.Pack(cerm.NamespaceService, namespace.MethodGetNamespace, req)
	if err != nil {
		return
	}
	if err = utils.Send(client.c, data); err != nil {
		return
	}
	rsp := namespace.GetNamespaceResponse{}
	if err = utils.ReceiveObject(client.c, &rsp); err != nil {
		return
	}
	if rsp.Fd == -1 {
		return namespaceID, namespacePath, info, errors.New(rsp.Info.(string))
	}
	namespacePath = fmt.Sprintf("/proc/%d/fd/%d", rsp.Pid, rsp.Fd)
	namespaceID = rsp.Fd
	info = rsp.Info
	return
}

func (client *Client) PutNamespace(t types.NamespaceType, nsID int) error {
	req := namespace.PutNamespaceRequest{
		T:  t,
		ID: nsID,
	}
	data, err := utils.Pack(cerm.NamespaceService, namespace.MethodPutNamespace, req)
	if err != nil {
		return err
	}
	if err = utils.Send(client.c, data); err != nil {
		return err
	}
	rsp := namespace.PutNamespaceResponse{}
	if err = utils.ReceiveObject(client.c, &rsp); err != nil {
		return err
	}
	if rsp.Error != "" {
		return errors.New(rsp.Error)
	}
	return nil
}
