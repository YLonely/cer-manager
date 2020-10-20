package namespace

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"

	"github.com/YLonely/cr-daemon/service"
)

type NamespaceType string

const (
	IPC NamespaceType = "ipc"
	UTS NamespaceType = "uts"
	MNT NamespaceType = "mnt"
)

const (
	MethodGetNamespace string = "Get"
	MethodPutNamespace string = "Put"
)

type GetNamespaceRequest struct {
	T   NamespaceType
	Arg interface{}
}

type PutNamespaceRequest struct {
	T  NamespaceType
	ID int
}

type GetNamespaceResponse struct {
	NSId int
	Pid  int
	Fd   int
	Info interface{}
}

func NewNamespaceService(root string) (service.Service, error) {
	const configName = "namespace_service.json"
	configPath := root + "/" + configName
	config := defaultConfig()
	if _, err := os.Stat(configPath); err == nil {
		content, err := ioutil.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		c := serviceConfig{}
		if err = json.Unmarshal(content, &c); err != nil {
			return nil, err
		}
		if err = mergeConfig(&config, &c); err != nil {
			return nil, err
		}
	} else if err != os.ErrNotExist {
		return nil, err
	}
	ns := []NamespaceType{IPC, UTS, MNT}

}

type namespaceService struct {
	managers map[NamespaceType]namespaceManager
}

var _ service.Service = &namespaceService{}

type serviceConfig struct {
	Capacity  map[NamespaceType]int    `json:"capacity"`
	ExtraArgs map[NamespaceType]string `json:"extra_args"`
}

func mergeConfig(to, from *serviceConfig) error {
	nsTypes := []NamespaceType{IPC, UTS, MNT}
	for _, t := range nsTypes {
		if v, exists := from.Capacity[t]; exists {
			if v < 0 {
				return errors.New("negative namespace capacity")
			}
			to.Capacity[t] = v
		}
		if v, exists := from.ExtraArgs[t]; exists {
			to.ExtraArgs[t] = v
		}
	}
	return nil
}

func defaultConfig() serviceConfig {
	return serviceConfig{
		Capacity: map[NamespaceType]int{
			IPC: 5,
			UTS: 5,
			MNT: 5,
		},
		ExtraArgs: map[NamespaceType]string{},
	}
}
