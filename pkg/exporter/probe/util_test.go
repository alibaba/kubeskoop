package probe

import (
	"golang.org/x/exp/slices"
	"reflect"
	"testing"
)

type test interface {
	TestA()
	TestB()
}

type aa struct {
}

func (a *aa) TestA() {
}

func (a *aa) TestB() {

}

func funcA() (test, error) {
	return nil, nil
}

func funcB() (*aa, error) {
	return nil, nil
}

func funcC() test {
	return nil
}

func funcD() (string, error) {
	return "", nil
}

func funcE() (test, string) {
	return nil, ""
}

func TestValidateProbeCreatorReturnValue(t *testing.T) {
	testcases := []struct {
		name string
		f    interface{}
		err  bool
	}{
		{"funcA", funcA, false},
		{"funcB", funcB, false},
		{"funcC", funcC, true},
		{"funcD", funcD, true},
		{"funcE", funcE, true},
	}

	for _, c := range testcases {
		err := validateProbeCreatorReturnValue[test](reflect.TypeOf(c.f))
		if c.err && err == nil {
			t.Errorf("testcase %s except error, but nil", c.name)
			t.Fail()
		}

		if !c.err && err != nil {
			t.Errorf("testcase %s except nil error, but %s", c.name, err.Error())
			t.Fail()
		}
	}
}

func TestCreateStructFromTypeWithArgs(t *testing.T) {
	type s struct {
		ArgA string
		ArgB uint32
		ArgC []string
	}

	st := reflect.TypeOf(s{})

	m := map[string]interface{}{
		"argA": "a",
		"argB": 1,
		"argC": []string{"a", "b"},
		"argD": 1,
	}
	r, err := createStructFromTypeWithArgs(st, m)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	sr := r.Interface().(s)
	if sr.ArgA != "a" || sr.ArgB != 1 || !slices.Equal(sr.ArgC, []string{"a", "b"}) {
		t.Fatalf("expect %+v, actual %+v", m, sr)
	}

	m = map[string]interface{}{
		"a": "a",
		"b": "b",
		"c": "c",
	}
	r, err = createStructFromTypeWithArgs(reflect.TypeOf(map[string]string{}), m)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}
	mr := r.Interface().(map[string]string)
	if len(mr) != 3 || mr["a"] != "a" || mr["b"] != "b" || mr["c"] != "c" {
		t.Fatalf("expect %+v, actual %+v", m, mr)
	}
}
