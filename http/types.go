package http

type updateNamespaceRequest struct {
	CheckpointName      string `json:"checkpoint_name"`
	CheckpointNamespace string `json:"checkpoint_namespace"`
	Capacity            int    `json:"capacity"`
}

type updateNamespaceResponse struct {
	Message string `json:"message"`
}
