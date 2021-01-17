package checkpoint

import "github.com/YLonely/cer-manager/api/types"

const (
	MethodGetCheckpoint string = "Get"
	MethodPutCheckpoint string = "Put"
)

type GetCheckpointRequest struct {
	Ref types.Reference `json:"ref"`
}

type GetCheckpointResponse struct {
	Path string `json:"path"`
}

type PutCheckpointRequest struct {
	Ref types.Reference `json:"ref"`
}

type PutCheckpointResponse struct {
	Error string `json:"error,omitempty"`
}
