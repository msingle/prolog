package engine

import (
	"context"
	"errors"
	"fmt"
)

type bytecode []instruction

type instruction struct {
	opcode  opcode
	operand byte
}

type opcode byte

const (
	opEnter opcode = iota
	opCall
	opExit
	opConst
	opVar
	opFunctor
	opPop

	opCut

	_opLen
)

// VM is the core of a Prolog interpreter. The zero value for VM is a valid VM without any builtin predicates.
type VM struct {

	// OnCall is a callback that is triggered when the VM reaches to the predicate.
	OnCall func(pi ProcedureIndicator, args []Term, env *Env)

	// OnExit is a callback that is triggered when the predicate succeeds and the VM continues.
	OnExit func(pi ProcedureIndicator, args []Term, env *Env)

	// OnFail is a callback that is triggered when the predicate fails and the VM backtracks.
	OnFail func(pi ProcedureIndicator, args []Term, env *Env)

	// OnRedo is a callback that is triggered when the VM retries the predicate as a result of backtrack.
	OnRedo func(pi ProcedureIndicator, args []Term, env *Env)

	// OnUnknown is a callback that is triggered when the VM reaches to an unknown predicate and also current_prolog_flag(unknown, warning).
	OnUnknown func(pi ProcedureIndicator, args []Term, env *Env)

	procedures map[ProcedureIndicator]procedure
	unknown    unknownAction
}

// Register0 registers a predicate of arity 0.
func (vm *VM) Register0(name string, p func(func(*Env) *Promise, *Env) *Promise) {
	if vm.procedures == nil {
		vm.procedures = map[ProcedureIndicator]procedure{}
	}
	vm.procedures[ProcedureIndicator{Name: Atom(name), Arity: 0}] = predicate0(p)
}

// Register1 registers a predicate of arity 1.
func (vm *VM) Register1(name string, p func(Term, func(*Env) *Promise, *Env) *Promise) {
	if vm.procedures == nil {
		vm.procedures = map[ProcedureIndicator]procedure{}
	}
	vm.procedures[ProcedureIndicator{Name: Atom(name), Arity: 1}] = predicate1(p)
}

// Register2 registers a predicate of arity 2.
func (vm *VM) Register2(name string, p func(Term, Term, func(*Env) *Promise, *Env) *Promise) {
	if vm.procedures == nil {
		vm.procedures = map[ProcedureIndicator]procedure{}
	}
	vm.procedures[ProcedureIndicator{Name: Atom(name), Arity: 2}] = predicate2(p)
}

// Register3 registers a predicate of arity 3.
func (vm *VM) Register3(name string, p func(Term, Term, Term, func(*Env) *Promise, *Env) *Promise) {
	if vm.procedures == nil {
		vm.procedures = map[ProcedureIndicator]procedure{}
	}
	vm.procedures[ProcedureIndicator{Name: Atom(name), Arity: 3}] = predicate3(p)
}

// Register4 registers a predicate of arity 4.
func (vm *VM) Register4(name string, p func(Term, Term, Term, Term, func(*Env) *Promise, *Env) *Promise) {
	if vm.procedures == nil {
		vm.procedures = map[ProcedureIndicator]procedure{}
	}
	vm.procedures[ProcedureIndicator{Name: Atom(name), Arity: 4}] = predicate4(p)
}

// Register5 registers a predicate of arity 5.
func (vm *VM) Register5(name string, p func(Term, Term, Term, Term, Term, func(*Env) *Promise, *Env) *Promise) {
	if vm.procedures == nil {
		vm.procedures = map[ProcedureIndicator]procedure{}
	}
	vm.procedures[ProcedureIndicator{Name: Atom(name), Arity: 5}] = predicate5(p)
}

type unknownAction int

const (
	unknownError unknownAction = iota
	unknownFail
	unknownWarning
	_unknownActionLen
)

func (u unknownAction) String() string {
	return [_unknownActionLen]string{
		unknownError:   "error",
		unknownFail:    "fail",
		unknownWarning: "warning",
	}[u]
}

type procedure interface {
	Call(*VM, []Term, func(*Env) *Promise, *Env) *Promise
}

// Arrive is the entry point of the VM.
func (vm *VM) Arrive(pi ProcedureIndicator, args []Term, k func(*Env) *Promise, env *Env) *Promise {
	if vm.OnUnknown == nil {
		vm.OnUnknown = func(ProcedureIndicator, []Term, *Env) {}
	}

	p, ok := vm.procedures[pi]
	if !ok {
		switch vm.unknown {
		case unknownError:
			return Error(existenceErrorProcedure(pi.Term()))
		case unknownWarning:
			vm.OnUnknown(pi, args, env)
			fallthrough
		case unknownFail:
			return Bool(false)
		default:
			return Error(SystemError(fmt.Errorf("unknown unknown: %s", vm.unknown)))
		}
	}

	return Delay(func(context.Context) *Promise {
		return p.Call(vm, args, k, env)
	})
}

