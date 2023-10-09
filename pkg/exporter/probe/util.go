package probe

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"
)

func validateProbeCreatorReturnValue[T interface{}](t reflect.Type) error {
	if t.Kind() != reflect.Func {
		return fmt.Errorf("%s is not Func type", t)
	}

	if t.NumOut() != 2 {
		return fmt.Errorf("expect return value count 2, but actual %d", t.NumOut())
	}

	it := reflect.TypeOf((*T)(nil)).Elem()
	if !t.Out(0).Implements(it) {
		return fmt.Errorf("arg 0 should implement interface %s", it)
	}

	et := reflect.TypeOf((*error)(nil)).Elem()
	if !t.Out(1).Implements(et) {
		return fmt.Errorf("arg 1 should implement error")
	}
	return nil
}

func createStructFromTypeWithArgs(st reflect.Type, args map[string]interface{}) (reflect.Value, error) {
	v := reflect.New(st)
	err := mapstructure.Decode(args, v.Interface())
	return v.Elem(), err
}
