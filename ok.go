package ok

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

var Lexer = lexer.MustSimple([]lexer.Rule{
	{Name: "WS", Pattern: `\s+`},
	{Name: "Open", Pattern: `\(`},
	{Name: "Close", Pattern: `\)`},
	{Name: "String", Pattern: `"([^"]*)"`}, // FIXME good enough for now
	{Name: "Number", Pattern: `\d+`},       // FIXME good enough for now
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "Op", Pattern: `[-+*/%<>=!&|^]+`},
	{Name: "Semi", Pattern: `;`},
})

type BlockNode struct {
	Statements []StatementNode `parser:"(@@ ';'?)*"`
}

func (n BlockNode) Parse() Node {
	var out Block
	for _, stmt := range n.Statements {
		out = append(out, stmt.Parse())
	}
	switch len(out) {
	case 0:
		return Const{Value: Nil{}}
	case 1:
		return out[0]
	}
	return out
}

type StatementNode struct {
	Sexpr *SexprNode `parser:"  '(' @@ ')'"`
	Const *ConstNode `parser:"| @@"`
	Tag   *TagNode   `parser:"| @@"`
}

func (n StatementNode) Parse() Node {
	switch {
	case n.Sexpr != nil:
		return n.Sexpr.Parse()
	case n.Const != nil:
		return n.Const.Parse()
	case n.Tag != nil:
		return Ref(n.Tag.Value)
	}
	panic("unreachable")
}

type SexprNode struct {
	Tag  TagNode         `parser:"@@"`
	Args []StatementNode `parser:"@@*"`
}

func (n SexprNode) Parse() Node {
	var args []Node
	for _, arg := range n.Args {
		args = append(args, arg.Parse())
	}
	return Call{
		Callee: Ref(n.Tag.Value),
		Args:   args,
	}
}

type ConstNode struct {
	String string `parser:"  @String"`
	Number string `parser:"| @Number"`
}

func (n ConstNode) Parse() Node {
	switch {
	case n.String != "":
		s, err := strconv.Unquote(n.String)
		if err != nil {
			panic(err)
		}
		return Const{Value: String(s)}
	case n.Number != "":
		i, err := strconv.ParseInt(n.Number, 10, 64)
		if err != nil {
			panic(err)
		}
		return Const{Value: Number(i)}
	}
	panic("unreachable")
}

type TagNode struct {
	Value string `parser:"@(Ident | Op)"`
}

var Parser = participle.MustBuild(&BlockNode{}, participle.Lexer(Lexer), participle.Elide("WS"))

type Node interface {
	Eval(env *Env) (Value, error)
	String() string
}

type Env []Scope

func (env Env) Get(key string) (Value, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := env[i].Get(key); ok {
			return v, true
		}
	}
	return nil, false
}

func (env Env) Set(key string, val Value) {
	env[len(env)-1].Set(key, val)
}

func (env Env) Del(key string) {
	env[len(env)-1].Del(key)
}

func (env *Env) Push(scope Scope) {
	*env = append(*env, scope)
}

func (env *Env) Pop() {
	*env = (*env)[:len(*env)-1]
}

type Scope map[string]Value

func (scope Scope) Get(key string) (Value, bool) {
	v, ok := scope[key]
	return v, ok
}

func (scope Scope) Set(key string, val Value) { scope[key] = val }
func (scope Scope) Del(key string)            { delete(scope, key) }

type Value interface {
	TypeName() string
	String() string
	Bool() bool
	Nil() bool
}

type Callable interface {
	Call(env *Env, args ...Value) (Value, error)
}

type Expander interface {
	Expand(env *Env, args ...Node) (Node, error)
}

type Block []Node

var _ Node = Block(nil)

func (b Block) Eval(env *Env) (Value, error) {
	var val Value
	for _, node := range b {
		var err error
		val, err = node.Eval(env)
		if err != nil {
			return nil, err
		}
	}
	return val, nil
}

func (b Block) String() string {
	var out string
	for _, node := range b {
		out += node.String()
	}
	return out
}

type Const struct{ Value }

