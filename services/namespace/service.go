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

	nsapi "github.com/YLonely/cer-manager/api/services/namespace"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/rootfs/containerd"
	"github.com/YLonely/cer-manager/services"
	"github.com/YLonely/cer-manager/utils"
	"github.com/pkg/errors"
)

type serviceConfig struct {
	ContainerdCheckpoints []struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace,omitempty"`
		Capacity  int    `json:"capacity,omitempty"`
	} `json:"containerd_checkpoints"`
	DefaultCapacity int `json:"default_capacity"`
}

func New(root string, supplier types.Supplier) (services.Service, error) {
	const configName = "namespace_service.json"
	configPath := path.Join(root, configName)
	config := serviceConfig{}
	if _, err := os.Stat(configPath); err == nil {
		content, err := ioutil.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(content, &config); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if config.DefaultCapacity <= 0 {
		return nil, errors.New("non-positive default capacity is invalid")
	}
	log.WithInterface(log.Logger(cerm.NamespaceService, "New"), "config", config).Debug("create service with config")
	refs := make([]types.Reference, 0, len(config.ContainerdCheckpoints))
	capacities := make([]int, 0, len(config.ContainerdCheckpoints))
	for _, cp := range config.ContainerdCheckpoints {
		ref := types.NewContainerdReference(cp.Name, cp.Namespace)
		refs = append(refs, ref)
		if cp.Capacity <= 0 {
			cp.Capacity = config.DefaultCapacity
		}
		capacities = append(capacities, cp.Capacity)
	}
	return &namespaceService{
		capacities: capacities,
		refs:       refs,
		managers:   map[types.NamespaceType]ns.Manager{},
		root:       root,
		router:     services.NewRouter(),
		supplier:   supplier,
	}, nil
}

type namespaceService struct {
	capacities []int
	refs       []types.Reference
	managers   map[types.NamespaceType]ns.Manager
	root       string
	router     services.Router
	supplier   types.Supplier
}

var _ services.Service = &namespaceService{}

func (svr *namespaceService) Init() error {
	var err error
	if svr.managers[types.NamespaceUTS], err = uts.NewManager(
		svr.root,
		svr.capacities,
		svr.refs,
	); err != nil {
		return errors.Wrap(err, "failed to create uts namespace manager")
	}
	if svr.managers[types.NamespaceIPC], err = ipc.NewManager(
		svr.root,
		svr.capacities,
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
		svr.capacities,
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
