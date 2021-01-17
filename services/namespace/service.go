package namespace

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"path"

	cerm "github.com/YLonely/cer-manager"
	ns "github.com/YLonely/cer-manager/namespace"
	"github.com/YLonely/cer-manager/namespace/ipc"
	"github.com/YLonely/cer-manager/namespace/mnt"
	"github.com/YLonely/cer-manager/namespace/uts"
	"github.com/YLonely/cer-manager/services/checkpoint"

	nsapi "github.com/YLonely/cer-manager/api/services/namespace"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/rootfs/containerd"
	"github.com/YLonely/cer-manager/services"
	"github.com/YLonely/cer-manager/utils"
	"github.com/pkg/errors"
)

type serviceConfig struct {
	Capacity map[types.NamespaceType]int `json:"capacity,omitempty"`
	Refs     []struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"checkpoint_refs"`
}

func New(root string, supplier checkpoint.Supplier) (services.Service, error) {
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
	log.WithInterface(log.Logger(cerm.NamespaceService, "New"), "config", config).Debug("create service with config")
	refs := make([]types.Reference, 0, len(config.Refs))
	for _, ref := range config.Refs {
		refs = append(refs, types.NewContainerdReference(ref.Name, ref.Namespace))
	}
	return &namespaceService{
		capacity: config.Capacity,
		refs:     refs,
		managers: map[types.NamespaceType]ns.Manager{},
		root:     root,
		router:   services.NewRouter(),
		supplier: supplier,
	}, nil
}

type namespaceService struct {
	capacity map[types.NamespaceType]int
	refs     []types.Reference
	managers map[types.NamespaceType]ns.Manager
	root     string
	router   services.Router
	supplier checkpoint.Supplier
}

var _ services.Service = &namespaceService{}

func (svr *namespaceService) Init() error {
	var err error
	if svr.managers[types.NamespaceUTS], err = uts.NewManager(
		svr.root,
		svr.capacity[types.NamespaceUTS],
		svr.refs,
	); err != nil {
		return errors.Wrap(err, "failed to create uts namespace manager")
	}
	if svr.managers[types.NamespaceIPC], err = ipc.NewManager(
		svr.root,
		svr.capacity[types.NamespaceIPC],
		svr.refs,
		svr.supplier,
	); err != nil {
		return errors.Wrap(err, "failed to create ipc namespace manager")
	}
	p, err := containerd.NewProvider()
	if err != nil {
		return err
	}
	if svr.managers[types.NamespaceMNT], err = mnt.NewManager(
		svr.root,
		svr.capacity[types.NamespaceMNT],
		svr.refs,
		p,
		svr.supplier,
	); err != nil {
		return errors.Wrap(err, "failed to create mount namespace namager")
	}
	svr.router.AddHandler(nsapi.MethodGetNamespace, svr.handleGetNamespace)
	svr.router.AddHandler(nsapi.MethodPutNamespace, svr.handlePutNamespace)
	log.Logger(cerm.NamespaceService, "Init").Info("Service initialized")
	return nil
}

func (svr *namespaceService) Handle(ctx context.Context, conn net.Conn) {
	if err := svr.router.Handle(conn); err != nil {
		log.Logger(cerm.NamespaceService, "Handle").Error(err)
		conn.Close()
	}
}

func (svr *namespaceService) Stop() error {
	for t, mgr := range svr.managers {
		err := mgr.CleanUp()
		if err != nil {
			log.Logger(cerm.NamespaceService, "Stop").WithField("namespace", t).Error(err)
		}
	}
	return nil
}

func (svr *namespaceService) handleGetNamespace(conn net.Conn) error {
	var r nsapi.GetNamespaceRequest
	if err := utils.ReceiveObject(conn, &r); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.NamespaceService, "handleGetNamespace"), "request", r).Debug()
	rsp := nsapi.GetNamespaceResponse{}
	if mgr, exists := svr.managers[r.T]; !exists {
		rsp.Fd = -1
		rsp.Info = "No such namespace"
	} else {
		fd, info, err := mgr.Get(r.Ref)
		if err != nil {
			rsp.Fd = -1
			rsp.Info = err.Error()
		} else {
			rsp.Fd = fd
			rsp.Info = info
			rsp.Pid = os.Getpid()
		}
	}
	if err := utils.SendObject(conn, rsp); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.NamespaceService, "handleGetNamespace"), "response", rsp).Debug()
	return nil
}

func (svr *namespaceService) handlePutNamespace(conn net.Conn) error {
	var r nsapi.PutNamespaceRequest
	if err := utils.ReceiveObject(conn, &r); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.NamespaceService, "handlePutNamespace"), "request", r).Debug()
	rsp := nsapi.PutNamespaceResponse{}
	if mgr, exists := svr.managers[r.T]; !exists {
		rsp.Error = "No such namespace"
	} else {
		err := mgr.Put(r.ID)
		if err != nil {
			rsp.Error = err.Error()
		}
	}
	if err := utils.SendObject(conn, rsp); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.NamespaceService, "handlePutNamespace"), "response", rsp).Debug()
	return nil
}

func mergeConfig(to, from *serviceConfig) error {
	for _, t := range []types.NamespaceType{types.NamespaceIPC, types.NamespaceMNT, types.NamespaceUTS} {
		if v, exists := from.Capacity[t]; exists {
			if v < 0 {
				return errors.New("negative namespace capacity")
			}
			to.Capacity[t] = v
		}
	}
	to.Refs = append(to.Refs, from.Refs...)
	return nil
}

func defaultConfig() serviceConfig {
	return serviceConfig{
		Capacity: map[types.NamespaceType]int{
			types.NamespaceIPC: 5,
			types.NamespaceUTS: 5,
			types.NamespaceMNT: 5,
		},
	}
}
