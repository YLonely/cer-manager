package namespace

import "context"

type NamespaceType int

const (
	IPC NamespaceType = iota
	UTS
	MNT
)

type PoolClient interface {
	GetNamespace(context.Context, NamespaceType) (int, int, error)
	PutNamespace(context.Context, NamespaceType, int) error
	Close(context.Context) error
}

type PoolClientConfig struct {
	socketPath string
}
