package services

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
	CheckpointService
)

type Service interface {
	Init() error
	Handle(context.Context, net.Conn)
	Stop() error
}
