package list

type T interface {
}

type node struct {
	prev *node
	next *node
	data T
}

type List struct {
	head *node
	len  int32
}

func New() *List {
	return &List{
		head: &node{
			prev: nil,
			next: nil,
		},
		len: 0,
	}
}

func (l *List) PushBack(d T) {
	n := &node{data: d}
	if l.len == 0 {
		n.next = l.head
		n.prev = l.head
		l.head.next = n
		l.head.prev = n
	} else {
		n.next = l.head
		n.prev = l.head.prev
		n.prev.next = n
		l.head.prev = n
	}
	l.len++
}

func (l *List) PushFront(d T) {
	n := &node{data: d}
	if l.len == 0 {
		n.next = l.head
		n.prev = l.head
		l.head.next = n
		l.head.prev = n
	} else {
		n.next = l.head.next
		n.prev = l.head
		n.next.prev = n
		l.head.next = n
	}
	l.len++
}

func (l *List) PopBack() (d T) {
	p
	l.head.prev = l.head.prev.prev
	l.head.prev
}
