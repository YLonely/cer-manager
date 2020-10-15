package service

import (
	"context"
	"math"
	"net"
)

type ServiceType int

const (
	ServiceTypePrefixLen int    = 2
	ServiceTypeMax       uint16 = math.MaxUint16
)

const (
	NamespaceService ServiceType = iota
)

type Service interface {
	Init(context.Context) error
	Handle(context.Context, net.Conn) error
}
