package derivation

type stringStack struct {
	store []string
}

func (ss *stringStack) Pop() string {
	n := len(ss.store) - 1
	if n < 0 {
		return ""
	}
	out := ss.store[n]
	ss.store = ss.store[:n]
	return out
}
func (ss *stringStack) Peek() string {
	n := len(ss.store) - 1
	if n < 0 {
		return ""
	}
	return ss.store[n]
}

func (ss *stringStack) Push(v string) {
	ss.store = append(ss.store, v)
}
