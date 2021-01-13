package checkpoint

const (
	MethodGetCheckpoint string = "Get"
	MethodPutCheckpoint string = "Put"
)

type GetCheckpointRequest struct {
	Ref string `json:"ref"`
}

type GetCheckpointResponse struct {
	Path string `json:"path"`
}

type PutCheckpointRequest struct {
	Ref string `json:"ref"`
}

type PutCheckpointResponse struct {
	Error string `json:"error,omitempty"`
}
