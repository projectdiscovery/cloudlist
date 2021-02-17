package main

import (
	"github.com/projectdiscovery/cloudlist/internal/runner"
	"github.com/projectdiscovery/gologger"
)

func main() {
	options := runner.ParseOptions()
	runner, err := runner.New(options)
	if err != nil {
		gologger.Fatal().Msgf("Could not create runner: %s\n", err)
	}
	runner.Enumerate()
}
