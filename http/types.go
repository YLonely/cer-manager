package http

import "github.com/pkg/errors"

const (
	periodInitial        = "initial"
	periodImageImport    = "import image"
	periodImageUnpack    = "unpack image"
	periodContainerStart = "start container"
	periodCheckpoint     = "checkpoint"
)

var (
	errInitialFailed        = errors.New(periodInitial)
	errImageImportFailed    = errors.New(periodImageImport)
	errImageUnpackFailed    = errors.New(periodImageUnpack)
	errContainerStartFailed = errors.New(periodContainerStart)
	errCheckpointFailed     = errors.New(periodCheckpoint)
)

type updateNamespaceRequest struct {
	CheckpointName      string `json:"checkpoint_name"`
	CheckpointNamespace string `json:"checkpoint_namespace"`
	Capacity            int    `json:"capacity"`
}

type updateNamespaceResponse struct {
	Message string `json:"message"`
}

type makeCheckpointRequest struct {
	TarFileName    string `json:"tar_file_name"`
	ImageName      string `json:"image_name"`
	CheckpointName string `json:"checkpoint_name"`
	Namespace      string `json:"namespace"`
}

type makeCheckpointResponse struct {
	Error string `json:"error,omitempty"`
}
