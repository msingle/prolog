package prolog

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/ichiban/prolog/engine"
)

//go:embed bootstrap.pl
var bootstrap string

var libraries = map[string]func(*Interpreter) error{}

// Register registers a library. if there's a library with the same name registered, it panics.
func Register(name string, library func(*Interpreter) error) {
	if _, ok := libraries[name]; ok {
		panic(fmt.Sprintf("%s is already registered.", name))
	}

	libraries[name] = library
}

// Interpreter is a Prolog interpreter. The zero value is a valid interpreter without any predicates/operators defined.
type Interpreter struct {
	engine.State
}

// New creates a new Prolog interpreter with predefined predicates/operators.
func New(in io.Reader, out io.Writer) *Interpreter {
	var i Interpreter
	i.SetUserInput(in)
	i.SetUserOutput(out)
	i.Register0("repeat", i.Repeat)
	i.Register1(`\+`, i.Negation)
	i.Register1("call", i.Call)
	i.Register1("current_predicate", i.CurrentPredicate)
	i.Register1("assertz", i.Assertz)
	i.Register1("asserta", i.Asserta)
	i.Register1("retract", i.Retract)
	i.Register1("abolish", i.Abolish)
	i.Register1("var", engine.TypeVar)
	i.Register1("float", engine.TypeFloat)
	i.Register1("integer", engine.TypeInteger)
	i.Register1("atom", engine.TypeAtom)
	i.Register1("compound", engine.TypeCompound)
	i.Register1("throw", engine.Throw)
	i.Register2("=", engine.Unify)
	i.Register2("unify_with_occurs_check", engine.UnifyWithOccursCheck)
	i.Register2("=..", engine.Univ)
	i.Register2("copy_term", engine.CopyTerm)
	i.Register3("arg", engine.Arg)
	i.Register3("bagof", i.BagOf)
	i.Register3("setof", i.SetOf)
	i.Register3("findall", i.FindAll)
	i.Register3("catch", i.Catch)
	i.Register3("functor", engine.Functor)
	i.Register3("op", i.Op)
	i.Register3("compare", engine.Compare)
	i.Register3("current_op", i.CurrentOp)
	i.Register1("current_input", i.CurrentInput)
	i.Register1("current_output", i.CurrentOutput)
	i.Register1("set_input", i.SetInput)
	i.Register1("set_output", i.SetOutput)
	i.Register4("open", i.Open)
	i.Register2("close", i.Close)
	i.Register1("flush_output", i.FlushOutput)
	i.Register3("write_term", i.WriteTerm)
	i.Register2("char_code", engine.CharCode)
	i.Register2("put_byte", i.PutByte)
	i.Register2("put_code", i.PutCode)
	i.Register3("read_term", i.ReadTerm)
	i.Register2("get_byte", i.GetByte)
	i.Register2("get_char", i.GetChar)
	i.Register2("peek_byte", i.PeekByte)
	i.Register2("peek_char", i.PeekChar)
	i.Register1("halt", engine.Halt)
	i.Register2("clause", i.Clause)
	i.Register2("atom_length", engine.AtomLength)
	i.Register3("atom_concat", engine.AtomConcat)
	i.Register5("sub_atom", engine.SubAtom)
	i.Register2("atom_chars", engine.AtomChars)
	i.Register2("atom_codes", engine.AtomCodes)
	i.Register2("number_chars", engine.NumberChars)
	i.Register2("number_codes", engine.NumberCodes)
	i.Register2("is", engine.DefaultFunctionSet.Is)
	i.Register2("=:=", engine.DefaultFunctionSet.Equal)
	i.Register2("=\\=", engine.DefaultFunctionSet.NotEqual)
	i.Register2("<", engine.DefaultFunctionSet.LessThan)
	i.Register2(">", engine.DefaultFunctionSet.GreaterThan)
	i.Register2("=<", engine.DefaultFunctionSet.LessThanOrEqual)
	i.Register2(">=", engine.DefaultFunctionSet.GreaterThanOrEqual)
	i.Register2("stream_property", i.StreamProperty)
	i.Register2("set_stream_position", i.SetStreamPosition)
	i.Register2("char_conversion", i.CharConversion)
	i.Register2("current_char_conversion", i.CurrentCharConversion)
	i.Register2("set_prolog_flag", i.SetPrologFlag)
	i.Register2("current_prolog_flag", i.CurrentPrologFlag)
	i.Register1("dynamic", i.Dynamic)
	i.Register1("built_in", i.BuiltIn)
	i.Register2("expand_term", i.ExpandTerm)
	i.Register1("consult", i.consult)
	i.Register2("environ", engine.Environ)
	if err := i.Exec(bootstrap); err != nil {
		panic(err)
	}

	return &i
}

