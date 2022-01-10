# ok

a toy interpreted lisp

## repl

run the repl

```shell
go run github.com/jncornett/ok/cmd/repl
```

example session:

```lisp
ok> (let concat (func a b (list a b)))
((a, b) => 9f90b94a128e848a86650d6ce75619c8)@func
ok> (concat
ok*   2
ok*   3)
[2@number 3@number]@array
ok> 
```

## grammar

like other lisps, the grammar is exceedingly simple:

```ebnf
BlockNode = (StatementNode ";"?)* .
StatementNode = ("(" SexprNode ")") | ConstNode | TagNode .
SexprNode = TagNode StatementNode* .
TagNode = (<ident> | <op>) .
ConstNode = <string> | <number> .
```

*Grammar can also be evaluated dynamically -- see [`cmd/ebnf/main.go`](cmd/ebnf/main.go).*

## builtins

core functionality is provided through builtins.

### `let` assignment

syntax: todo

### `func` function literals

syntax: todo

### `switch` branching

syntax: todo

## acknowledgements

- [participle](https://github.com/alecthomas/participle) - a unique, reflection-based parser generator.
- [liner](https://github.com/peterh/liner) - a read-evaluate-print looper.
