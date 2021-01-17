package client

import (
	cermanager "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/api/services/checkpoint"
	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/utils"
)

// GetCheckpoint returns the dir path in which checkpoint files of the container with ref located
func (client *Client) GetCheckpoint(ref types.Reference) (string, error) {
	req := checkpoint.GetCheckpointRequest{
		Ref: ref,
	}
	data, err := utils.Pack(cermanager.CheckpointService, checkpoint.MethodGetCheckpoint, req)
	if err != nil {
		return "", err
	}
	if err = utils.Send(client.c, data); err != nil {
		return "", err
	}
	rsp := checkpoint.GetCheckpointResponse{}
	if err = utils.ReceiveObject(client.c, &rsp); err != nil {
		return "", err
	}
	return rsp.Path, nil
}

// PutCheckpoint release the checkpoint
func (client *Client) PutCheckpoint(ref types.Reference) error {
	req := checkpoint.PutCheckpointRequest{
		Ref: ref,
	}
	data, err := utils.Pack(cermanager.CheckpointService, checkpoint.MethodPutCheckpoint, req)
	if err != nil {
		return err
	}
	if err = utils.Send(client.c, data); err != nil {
		return err
	}
	rsp := checkpoint.PutCheckpointResponse{}
	if err = utils.ReceiveObject(client.c, &rsp); err != nil {
		return err
	}
	return nil
}