var _ Node = Const{}

func (c Const) Eval(env *Env) (Value, error) { return c.Value, nil }

type Assign struct {
	Key   string
	Value Node
}

func (a Assign) Eval(env *Env) (Value, error) {
	val, err := a.Value.Eval(env)
	if err != nil {
		return nil, err
	}
	env.Set(a.Key, val)
	return val, nil
}

func (a Assign) String() string {
	return fmt.Sprintf("(let %s %v)", a.Key, a.Value)
}

type Switch []Branch

type Branch struct {
	Cond Node
	Body Node
}

func (sw Switch) Eval(env *Env) (Value, error) {
	for _, b := range sw {
		cond, err := b.Cond.Eval(env)
		if err != nil {
			return nil, err
		}
		if cond.Bool() {
			return b.Body.Eval(env)
		}
	}
	return Nil{}, nil
}

func (sw Switch) String() string {
	var sb strings.Builder
	sb.WriteString("(switch ")
	for _, b := range sw {
		sb.WriteString(b.Cond.String())
		sb.WriteString(" ")
		sb.WriteString(b.Body.String())
	}
	sb.WriteString(")")
	return sb.String()
}

type Ref string

func (r Ref) Eval(env *Env) (Value, error) {
	v, ok := env.Get(string(r))
	if !ok {
		return Nil{}, nil
	}
	return v, nil
}

func (r Ref) String() string { return fmt.Sprintf("%T(%q)", r, string(r)) }

type Call struct {
	Callee Node
	Args   []Node
}

func (c Call) Eval(env *Env) (Value, error) {
	fn, err := c.Callee.Eval(env)
	if err != nil {
		return nil, err
	}
	if ex, ok := fn.(Expander); ok {
		node, err := ex.Expand(env, c.Args...)
		if err != nil {
			return nil, err
		}
		return node.Eval(env)
	}
	var args []Value
	for _, arg := range c.Args {
		val, err := arg.Eval(env)
		if err != nil {
			return nil, err
		}
		args = append(args, val)
	}
	cb, ok := fn.(Callable)
	if !ok {
		return nil, fmt.Errorf("%s is not callable", fn)
	}
	return cb.Call(env, args...)
}

func (c Call) String() string {
	return fmt.Sprintf("%T(%v, %v)", c, c.Callee, c.Args)
}

type Array []Value

var _ Value = Array(nil)

func (Array) TypeName() string { return "array" }
func (a Array) String() string { return fmt.Sprintf("%v@%v", []Value(a), a.TypeName()) }
func (a Array) Bool() bool     { return len(a) != 0 }
func (Array) Nil() bool        { return false }

type Func struct {
	Params []string
	Code   Node
}

var (
	_ Value    = Func{}
	_ Callable = Func{}
)

func (Func) TypeName() string { return "func" }
func (f Func) String() string {
	const maxCode = 32
	var sb strings.Builder
	sb.WriteString("(")
	sb.WriteString("(")
	sb.WriteString(strings.Join(f.Params, ", "))
	sb.WriteString(") => ")
	code := fmt.Sprintf("{ %v }", f.Code.String())
	if len(code) > maxCode {
		h := sha256.New()
		h.Write([]byte(code))
		code = hex.EncodeToString(h.Sum(nil))[:maxCode]
	}
	sb.WriteString(code)
	sb.WriteString(")")
	sb.WriteString("@")
	sb.WriteString(f.TypeName())
	return sb.String()
}
func (Func) Bool() bool { return true }
func (Func) Nil() bool  { return false }

func (f Func) Call(env *Env, args ...Value) (Value, error) {
	next := Scope{}
	n := len(f.Params)
	if n > len(args) {
		n = len(args)
	}
	for i, p := range f.Params[:n] {
		next.Set(p, args[i])
	}
	env.Push(next)
	defer env.Pop()
	return f.Code.Eval(env)
}

type Builtin func(env *Env, args ...Value) (Value, error)

var _ Value = Builtin(nil)

