package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVariable_Unify(t *testing.T) {
	v1, v2 := Variable("V1"), Variable("V2")
	env, ok := v1.Unify(v2, false, nil)
	assert.True(t, ok)
	env, ok = v1.Unify(Atom("foo"), false, env)
	assert.True(t, ok)
	assert.Equal(t, Atom("foo"), env.Resolve(v1))
	assert.Equal(t, Atom("foo"), env.Resolve(v2))

	v3, v4 := Variable("V3"), Variable("V4")
	env, ok = v3.Unify(v4, false, env)
	assert.True(t, ok)
	env, ok = v4.Unify(Atom("bar"), false, env)
	assert.True(t, ok)
	assert.Equal(t, Atom("bar"), env.Resolve(v3))
	assert.Equal(t, Atom("bar"), env.Resolve(v4))
}

func TestVariable_Unparse(t *testing.T) {
	t.Run("named", func(t *testing.T) {
		v := Variable("X")
		var tokens []Token
		v.Unparse(func(token Token) {
			tokens = append(tokens, token)
		}, WriteTermOptions{}, nil)
		assert.Equal(t, []Token{
			{Kind: TokenVariable, Val: "X"},
		}, tokens)
	})

	t.Run("unnamed", func(t *testing.T) {
		v := NewVariable()
		var tokens []Token
		v.Unparse(func(token Token) {
			tokens = append(tokens, token)
		}, WriteTermOptions{}, nil)
		assert.Len(t, tokens, 1)
		assert.Equal(t, TokenVariable, tokens[0].Kind)
		assert.Regexp(t, `\A_\d+\z`, tokens[0].Val)
	})
}