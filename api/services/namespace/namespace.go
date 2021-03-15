package namespace

import "github.com/YLonely/cer-manager/api/types"

const (
	MethodGetNamespace    string = "Get"
	MethodPutNamespace    string = "Put"
	MethodUpdateNamespace string = "Update"
)

type GetNamespaceRequest struct {
	T         types.NamespaceType `json:"namespace_type"`
	Ref       types.Reference     `json:"ref"`
	ExtraRefs []types.Reference   `json:"extra_refs,omitempty"`
}

type PutNamespaceRequest struct {
	T  types.NamespaceType `json:"namespace_type"`
	ID int                 `json:"namespace_id"`
}

type PutNamespaceResponse struct {
	Error string `json:"error,omitempty"`
}

type GetNamespaceResponse struct {
	Pid  int         `json:"pid"`
	Fd   int         `json:"namespace_fd"`
	Info interface{} `json:"info,omitempty"`
}

type UpdateNamespaceRequest struct {
	Ref      types.Reference `json:"ref"`
	Capacity int             `json:"capacity"`
}

type UpdateNamespaceResponse struct {
	Error string `json:"error,omitempty"`
}
