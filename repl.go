package ok

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/peterh/liner"
)

const (
	replPrompt     = "ok> "
	replEditPrompt = "ok* "
)

var (
	lexerSymbols          = Lexer.Symbols()
	leftToken, rightToken = lexerSymbols["Open"], lexerSymbols["Close"]
)

func REPL(env *Env) error {
	line := liner.NewLiner()
	defer line.Close()
	line.SetCtrlCAborts(true)
	var (
		bal int
		src []string
	)
	for {
		var ps string
		if len(src) == 0 {
			ps = replPrompt
		} else {
			ps = replEditPrompt
			// apply indentation
			ps += strings.Repeat("  ", bal)
		}
		suffix, err := line.Prompt(ps)
		if err != nil {
			if errors.Is(err, liner.ErrPromptAborted) {
				if len(src) != 0 {
					src = nil // reset partial state support
					continue
				}
				return nil
			}
			return err
		}
		tmp := append(src, suffix)
		result, err := EvalString("<stdin>", strings.Join(tmp, " "), env)
		if err != nil {
			i := computeParensBalance(strings.Join(tmp, " "))
			if i >= 0 {
				src, bal = tmp, i
			} else {
				src, bal = nil, 0
				fmt.Println("error:", err)
			}
			continue
		}
		src, bal = nil, 0
		env.Set("_", result)
		line.AppendHistory(suffix)
		fmt.Println(result)
	}
}

func computeParensBalance(src string) int {
	lex, err := Lexer.LexString("<internal>", src)
	if err != nil {
		panic(err)
	}
	tokens, err := collectTokens(lex)
	if err != nil {
		return -1
	}
	var bal int
	for _, t := range tokens {
		switch t.Type {
		case leftToken:
			bal++
		case rightToken:
			bal--
			if bal < 0 {
				return -1
			}
		}
	}
	return bal
}

func collectTokens(lex lexer.Lexer) ([]lexer.Token, error) {
	var out []lexer.Token
	for {
		t, err := lex.Next()
		if err != nil {
			return nil, err
		}
		if t.EOF() {
			break
		}
		out = append(out, t)
	}
	return out, nil
}
