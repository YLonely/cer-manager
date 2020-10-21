package namespace

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"path"

	"github.com/YLonely/cr-daemon/log"
	"github.com/YLonely/cr-daemon/service"
	"github.com/YLonely/cr-daemon/utils"
	"github.com/pkg/errors"
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

type PutNamespaceResponse struct {
	Error string
}

type GetNamespaceResponse struct {
	NSId int
	Pid  int
	Fd   int
	Info interface{}
}

func NewNamespaceService(root string) (service.Service, error) {
	const configName = "namespace_service.json"
	configPath := path.Join(root, configName)
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

func (svr *namespaceService) Init() error {
	log.Logger(service.NamespaceService).Info("Service initialized")
	return nil
}

func (svr *namespaceService) Handle(ctx context.Context, conn net.Conn) {
	var methodType string
	err := utils.ReceiveData(conn, &methodType)
	if err != nil {
		log.Logger(service.NamespaceService).WithError(err).Error()
		conn.Close()
		return
	}
	err = svr.handleRequest(methodType, conn)
	if err != nil {
		log.Logger(service.NamespaceService).WithError(err).Error()
		conn.Close()
		return
	}
}

func (svr *namespaceService) Stop() error {
	for t, mgr := range svr.managers {
		err := mgr.CleanUp()
		if err != nil {
			log.Logger(service.NamespaceService).WithField("namespace", t).Error(err)
		}
	}
	return nil
}

type serviceConfig struct {
	Capacity  map[NamespaceType]int      `json:"capacity"`
	ExtraArgs map[NamespaceType][]string `json:"extra_args"`
}

func (svr *namespaceService) handleGetNamespace(conn net.Conn, r GetNamespaceRequest) error {
	log.Logger(service.NamespaceService).WithField("request", r).Info()
	rsp := GetNamespaceResponse{}
	if mgr, exists := svr.managers[r.T]; !exists {
		rsp.Fd = -1
		rsp.Info = "No such namespace"
	} else {
		id, fd, info, err := mgr.Get(r.Arg)
		if err != nil {
			rsp.Fd = -1
			rsp.Info = err.Error()
		} else {
			rsp.Fd = fd
			rsp.NSId = id
			rsp.Info = info
			rsp.Pid = os.Getpid()
		}
	}
	if err := utils.SendWithSizePrefix(conn, rsp); err != nil {
		return err
	}
	log.Logger(service.NamespaceService).WithField("response", rsp).Info()
	return nil
}

func (svr *namespaceService) handlePutNamespace(conn net.Conn, r PutNamespaceRequest) error {
	log.Logger(service.NamespaceService).WithField("request", r).Info()
	rsp := PutNamespaceResponse{}
	if mgr, exists := svr.managers[r.T]; !exists {
		rsp.Error = "No such namespace"
	} else {
		err := mgr.Put(r.ID)
		if err != nil {
			rsp.Error = err.Error()
		}
	}
	if err := utils.SendWithSizePrefix(conn, rsp); err != nil {
		return err
	}
	log.Logger(service.NamespaceService).WithField("response", rsp).Info()
	return nil
}

func (svr *namespaceService) handleRequest(method string, conn net.Conn) error {
	switch method {
	case MethodGetNamespace:
		{
			var r GetNamespaceRequest
			if err := utils.ReceiveData(conn, &r); err != nil {
				return err
			}
			return svr.handleGetNamespace(conn, r)
		}
	case MethodPutNamespace:
		{
			var r PutNamespaceRequest
			if err := utils.ReceiveData(conn, &r); err != nil {
				return err
			}
			return svr.handlePutNamespace(conn, r)
		}
	default:
		return errors.New("Unknown method type")
	}
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
