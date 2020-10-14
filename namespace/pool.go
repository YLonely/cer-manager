package namespace

type requestType string

const (
	typeGetNamespace requestType = "GetNamespace"
	typePutNamespace requestType = "PutNamespace"
)

type getNamespaceRequest struct {
	T NamespaceType
}

type putNamespaceRequest struct {
	T  NamespaceType
	ID int
}

type getNamespaceResponse struct {
	NSId int
	Pid  int
	Fd   int
}
