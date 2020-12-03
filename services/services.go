package services

import (
	"context"
	"net"
)

type Service interface {
	Init() error
	Handle(context.Context, net.Conn)
	Stop() error
}

type CheckpointSupplier interface {
	Get(ref string) (string, error)
}
