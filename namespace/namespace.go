package namespace

import (
	"fmt"
	"os"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/pkg/errors"
)

type NamespaceFunction func(map[string]interface{}) (string, error)

var namespaceFunctions = map[NamespaceFunctionKey]map[types.NamespaceType]NamespaceFunction{}

func GetNamespaceFunction(key NamespaceFunctionKey, t types.NamespaceType) NamespaceFunction {
	if functionsOfType, exists := namespaceFunctions[key]; exists {
		if f, valid := functionsOfType[t]; valid {
			return f
		}
	}
	return nil
}

func PutNamespaceFunction(key NamespaceFunctionKey, t types.NamespaceType, f NamespaceFunction) {
	var functionsOfType map[types.NamespaceType]NamespaceFunction
	var exists bool
	if functionsOfType, exists = namespaceFunctions[key]; !exists {
		namespaceFunctions[key] = map[types.NamespaceType]NamespaceFunction{}
		functionsOfType = namespaceFunctions[key]
	}
	functionsOfType[t] = f
}

type NamespaceFunctionKey string

const (
	namespaceFunctionKeyCreate  NamespaceFunctionKey = "create"
	namespaceFunctionKeyRelease NamespaceFunctionKey = "release"
	namespaceFunctionKeyReset   NamespaceFunctionKey = "reset"
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
