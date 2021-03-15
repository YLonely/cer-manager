package cermanager

import "math"

type ServiceType uint16

const (
	ServiceTypePrefixLen int    = 2
	ServiceTypeMax       uint16 = math.MaxUint16
)

const (
	NamespaceService ServiceType = iota + 10
	CheckpointService
	HttpService
)

var Type2Services = map[ServiceType]string{
	NamespaceService:  "namespace",
	CheckpointService: "checkpoint",
	HttpService:       "http",
}
