package main

import (
	"flag"
	"io"
	"log"
	"os"

	"github.com/jncornett/ok"
)

func main() {
	flag.Parse()
	if err := run(flag.Args()...); err != nil {
		log.Fatal(err)
	}
}

func run(args ...string) error {
	type input struct {
		name string
		r    io.Reader
	}
	var inputs []input
	if len(args) == 0 {
		inputs = []input{{"<stdin>", os.Stdin}}
	} else {
		for _, arg := range args {
			f, err := os.Open(arg)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			inputs = append(inputs, input{arg, f})
		}
	}
	env := ok.DefaultEnv()
	for _, in := range inputs {
		_, err := ok.Eval(in.name, in.r, env)
		if err != nil {
			return err
		}
	}
	return nil
}
