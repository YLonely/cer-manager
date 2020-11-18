package namespace

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
)

type NamespaceFunction func(args ...interface{}) error

var namespaceFunctions = map[NamespaceOpType]map[NamespaceType]NamespaceFunction{}

func GetNamespaceFunction(op NamespaceOpType, t NamespaceType) NamespaceFunction {
	if functionsOfType, exists := namespaceFunctions[op]; exists {
		if f, valid := functionsOfType[t]; valid {
			return f
		}
	}
	return nil
}

func PutNamespaceFunction(op NamespaceOpType, t NamespaceType, f NamespaceFunction) {
	var functionsOfType map[NamespaceType]NamespaceFunction
	var exists bool
	if functionsOfType, exists = namespaceFunctions[op]; !exists {
		namespaceFunctions[op] = map[NamespaceType]NamespaceFunction{}
		functionsOfType = namespaceFunctions[op]
	}
	functionsOfType[t] = f
}

type NamespaceOpType string

const (
	NamespaceOpCreate  NamespaceOpType = "create"
	NamespaceOpRelease NamespaceOpType = "release"
)

const (
	NamespaceErrorFormat  string = "error:%s"
	NamespaceReturnFormat string = "ret:%s"
)

type NamespaceType string

const (
	IPC NamespaceType = "ipc"
	UTS NamespaceType = "uts"
	MNT NamespaceType = "mnt"
)

func OpenNSFile(t NamespaceType, pid int) (*os.File, error) {
	var nsFileName string
	switch t {
	case IPC:
		nsFileName = "ipc"
	case UTS:
		nsFileName = "uts"
	case MNT:
		nsFileName = "mnt"
	default:
		return nil, errors.New("invalid ns type")
	}
	nsFilePath := fmt.Sprintf("/proc/%d/ns/%s", pid, nsFileName)
	f, err := os.Open(nsFilePath)
	if err != nil {
		return nil, err
	}
	return f, nil
}
