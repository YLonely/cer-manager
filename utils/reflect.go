package utils

import (
	"reflect"

	"github.com/pkg/errors"
)

type Gatherer interface {
	Gather() error
}

type Scatterer interface {
	Scatter() error
}

type GatherScatterer interface {
	Gatherer
	Scatterer
}

// Source returns a value to set a struct field
type Source func() (interface{}, error)

// Target use value v to set the item from which the Source get
type Target func(v interface{}) error

func NewFieldsGatherer(obj interface{}, sources map[string]Source) Gatherer {
	return &fieldsGatherer{
		obj:     obj,
		sources: sources,
	}
}

type fieldsGatherer struct {
	obj     interface{}
	sources map[string]Source
}

func (g *fieldsGatherer) Gather() error {
	v := reflect.ValueOf(g.obj)
	if v.Kind() != reflect.Ptr {
		return errors.New("obj must be a pointer")
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return errors.New("obj must be pointing to a struct")
	}
	for field, source := range g.sources {
		f := elem.FieldByName(field)
		if !f.IsValid() {
			return errors.New("struct does not have field named " + field)
		}
		if !f.CanSet() {
			return errors.Errorf("field %s is unsettable")
		}
		setValue, err := source()
		if err != nil {
			return errors.Wrapf(err, "failed to gather field %s", field)
		}
		if setValue == nil {
			continue
		}
		f.Set(reflect.ValueOf(setValue))
	}
	return nil
}

func NewFieldsScatterer(obj interface{}, targets map[string]Target) Scatterer {
	return &fieldsScatterer{
		obj:     obj,
		targets: targets,
	}
}

type fieldsScatterer struct {
	obj     interface{}
	targets map[string]Target
}

func (s *fieldsScatterer) Scatter() error {
	v := reflect.ValueOf(s.obj)
	if v.Kind() != reflect.Ptr {
		return errors.New("obj must be a pointer")
	}
	elem := v.Elem()
	if elem.Kind() != reflect.Struct {
		return errors.New("obj must be pointting to a struct")
	}
	for field, target := range s.targets {
		f := elem.FieldByName(field)
		if !f.IsValid() {
			return errors.New("struct does not have field named " + field)
		}
		if err := target(f.Interface()); err != nil {
			return errors.Wrap(err, "failed to scatter field "+field)
		}
	}
	return nil
}

func NewFieldsGatherScatterer(obj interface{}, sources map[string]Source, targets map[string]Target) GatherScatterer {
	return &fieldsGatherScatterer{
		fieldsGatherer: &fieldsGatherer{
			obj:     obj,
			sources: sources,
		},
		fieldsScatterer: &fieldsScatterer{
			obj:     obj,
			targets: targets,
		},
	}
}

type fieldsGatherScatterer struct {
	*fieldsGatherer
	*fieldsScatterer
}