type registers struct {
	pc           bytecode
	xr           []Term
	vars         []Variable
	cont         func(*Env) *Promise
	args, astack Term

	pi        []ProcedureIndicator
	env       *Env
	cutParent *Promise
}

func (vm *VM) exec(r registers) *Promise {
	jumpTable := [_opLen]func(r *registers) *Promise{
		opConst:   vm.execConst,
		opVar:     vm.execVar,
		opFunctor: vm.execFunctor,
		opPop:     vm.execPop,
		opEnter:   vm.execEnter,
		opCall:    vm.execCall,
		opExit:    vm.execExit,
		opCut:     vm.execCut,
	}
	for len(r.pc) != 0 {
		op := jumpTable[r.pc[0].opcode]
		p := op(&r)
		if p != nil {
			return p
		}
	}
	return Error(errors.New("non-exit end of bytecode"))
}

func (*VM) execConst(r *registers) *Promise {
	x := r.xr[r.pc[0].operand]
	arest := NewVariable()
	cons := Compound{
		Functor: ".",
		Args:    []Term{x, arest},
	}
	var ok bool
	r.env, ok = r.args.Unify(&cons, false, r.env)
	if !ok {
		return Bool(false)
	}
	r.pc = r.pc[1:]
	r.args = arest
	return nil
}

func (*VM) execVar(r *registers) *Promise {
	v := r.vars[r.pc[0].operand]
	arest := NewVariable()
	cons := Compound{
		Functor: ".",
		Args:    []Term{v, arest},
	}
	var ok bool
	r.env, ok = cons.Unify(r.args, false, r.env)
	if !ok {
		return Bool(false)
	}
	r.pc = r.pc[1:]
	r.args = arest
	return nil
}

func (*VM) execFunctor(r *registers) *Promise {
	pi := r.pi[r.pc[0].operand]
	arg, arest := NewVariable(), NewVariable()
	cons1 := Compound{
		Functor: ".",
		Args:    []Term{arg, arest},
	}
	var ok bool
	r.env, ok = r.args.Unify(&cons1, false, r.env)
	if !ok {
		return Bool(false)
	}
	ok, err := Functor(arg, pi.Name, pi.Arity, func(e *Env) *Promise {
		r.env = e
		return Bool(true)
	}, r.env).Force(context.Background())
	if err != nil {
		return Error(err)
	}
	if !ok {
		return Bool(false)
	}
	r.pc = r.pc[1:]
	r.args = NewVariable()
	cons2 := Compound{
		Functor: ".",
		Args:    []Term{pi.Name, r.args},
	}
	ok, err = Univ(arg, &cons2, func(e *Env) *Promise {
		r.env = e
		return Bool(true)
	}, r.env).Force(context.Background())
	if err != nil {
		return Error(err)
	}
	if !ok {
		return Bool(false)
	}
	r.astack = Cons(arest, r.astack)
	return nil
}

func (*VM) execPop(r *registers) *Promise {
	var ok bool
	r.env, ok = r.args.Unify(List(), false, r.env)
	if !ok {
		return Bool(false)
	}
	r.pc = r.pc[1:]
	a, arest := NewVariable(), NewVariable()
	cons := Compound{
		Functor: ".",
		Args:    []Term{a, arest},
	}
	r.env, ok = r.astack.Unify(&cons, false, r.env)
	if !ok {
		return Bool(false)
	}
	r.args = a
	r.astack = arest
	return nil
}

func (*VM) execEnter(r *registers) *Promise {
	var ok bool
	r.env, ok = r.args.Unify(List(), false, r.env)
	if !ok {
		return Bool(false)
	}
	r.env, ok = r.astack.Unify(List(), false, r.env)
	if !ok {
		return Bool(false)
	}
	r.pc = r.pc[1:]
	v := NewVariable()
	r.args = v
	r.astack = v
	return nil
}

func (vm *VM) execCall(r *registers) *Promise {
	pi := r.pi[r.pc[0].operand]
	var ok bool
	r.env, ok = r.args.Unify(List(), false, r.env)
	if !ok {
		return Bool(false)
	}
	r.pc = r.pc[1:]
	return Delay(func(context.Context) *Promise {
		env := r.env
		args, err := Slice(r.astack, env)
		if err != nil {
			return Error(err)
		}
		return vm.Arrive(pi, args, func(env *Env) *Promise {
			v := NewVariable()
			return vm.exec(registers{
				pc:        r.pc,
				xr:        r.xr,
				vars:      r.vars,
				cont:      r.cont,
				args:      v,
				astack:    v,
				pi:        r.pi,
				env:       env,
				cutParent: r.cutParent,
			})
		}, env)
	})
}

func (*VM) execExit(r *registers) *Promise {
	return r.cont(r.env)
}

func (vm *VM) execCut(r *registers) *Promise {
	r.pc = r.pc[1:]
	return Cut(r.cutParent, func(context.Context) *Promise {
		env := r.env
		return vm.exec(registers{
			pc:        r.pc,
			xr:        r.xr,
			vars:      r.vars,
			cont:      r.cont,
			args:      r.args,
			astack:    r.astack,
			pi:        r.pi,
			env:       env,
			cutParent: r.cutParent,
		})
	})
}

