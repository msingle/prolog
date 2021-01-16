package prolog

import (
	"fmt"
	"strconv"
	"strings"
)

type Term interface {
	fmt.Stringer
	TermString(operators, *[]*Variable) string
	Unify(Term, bool) bool
	Copy() Term
}

type Atom string

func (a Atom) String() string {
	return a.TermString(nil, nil)
}

func (a Atom) TermString(operators, *[]*Variable) string {
	return string(a)
}

func (a Atom) Unify(t Term, occursCheck bool) bool {
	switch t := t.(type) {
	case Atom:
		return a == t
	case *Variable:
		return t.Unify(a, occursCheck)
	default:
		return false
	}
}

func (a Atom) Copy() Term {
	return a
}

type Integer int64

func (i Integer) String() string {
	return i.TermString(nil, nil)
}

func (i Integer) TermString(operators, *[]*Variable) string {
	return strconv.FormatInt(int64(i), 10)
}

func (i Integer) Unify(t Term, occursCheck bool) bool {
	switch t := t.(type) {
	case Integer:
		return i == t
	case *Variable:
		return t.Unify(i, occursCheck)
	default:
		return false
	}
}

func (i Integer) Copy() Term {
	return i
}

type Variable struct {
	Name string
	Ref  Term
}

func (v *Variable) String() string {
	var stop []*Variable
	return v.TermString(nil, &stop)
}

func (v *Variable) TermString(os operators, stop *[]*Variable) string {
	name := v.Name
	if name == "" {
		if v.Ref != nil {
			return v.Ref.TermString(os, stop)
		}
		name = fmt.Sprintf("_%p", v)
	}
	if v.Ref == nil {
		return name
	}
	if stop != nil {
		for _, s := range *stop {
			if v == s {
				return name
			}
		}
	}
	*stop = append(*stop, v)
	return fmt.Sprintf("%s = %s", name, v.Ref.TermString(os, stop))
}

func (v *Variable) Unify(t Term, occursCheck bool) bool {
	if occursCheck && Contains(t, v) {
		return false
	}
	if v.Ref != nil {
		return v.Ref.Unify(t, occursCheck)
	}
	if w, ok := t.(*Variable); ok && w.Ref == nil {
		t = &Variable{}
		w.Ref = t
	}
	v.Ref = t
	return true
}

func (v *Variable) Copy() Term {
	if v.Ref == nil {
		return &Variable{}
	}
	return &Variable{Ref: v.Ref.Copy()}
}

type Compound struct {
	Functor Atom
	Args    []Term
}

func (c *Compound) String() string {
	var stop []*Variable
	return c.TermString([]operator{
		{Precedence: 400, Type: "yfx", Name: "/"}, // for principal functors
	}, &stop)
}

func (c *Compound) TermString(os operators, stop *[]*Variable) string {
	if c.Functor == "." && len(c.Args) == 2 { // list
		t := Term(c)
		var (
			elems []string
			rest  string
		)
		for {
			if l, ok := t.(*Compound); ok && l.Functor == "." && len(l.Args) == 2 {
				elems = append(elems, l.Args[0].TermString(os, stop))
				t = l.Args[1]
				continue
			}
			if a, ok := t.(Atom); ok && a == "[]" {
				break
			}
			rest = "|" + t.TermString(os, stop)
			break
		}
		return fmt.Sprintf("[%s%s]", strings.Join(elems, ", "), rest)
	}

	switch len(c.Args) {
	case 1:
		for _, o := range os {
			if o.Name != c.Functor {
				continue
			}
			switch o.Type {
			case `xf`, `yf`:
				return fmt.Sprintf("%s%s", c.Args[0].TermString(os, stop), c.Functor.TermString(os, stop))
			case `fx`, `fy`:
				return fmt.Sprintf("%s%s", c.Functor.TermString(os, stop), c.Args[0].TermString(os, stop))
			}
		}
	case 2:
		for _, o := range os {
			if o.Name != c.Functor {
				continue
			}
			switch o.Type {
			case `xfx`, `xfy`, `yfx`:
				return fmt.Sprintf("%s%s%s", c.Args[0].TermString(os, stop), c.Functor.TermString(os, stop), c.Args[1].TermString(os, stop))
			}
		}
	}

	args := make([]string, len(c.Args))
	for i, arg := range c.Args {
		args[i] = arg.TermString(os, stop)
	}
	return fmt.Sprintf("%s(%s)", c.Functor.TermString(os, stop), strings.Join(args, ", "))
}

func (c *Compound) Unify(t Term, occursCheck bool) bool {
	switch t := t.(type) {
	case *Compound:
		if c.Functor != t.Functor {
			return false
		}
		if len(c.Args) != len(t.Args) {
			return false
		}
		for i := range c.Args {
			if !c.Args[i].Unify(t.Args[i], occursCheck) {
				return false
			}
		}
		return true
	case *Variable:
		return t.Unify(c, occursCheck)
	default:
		return false
	}
}

func (c *Compound) Copy() Term {
	args := make([]Term, len(c.Args))
	for i, a := range c.Args {
		args[i] = a.Copy()
	}
	return &Compound{
		Functor: c.Functor,
		Args:    args,
	}
}

func Cons(car, cdr Term) Term {
	return &Compound{
		Functor: ".",
		Args:    []Term{car, cdr},
	}
}

func List(ts ...Term) Term {
	return ListRest(Atom("[]"), ts...)
}

func ListRest(rest Term, ts ...Term) Term {
	l := rest
	for i := len(ts) - 1; i >= 0; i-- {
		l = Cons(ts[i], l)
	}
	return l
}

func Resolve(t Term) Term {
	var stop []*Variable
	for t != nil {
		switch v := t.(type) {
		case Atom, Integer, *Compound:
			return v
		case *Variable:
			if v.Ref == nil {
				return v
			}
			for _, s := range stop {
				if v == s {
					return v
				}
			}
			stop = append(stop, v)
			t = v.Ref
		}
	}
	return nil
}

func Contains(t, s Term) bool {
	switch t := t.(type) {
	case *Variable:
		if t == s {
			return true
		}
		if t.Ref == nil {
			return false
		}
		return Contains(t.Ref, s)
	case *Compound:
		if s, ok := s.(Atom); ok && t.Functor == s {
			return true
		}
		for _, a := range t.Args {
			if Contains(a, s) {
				return true
			}
		}
		return false
	default:
		return t == s
	}
}
