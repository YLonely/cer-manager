package cermanager

import (
	"context"
	"io"
	"net"
	"os"
	"path"
	"sync"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/services"
	"github.com/YLonely/cer-manager/services/checkpoint"
	"github.com/YLonely/cer-manager/services/namespace"
	"github.com/YLonely/cer-manager/utils"
)

const DefaultRootPath = "/var/lib/cermanager"
const DefaultSocketName = "daemon.socket"

type Server struct {
	services map[cerm.ServiceType]services.Service
	listener net.Listener
	group    sync.WaitGroup
}

func NewServer() (*Server, error) {
	if err := os.MkdirAll(DefaultRootPath, 0755); err != nil {
		return nil, err
	}
	socketPath := path.Join(DefaultRootPath, DefaultSocketName)
	os.Remove(socketPath)
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}
	checkpointSvr, err := checkpoint.New(DefaultRootPath)
	if err != nil {
		return nil, err
	}
	namespaceSvr, err := namespace.New(DefaultRootPath, checkpointSvr.(services.CheckpointSupplier))
	if err != nil {
		return nil, err
	}
	svr := &Server{
		services: map[cerm.ServiceType]services.Service{
			cerm.NamespaceService:  namespaceSvr,
			cerm.CheckpointService: checkpointSvr,
		},
		listener: listener,
	}
	for _, service := range svr.services {
		if err = service.Init(); err != nil {
			return nil, err
		}
	}
	return svr, nil
}

func (s *Server) Start(ctx context.Context) chan error {
	errorC := make(chan error, 1)
	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				errorC <- err
				return
			}
			go s.serve(ctx, conn, errorC)
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return errorC
}

func (s *Server) serve(ctx context.Context, conn net.Conn, errorC chan error) {
	s.group.Add(1)
	defer s.group.Done()
	for {
		svrType, err := utils.ReceiveServiceType(conn)
		if err != nil {
			if err != io.EOF {
				log.Logger(cerm.MainService, "").WithError(err).Error("Can't handle service type")
			}
			conn.Close()
			return
		}
		if svr, exists := s.services[svrType]; !exists {
			conn.Close()
			log.Logger(cerm.MainService, "").WithField("serviceType", svrType).Error("No such service")
		} else {
			svr.Handle(ctx, conn)
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (s *Server) Shutdown() {
	s.group.Wait()
	for t, ss := range s.services {
		if err := ss.Stop(); err != nil {
			log.Logger(cerm.MainService, "").WithField("serviceType", t).WithError(err).Error()
		}
	}
}
