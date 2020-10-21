package service

import (
	"context"
	"math"
	"net"
)

type ServiceType uint16

const (
	ServiceTypePrefixLen int    = 2
	ServiceTypeMax       uint16 = math.MaxUint16
)

const (
	MainService ServiceType = iota + 10
	NamespaceService
)

type Service interface {
	Init() error
	Handle(context.Context, net.Conn)
	Stop(context.Context) error
}
