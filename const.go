package cermanager

import "math"

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

var Type2Services = map[ServiceType]string{
	MainService:       "main",
	NamespaceService:  "namespace",
	CheckpointService: "checkpoint",
}
