package crdaemon

import (
	"context"
	"net"
	"os"
	"path"
	"sync"

	"github.com/YLonely/cr-daemon/log"
	"github.com/YLonely/cr-daemon/namespace"
	"github.com/YLonely/cr-daemon/service"
	"github.com/YLonely/cr-daemon/utils"
	"github.com/pkg/errors"
)

const bundle = "/var/lib/crdaemon"

type Server struct {
	services map[service.ServiceType]service.Service
	listener net.Listener
	group    sync.WaitGroup
}

func NewServer() (*Server, error) {
	if err := os.MkdirAll(bundle, 0755); err != nil {
		return nil, err
	}
	socketPath := path.Join(bundle, "daemon.socket")
	os.Remove(socketPath)
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}
	namespaceSvr, err := namespace.NewNamespaceService(bundle)
	if err != nil {
		return nil, err
	}
	svr := &Server{
		services: map[service.ServiceType]service.Service{
			service.NamespaceService: namespaceSvr,
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
			conn.Close()
			errorC <- errors.Wrap(err, "Can't receive service type")
			return
		}
		if svr, exists := s.services[svrType]; !exists {
			conn.Close()
			log.Logger(service.MainService).WithField("serviceType", svrType).Error("No such service")
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
			log.Logger(service.MainService).WithField("serviceType", t).WithError(err).Error()
		}
	}
}
