package service

import (
	"context"
	"math"
	"net"
)

type ServiceType uint8

const (
	ServiceTypePrefixLen int   = 1
	ServiceTypeMax       uint8 = math.MaxUint8
)

const (
	NamespaceService ServiceType = iota + 10
)

type Service interface {
	Init(context.Context) error
	Handle(context.Context, net.Conn) error
	Stop(context.Context) error
}
