package perm

// Set is a permission code set.
type Set map[Code]bool

// Has reports whether the set contains the code.
func (s Set) Has(code Code) bool { return s[code] }

// Effective expands granted codes into the full set including the full
// dependency closure across domains and modules. Unknown codes are ignored.
func Effective(granted []Code) Set {
	out := make(Set, len(granted)*2)
	var add func(code Code)
	add = func(code Code) {
		if out[code] {
			return
		}
		p, ok := regPerms[code]
		if !ok {
			return
		}
		out[code] = true
		for _, dep := range p.Deps {
			add(dep)
		}
	}
	for _, c := range granted {
		add(c)
	}
	return out
}
