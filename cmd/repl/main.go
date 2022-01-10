package main

import (
	"github.com/jncornett/ok"
)

func main() {
	err := ok.REPL(ok.DefaultEnv())
	if err != nil {
		panic(err)
	}
}