// Exec executes a prolog program.
func (i *Interpreter) Exec(query string, args ...interface{}) error {
	return i.ExecContext(context.Background(), query, args...)
}

// ExecContext executes a prolog program with context.
func (i *Interpreter) ExecContext(ctx context.Context, query string, args ...interface{}) error {
	// Ignore shebang line.
	if len(query) > 2 && query[:2] == "#!" {
		i := strings.Index(query, "\n")
		if i < 0 {
			i = len(query) - 1
		}
		query = query[i:]
	}

	p := i.Parser(strings.NewReader(query), nil)
	if err := p.Replace("?", args...); err != nil {
		return err
	}
	for p.More() {
		t, err := p.Term()
		if err != nil {
			return err
		}

		v := engine.NewVariable()
		if _, err := i.ExpandTerm(t, v, func(env *engine.Env) *engine.Promise {
			return i.AssertStatic(v, engine.Success, env)
		}, nil).Force(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Query executes a prolog query and returns *Solutions.
func (i *Interpreter) Query(query string, args ...interface{}) (*Solutions, error) {
	return i.QueryContext(context.Background(), query, args...)
}

// QueryContext executes a prolog query and returns *Solutions with context.
func (i *Interpreter) QueryContext(ctx context.Context, query string, args ...interface{}) (*Solutions, error) {
	p := i.Parser(strings.NewReader(query), nil)
	if err := p.Replace("?", args...); err != nil {
		return nil, err
	}
	t, err := p.Term()
	if err != nil {
		return nil, err
	}

	var env *engine.Env

	more := make(chan bool, 1)
	next := make(chan *engine.Env)
	sols := Solutions{
		vars: env.FreeVariables(t),
		more: more,
		next: next,
	}

	go func() {
		defer close(next)
		if !<-more {
			return
		}
		if _, err := i.Call(t, func(env *engine.Env) *engine.Promise {
			next <- env
			return engine.Bool(!<-more)
		}, env).Force(ctx); err != nil {
			sols.err = err
		}
	}()

	return &sols, nil
}

// ErrNoSolutions indicates there's no solutions for the query.
var ErrNoSolutions = errors.New("no solutions")

// QuerySolution executes a Prolog query for the first solution.
func (i *Interpreter) QuerySolution(query string, args ...interface{}) *Solution {
	return i.QuerySolutionContext(context.Background(), query, args...)
}

// QuerySolutionContext executes a Prolog query with context.
func (i *Interpreter) QuerySolutionContext(ctx context.Context, query string, args ...interface{}) *Solution {
	sols, err := i.QueryContext(ctx, query, args...)
	if err != nil {
		return &Solution{err: err}
	}

	if !sols.Next() {
		if err := sols.Err(); err != nil {
			return &Solution{err: err}
		}
		return &Solution{err: ErrNoSolutions}
	}

	return &Solution{sols: sols, err: sols.Close()}
}

func (i *Interpreter) consult(files engine.Term, k func(*engine.Env) *engine.Promise, env *engine.Env) *engine.Promise {
	switch f := env.Resolve(files).(type) {
	case engine.Variable:
		return engine.Error(engine.InstantiationError(files))
	case *engine.Compound:
		if f.Functor == "." && len(f.Args) == 2 {
			if err := engine.EachList(f, func(elem engine.Term) error {
				return i.consultOne(elem, env)
			}, env); err != nil {
				return engine.Error(err)
			}
			return k(env)
		}
		if err := i.consultOne(f, env); err != nil {
			return engine.Error(err)
		}
		return k(env)
	default:
		if err := i.consultOne(f, env); err != nil {
			return engine.Error(err)
		}
		return k(env)
	}
}

func (i *Interpreter) consultOne(file engine.Term, env *engine.Env) error {
	switch f := env.Resolve(file).(type) {
	case engine.Atom:
		for _, f := range []string{string(f), string(f) + ".pl"} {
			b, err := ioutil.ReadFile(f)
			if err != nil {
				continue
			}

			if err := i.Exec(string(b)); err != nil {
				return err
			}

			return nil
		}
		return engine.DomainError("source_sink", file, "%s does not exist.", file)
	case *engine.Compound:
		if f.Functor != "library" || len(f.Args) != 1 {
			return engine.TypeError("atom", file, "%s is not an atom.", file)
		}

		library, ok := env.Resolve(f.Args[0]).(engine.Atom)
		if !ok {
			return engine.TypeError("atom", f.Args[0], "%s is not an atom.", f.Args[0])
		}

		l, ok := libraries[string(library)]
		if !ok {
			return engine.ExistenceError("library", library, "%s is not a library.", library)
		}

		return l(i)
	default:
		return engine.TypeError("atom", file, "%s is not an atom.", file)
	}
}