type predicate0 func(func(*Env) *Promise, *Env) *Promise

func (p predicate0) Call(_ *VM, args []Term, k func(*Env) *Promise, env *Env) *Promise {
	if len(args) != 0 {
		return Error(errors.New("wrong number of arguments"))
	}

	return p(k, env)
}

type predicate1 func(Term, func(*Env) *Promise, *Env) *Promise

func (p predicate1) Call(_ *VM, args []Term, k func(*Env) *Promise, env *Env) *Promise {
	if len(args) != 1 {
		return Error(fmt.Errorf("wrong number of arguments: %s", args))
	}

	return p(args[0], k, env)
}

type predicate2 func(Term, Term, func(*Env) *Promise, *Env) *Promise

func (p predicate2) Call(_ *VM, args []Term, k func(*Env) *Promise, env *Env) *Promise {
	if len(args) != 2 {
		return Error(errors.New("wrong number of arguments"))
	}

	return p(args[0], args[1], k, env)
}

type predicate3 func(Term, Term, Term, func(*Env) *Promise, *Env) *Promise

func (p predicate3) Call(_ *VM, args []Term, k func(*Env) *Promise, env *Env) *Promise {
	if len(args) != 3 {
		return Error(errors.New("wrong number of arguments"))
	}

	return p(args[0], args[1], args[2], k, env)
}

type predicate4 func(Term, Term, Term, Term, func(*Env) *Promise, *Env) *Promise

func (p predicate4) Call(_ *VM, args []Term, k func(*Env) *Promise, env *Env) *Promise {
	if len(args) != 4 {
		return Error(errors.New("wrong number of arguments"))
	}

	return p(args[0], args[1], args[2], args[3], k, env)
}

type predicate5 func(Term, Term, Term, Term, Term, func(*Env) *Promise, *Env) *Promise

func (p predicate5) Call(_ *VM, args []Term, k func(*Env) *Promise, env *Env) *Promise {
	if len(args) != 5 {
		return Error(errors.New("wrong number of arguments"))
	}

	return p(args[0], args[1], args[2], args[3], args[4], k, env)
}

// Success is a continuation that leads to true.
func Success(*Env) *Promise {
	return Bool(true)
}

// Failure is a continuation that leads to false.
func Failure(*Env) *Promise {
	return Bool(false)
}

// ProcedureIndicator identifies procedure e.g. (=)/2.
type ProcedureIndicator struct {
	Name  Atom
	Arity Integer
}

// NewProcedureIndicator creates a new ProcedureIndicator from a term that matches Name/Arity.
func NewProcedureIndicator(pi Term, env *Env) (ProcedureIndicator, error) {
	switch p := env.Resolve(pi).(type) {
	case Variable:
		return ProcedureIndicator{}, InstantiationError(pi)
	case *Compound:
		if p.Functor != "/" || len(p.Args) != 2 {
			return ProcedureIndicator{}, typeErrorPredicateIndicator(pi)
		}
		switch f := env.Resolve(p.Args[0]).(type) {
		case Variable:
			return ProcedureIndicator{}, InstantiationError(pi)
		case Atom:
			switch a := env.Resolve(p.Args[1]).(type) {
			case Variable:
				return ProcedureIndicator{}, InstantiationError(pi)
			case Integer:
				pi := ProcedureIndicator{Name: f, Arity: a}
				return pi, nil
			default:
				return ProcedureIndicator{}, typeErrorPredicateIndicator(pi)
			}
		default:
			return ProcedureIndicator{}, typeErrorPredicateIndicator(pi)
		}
	default:
		return ProcedureIndicator{}, typeErrorPredicateIndicator(pi)
	}
}

func (p ProcedureIndicator) String() string {
	return fmt.Sprintf("%s/%d", p.Name, p.Arity)
}

// Term returns p as term.
func (p ProcedureIndicator) Term() Term {
	return &Compound{
		Functor: "/",
		Args: []Term{
			p.Name,
			p.Arity,
		},
	}
}

// Apply applies p to args.
func (p ProcedureIndicator) Apply(args ...Term) (Term, error) {
	if p.Arity != Integer(len(args)) {
		return nil, errors.New("wrong number of arguments")
	}
	return p.Name.Apply(args...), nil
}

func piArgs(t Term, env *Env) (ProcedureIndicator, []Term, error) {
	switch f := env.Resolve(t).(type) {
	case Variable:
		return ProcedureIndicator{}, nil, InstantiationError(t)
	case Atom:
		return ProcedureIndicator{Name: f, Arity: 0}, nil, nil
	case *Compound:
		return ProcedureIndicator{Name: f.Functor, Arity: Integer(len(f.Args))}, f.Args, nil
	default:
		return ProcedureIndicator{}, nil, typeErrorCallable(t)
	}
}
