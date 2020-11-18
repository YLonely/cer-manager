package namespace

import "github.com/YLonely/cer-manager/api/types"

const (
	MethodGetNamespace string = "Get"
	MethodPutNamespace string = "Put"
)

type GetNamespaceRequest struct {
	T   types.NamespaceType
	Arg interface{}
}

type PutNamespaceRequest struct {
	T  types.NamespaceType
	ID int
}

type PutNamespaceResponse struct {
	Error string
}

type GetNamespaceResponse struct {
	NSId int
	Pid  int
	Fd   int
	Info interface{}
}
