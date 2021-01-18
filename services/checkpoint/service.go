package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"

	api "github.com/YLonely/cer-manager/api/services/checkpoint"
	"github.com/YLonely/cer-manager/api/types"
	cp "github.com/YLonely/cer-manager/checkpoint"
	"github.com/YLonely/cer-manager/checkpoint/ccfs"
	"github.com/YLonely/cer-manager/checkpoint/containerd"
	"github.com/YLonely/cer-manager/utils"

	"path"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/services"
	"github.com/pkg/errors"
)

func New(root string) (services.Service, error) {
	const configName = "checkpoint_service.json"
	content, err := ioutil.ReadFile(path.Join(root, configName))
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}
	var providerConfigObj json.RawMessage
	c := config{
		Config: &providerConfigObj,
	}
	if json.Unmarshal(content, &c); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config file")
	}
	s := &service{
		root:    path.Join(root, "checkpoint"),
		router:  services.NewRouter(),
		targets: map[string]struct{}{},
	}
	err = s.initProvider(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create provider")
	}
	return s, nil
}

type service struct {
	root      string
	router    services.Router
	provider  cp.Provider
	sharedMgr cp.SharedManager
	//targets records all the target path where the checkpoint files located
	targets      map[string]struct{}
	m            sync.Mutex
	doneProvider func() error
}

type config struct {
	// type of the checkpoint provider (ccfs or containerd)
	Type string `json:"type"`
	// config for the checkpoint provider
	Config interface{} `json:"config"`
}

var _ services.Service = &service{}

func (s *service) Init() error {
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return err
	}
	s.router.AddHandler(api.MethodGetCheckpoint, s.handleGetCheckpoint)
	s.router.AddHandler(api.MethodPutCheckpoint, s.handlePutCheckpoint)
	log.Logger(cerm.CheckpointService, "Init").Info("Service initialized")
	return nil
}

func (s *service) Handle(ctx context.Context, c net.Conn) {
	if err := s.router.Handle(c); err != nil {
		log.Logger(cerm.CheckpointService, "").Error(err)
		c.Close()
	}
}

func (s *service) Stop() error {
	var failed []string
	for t := range s.targets {
		if err := s.provider.Remove(t); err != nil {
			failed = append(failed, fmt.Sprintf("remove %s with error %s", t, err))
		}
	}
	if s.doneProvider != nil {
		if err := s.doneProvider(); err != nil {
			failed = append(failed, err.Error())
		}
	}
	if len(failed) != 0 {
		return errors.New(strings.Join(failed, ";"))
	}
	return nil
}

func (s *service) Get(ref types.Reference) (string, error) {
	if ref.Name == "" {
		return "", errors.New("empty ref")
	}
	target := path.Join(s.root, ref.Digest())
	s.m.Lock()
	defer s.m.Unlock()
	if _, exists := s.targets[target]; exists {
		return target, nil
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return "", errors.Wrap(err, "failed to create dir "+target)
	}
	if err := s.provider.Prepare(ref, target); err != nil {
		return "", err
	}
	s.targets[target] = struct{}{}
	return target, nil
}

func (s *service) handleGetCheckpoint(c net.Conn) error {
	var r api.GetCheckpointRequest
	if err := utils.ReceiveObject(c, &r); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.CheckpointService, "GetCheckpoint"), "request", r).Debug()
	var resp api.GetCheckpointResponse
	var err error
	resp.Path, err = s.Get(r.Ref)
	if err != nil {
		log.Logger(cerm.CheckpointService, "GetCheckpoint").Error(err)
	}
	if s.sharedMgr != nil {
		s.sharedMgr.Add(r.Ref)
	}
	if err := utils.SendObject(c, resp); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.CheckpointService, "GetCheckpoint"), "response", resp).Debug()
	return nil
}

func (s *service) handlePutCheckpoint(c net.Conn) error {
	var r api.PutCheckpointRequest
	if err := utils.ReceiveObject(c, &r); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.CheckpointService, "PutCheckpoint"), "request", r).Debug()
	var resp api.PutCheckpointResponse
	if s.sharedMgr != nil {
		s.sharedMgr.Release(r.Ref)
	}
	if err := utils.SendObject(c, resp); err != nil {
		return err
	}
	log.WithInterface(log.Logger(cerm.CheckpointService, "PutCheckpoint"), "response", resp).Debug()
	return nil
}

func (s *service) initProvider(c config) error {
	var err error
	switch c.Type {
	case "ccfs":
		var cacheConfig ccfs.Config
		if err = json.Unmarshal(*(c.Config.(*json.RawMessage)), &cacheConfig); err != nil {
			return err
		}
		s.provider, s.doneProvider, err = ccfs.NewProvider(cacheConfig)
		if err != nil {
			return errors.Wrap(err, "failed to create ccfs provider")
		}
		s.sharedMgr = s.provider.(cp.SharedManager)
		log.WithInterface(log.Logger(cerm.CheckpointService, "initProvider"), "config", cacheConfig).Debug("use the checkpoint provider whose backend is ccfs")
	case "containerd":
		var cacheConfig containerd.Config
		if err = json.Unmarshal(*(c.Config.(*json.RawMessage)), &cacheConfig); err != nil {
			return err
		}
		s.provider, err = containerd.NewProvider(cacheConfig)
		if err != nil {
			return errors.Wrap(err, "failed to create containerd provider")
		}
		log.WithInterface(log.Logger(cerm.CheckpointService, "initProvider"), "config", cacheConfig).Debug("use the checkpoint provider whose backend is containerd")
	default:
		return errors.New("invalid provider type")
	}
	return nil
}
