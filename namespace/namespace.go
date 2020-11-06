package namespace

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
)

type NamespaceCreate func(args ...interface{}) error

var namespaceCreateFuncs map[NamespaceType]NamespaceCreate

func GetNamespaceCreate(t NamespaceType) NamespaceCreate {
	if f, exists := namespaceCreateFuncs[t]; exists {
		return f
	}
	return nil
}

type NamespaceDestroy func(args ...interface{}) error

var namespaceDestroyFuncs map[NamespaceType]NamespaceDestroy

func GetNamespaceDestroy(t NamespaceType) NamespaceDestroy {
	if f, exists := namespaceDestroyFuncs[t]; exists {
		return f
	}
	return nil
}

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

func OpenNSFd(t NamespaceType, pid int) (int, error) {
	var nsFileName string
	switch t {
	case IPC:
		nsFileName = "ipc"
	case UTS:
		nsFileName = "uts"
	case MNT:
		nsFileName = "mnt"
	default:
		return -1, errors.New("invalid ns type")
	}
	nsFilePath := fmt.Sprintf("/proc/%d/ns/%s", pid, nsFileName)
	f, err := os.Open(nsFilePath)
	if err != nil {
		return -1, err
	}
	return int(f.Fd()), nil
}
