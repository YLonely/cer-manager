package namespace

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"path"

	ns "github.com/YLonely/cer-manager/namespace"

	nsapi "github.com/YLonely/cer-manager/api/services/namespace"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/rootfs/containerd"
	"github.com/YLonely/cer-manager/services"
	"github.com/YLonely/cer-manager/utils"
	"github.com/pkg/errors"
)

func New(root string) (services.Service, error) {
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
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return &namespaceService{
		config:   config,
		managers: map[types.NamespaceType]ns.Manager{},
		root:     root,
	}, nil
}

type namespaceService struct {
	config   serviceConfig
	managers map[types.NamespaceType]ns.Manager
	root     string
}

var _ services.Service = &namespaceService{}

func (svr *namespaceService) Init() error {
	var err error
	if svr.managers[types.NamespaceUTS], err = ns.NewUTSManager(svr.root, svr.config.Capacity[types.NamespaceUTS]); err != nil {
		return err
	}
	if svr.managers[types.NamespaceIPC], err = ns.NewIPCManager(svr.root, svr.config.Capacity[types.NamespaceIPC]); err != nil {
		return err
	}
	p, err := containerd.NewProvider()
	if err != nil {
		return err
	}
	if svr.managers[types.NamespaceMNT], err = ns.NewMountManager(
		svr.root,
		svr.config.Capacity[types.NamespaceMNT],
		svr.config.ExtraArgs[types.NamespaceMNT],
		p,
	); err != nil {
		return err
	}
	log.Logger(services.NamespaceService, "Init").Info("Service initialized")
	return nil
}

func (svr *namespaceService) Handle(ctx context.Context, conn net.Conn) {
	var methodType string
	err := utils.ReceiveData(conn, &methodType)
	if err != nil {
		log.Logger(services.NamespaceService, "").WithError(err).Error()
		conn.Close()
		return
	}
	err = svr.handleRequest(methodType, conn)
	if err != nil {
		log.Logger(services.NamespaceService, "").WithError(err).Error()
		conn.Close()
		return
	}
}

func (svr *namespaceService) Stop() error {
	for t, mgr := range svr.managers {
		err := mgr.CleanUp()
		if err != nil {
			log.Logger(services.NamespaceService, "").WithField("namespace", t).Error(err)
		}
	}
	return nil
}

type serviceConfig struct {
	Capacity  map[types.NamespaceType]int      `json:"capacity"`
	ExtraArgs map[types.NamespaceType][]string `json:"extra_args"`
}

func (svr *namespaceService) handleGetNamespace(conn net.Conn, r nsapi.GetNamespaceRequest) error {
	log.WithInterface(log.Logger(services.NamespaceService, "GetNamespace"), "request", r).Info()
	rsp := nsapi.GetNamespaceResponse{}
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
	log.WithInterface(log.Logger(services.NamespaceService, "GetNamespace"), "response", rsp).Info()
	return nil
}

func (svr *namespaceService) handlePutNamespace(conn net.Conn, r nsapi.PutNamespaceRequest) error {
	log.WithInterface(log.Logger(services.NamespaceService, "PutNamespace"), "request", r).Info()
	rsp := nsapi.PutNamespaceResponse{}
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
	log.WithInterface(log.Logger(services.NamespaceService, "PutNamespace"), "response", rsp).Info()
	return nil
}

func (svr *namespaceService) handleRequest(method string, conn net.Conn) error {
	switch method {
	case nsapi.MethodGetNamespace:
		{
			var r nsapi.GetNamespaceRequest
			if err := utils.ReceiveData(conn, &r); err != nil {
				return err
			}
			return svr.handleGetNamespace(conn, r)
		}
	case nsapi.MethodPutNamespace:
		{
			var r nsapi.PutNamespaceRequest
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
	for _, t := range []types.NamespaceType{types.NamespaceIPC, types.NamespaceMNT, types.NamespaceUTS} {
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
		Capacity: map[types.NamespaceType]int{
			types.NamespaceIPC: 5,
			types.NamespaceUTS: 5,
			types.NamespaceMNT: 5,
		},
		ExtraArgs: map[types.NamespaceType][]string{},
	}
}
