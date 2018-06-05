package stack

import "container/list"

type Stack struct {
	l list.List
}

func New() Stack {
	return Stack{}
}

func (s *Stack) Push(v interface{}) {
	s.l.PushFront(v)
}

func (s *Stack) Pop() interface{} {
	return s.l.Remove(s.l.Front())
}

func (s *Stack) IsEmpty() bool {
	if s.l.Len() == 0 {
		return true
	}
	return false
}

func (s *Stack) Top() interface{} {
	if !s.IsEmpty() {
		return s.l.Front().Value
	}
	return nil
}
