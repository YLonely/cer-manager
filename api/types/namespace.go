package types

type NamespaceType string

const (
	NamespaceIPC NamespaceType = "ipc"
	NamespaceUTS NamespaceType = "uts"
	NamespaceMNT NamespaceType = "mnt"
)
