package utils

import "container/list"

type Queue[T any] struct {
	list *list.List
}

func NewQueue[T any]() Queue[T] {
	return Queue[T]{
		list: list.New(),
	}
}

func (q *Queue[T]) Enqueue(elements ...T) {
	for _, element := range elements {
		q.list.PushBack(element)
	}
}

// Pop will get a value from the end of the queue and remove it.
// If no element in the queue, it will return nil.
func (q *Queue[T]) Pop() T {
	element := q.list.Front()
	if element != nil {
		q.list.Remove(element)
	}

	return element.Value.(T)
}

func (q *Queue[T]) Empty() bool {
	return q.list.Len() == 0
}
