package utils

type Stack[V any] struct {
	arr []V
	len int
}

func NewStack[V any](v ...V) *Stack[V] {
	s := &Stack[V]{
		arr: []V{},
		len: 0,
	}

	for _, e := range v {
		s.Push(e)
	}

	return s
}

func (s *Stack[V]) Pop() V {
	if s.len == 0 {
		panic("stack is empty")
	}
	s.len = s.len - 1
	elem := s.arr[s.len]
	s.arr = s.arr[:s.len]
	return elem
}

func (s *Stack[V]) Push(v V) {
	s.arr = append(s.arr, v)
	s.len = s.len + 1
}

func (s *Stack[V]) Empty() bool {
	return s.len == 0
}

var _ Stack[int]
