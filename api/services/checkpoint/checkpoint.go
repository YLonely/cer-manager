package checkpoint

const (
	MethodGetCheckpoint string = "Get"
)

type GetCheckpointRequest struct {
	Ref string `json:"ref"`
}

type GetCheckpointResponse struct {
	Path string `json:"path"`
}
