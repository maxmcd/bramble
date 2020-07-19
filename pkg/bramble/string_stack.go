package bramble

type StringStack struct {
	store []string
}

func (ss *StringStack) Pop() string {
	n := len(ss.store) - 1
	if n < 0 {
		return ""
	}
	out := ss.store[n]
	ss.store = ss.store[:n]
	return out
}
func (ss *StringStack) Peek() string {
	n := len(ss.store) - 1
	if n < 0 {
		return ""
	}
	return ss.store[n]
}

func (ss *StringStack) Push(v string) {
	ss.store = append(ss.store, v)
}
