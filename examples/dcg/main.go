package main

import (
	"flag"
	"fmt"

	"github.com/ichiban/prolog"
	_ "github.com/ichiban/prolog/dcg" // import this to enable `:- [library(dcg)].`
)

// This example explains how to parse a simple English sentence with DCG (Definite Clause Grammar).
// You can check if it parses a sentence with `go run examples/dcg/main.go <SENTENCE>`. If it does, the program returns
// `true` otherwise `false`. Also, you can generate every possible sentence by providing a prefix
// `go run examples/dcg/main.go -prefix <PREFIX>`.
//
// e.g.)
//   $ go run examples/dcg/main.go the cat chases the mouse
//   $ go run examples/dcg/main.go -prefix the cat

func main() {
	var prefix bool
	flag.BoolVar(&prefix, "prefix", false, "prefix search mode")
	flag.Parse()

	// First, create a Prolog interpreter.
	i := prolog.New(nil, nil)

	// Then, define DCG rules with -->/2.
	// Note that we've imported "github.com/ichiban/prolog/dcg" to load `library(dcg)`.
	if err := i.Exec(`
% We need to consult library(dcg) to enable DCG.
:- [library(dcg)].

s --> np, vp.
np --> det, n.
vp --> v, np.

det --> [the].
det --> [a].
n --> [dog].
n --> [cat].
n --> [mouse].
v --> [chases].
v --> [ignores].
`); err != nil {
		panic(err)
	}

	// Finally, query with phrase/2.
	if prefix {
		sols, err := i.Query(`Prefix = ?, append(Prefix, _, Sentence), phrase(s, Sentence).`, flag.Args())
		if err != nil {
			panic(err)
		}
		defer func() {
			if err := sols.Close(); err != nil {
				panic(err)
			}
		}()

		for sols.Next() {
			var s struct {
				Sentence []string
			}
			if err := sols.Scan(&s); err != nil {
				panic(err)
			}

			fmt.Printf("%s\n", s.Sentence)
		}
		return
	}

	sols, err := i.Query(`phrase(s, ?).`, flag.Args())
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := sols.Close(); err != nil {
			panic(err)
		}
	}()

	fmt.Printf("%t\n", sols.Next())
}
