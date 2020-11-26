package namespace

import (
	"fmt"
	"os"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/pkg/errors"
)

type NamespaceFunction func(args ...interface{}) error

var namespaceFunctions = map[NamespaceOpType]map[types.NamespaceType]NamespaceFunction{}

func GetNamespaceFunction(op NamespaceOpType, t types.NamespaceType) NamespaceFunction {
	if functionsOfType, exists := namespaceFunctions[op]; exists {
		if f, valid := functionsOfType[t]; valid {
			return f
		}
	}
	return nil
}

func PutNamespaceFunction(op NamespaceOpType, t types.NamespaceType, f NamespaceFunction) {
	var functionsOfType map[types.NamespaceType]NamespaceFunction
	var exists bool
	if functionsOfType, exists = namespaceFunctions[op]; !exists {
		namespaceFunctions[op] = map[types.NamespaceType]NamespaceFunction{}
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
	NamespaceErrorPrefix  string = "error:"
	NamespaceReturnPrefix string = "ret:"
)

func OpenNSFile(t types.NamespaceType, pid int) (*os.File, error) {
	var nsFileName string
	switch t {
	case types.NamespaceIPC:
		nsFileName = "ipc"
	case types.NamespaceUTS:
		nsFileName = "uts"
	case types.NamespaceMNT:
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