func (Builtin) TypeName() string { return "builtin" }
func (b Builtin) String() string { return fmt.Sprintf("%p@%v", b, b.TypeName()) }
func (Builtin) Bool() bool       { return true }
func (Builtin) Nil() bool        { return false }

func (fn Builtin) Call(env *Env, args ...Value) (Value, error) { return fn(env, args...) }

type Macro func(env *Env, args ...Node) (Node, error)

var (
	_ Value    = Macro(nil)
	_ Expander = Macro(nil)
)

func (Macro) TypeName() string { return "macro" }
func (m Macro) String() string { return fmt.Sprintf("%p@%v", m, m.TypeName()) }
func (Macro) Bool() bool       { return true }
func (Macro) Nil() bool        { return false }

func (fn Macro) Expand(env *Env, args ...Node) (Node, error) { return fn(env, args...) }

type String string

var _ Value = String("")

func (String) TypeName() string { return "string" }
func (s String) String() string { return fmt.Sprintf("%q@%v", string(s), s.TypeName()) }
func (s String) Bool() bool     { return s != "" }
func (String) Nil() bool        { return false }

type Number int64

var _ Value = Number(0)

func (Number) TypeName() string { return "number" }
func (n Number) String() string { return fmt.Sprintf("%d@%v", int64(n), n.TypeName()) }
func (n Number) Bool() bool     { return n != 0 }
func (Number) Nil() bool        { return false }

type Bool bool

var _ Value = Bool(false)

func (Bool) TypeName() string { return "bool" }
func (b Bool) String() string { return fmt.Sprintf("%v@%v", bool(b), b.TypeName()) }
func (b Bool) Bool() bool     { return bool(b) }
func (Bool) Nil() bool        { return false }

type Nil struct{}

var _ Value = Nil{}

func (Nil) TypeName() string { return "nil" }
func (Nil) String() string   { return "nil" }
func (Nil) Bool() bool       { return false }
func (Nil) Nil() bool        { return true }

func Parse(file string, src io.Reader) (Node, error) {
	var cst BlockNode
	if err := Parser.Parse(file, src, &cst); err != nil {
		return nil, err
	}
	return cst.Parse(), nil
}

func Eval(file string, r io.Reader, env *Env) (Value, error) {
	node, err := Parse(file, r)
	if err != nil {
		return nil, err
	}
	return node.Eval(env)
}

func EvalString(file, src string, env *Env) (Value, error) {
	return Eval(file, strings.NewReader(src), env)
}

func DefaultEnv() *Env {
	return &Env{
		{
			"func": Macro(func(_ *Env, args ...Node) (Node, error) {
				var (
					params []string
					body   Node
				)
				if len(args) > 0 {
					for _, arg := range args[:len(args)-1] {
						ref, ok := arg.(Ref)
						if !ok {
							return nil, fmt.Errorf("expected parameter name, got %v", arg)
						}
						params = append(params, string(ref))
					}
					body = args[len(args)-1]
				} else {
					body = Const{Value: Nil{}}
				}
				return Const{
					Value: Func{
						Params: params,
						Code:   body,
					},
				}, nil
			}),
			"switch": Macro(func(_ *Env, args ...Node) (Node, error) {
				var out Switch
				for i := 0; i < len(args); i += 2 {
					branch := Branch{Cond: args[i]}
					if i+1 < len(args) {
						branch.Body = args[i+1]
					}
					out = append(out, branch)
				}
				return out, nil
			}),
			"let": Macro(func(_ *Env, args ...Node) (Node, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("let takes 2 arguments, got %d", len(args))
				}
				ref, ok := args[0].(Ref)
				if !ok {
					return nil, fmt.Errorf("expected parameter name, got %v", args[0])
				}
				return Assign{
					Key:   string(ref),
					Value: args[1],
				}, nil
			}),
			"id": Builtin(func(_ *Env, args ...Value) (Value, error) {
				if len(args) == 0 {
					return Nil{}, nil
				}
				return args[0], nil
			}),
			"list": Builtin(func(_ *Env, args ...Value) (Value, error) {
				return Array(args), nil
			}),
		},
		{},
	}
}
