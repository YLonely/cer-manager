package namespace

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
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
	svr := &namespaceService{
		managers: map[NamespaceType]namespaceManager{},
	}
	var err error
	if svr.managers[UTS], err = newUTSNamespaceManager(config.Capacity[UTS]); err != nil {
		return nil, err
	}
	if svr.managers[IPC], err = newIPCNamespaceManager(config.Capacity[IPC]); err != nil {
		return nil, err
	}
	if svr.managers[MNT], err = newMountNamespaceManager(config.Capacity[MNT], config.ExtraArgs[MNT]); err != nil {
		return nil, err
	}
	return svr, nil
}

type namespaceService struct {
	managers map[NamespaceType]namespaceManager
}

var _ service.Service = &namespaceService{}

func (svr *namespaceService) Init(context.Context) error {
	return nil
}

func (svr *namespaceService) Handle(ctx context.Context, conn net.Conn) error {

}

func (svr *namespaceService) Stop(context.Context) error {

}

type serviceConfig struct {
	Capacity  map[NamespaceType]int      `json:"capacity"`
	ExtraArgs map[NamespaceType][]string `json:"extra_args"`
}

func handleMethodType(conn net.Conn) (string, error) {

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
		ExtraArgs: map[NamespaceType][]string{},
	}
}
