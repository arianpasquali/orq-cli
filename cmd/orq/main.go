package main

import (
	bartolocli "github.com/orq-ai/bartolo/cli"
	custom "orq/cli/custom"
	generated "orq/cli/generated"
)

func main() {
	bartolocli.Init(&bartolocli.Config{
		AppName:             "orq",
		EnvPrefix:           "ORQ",
		APIKeyEnvVar:        "ORQ_API_KEY",
		DefaultOutputFormat: "toon",
		Version:             "0.1.0",
	})

	generated.Register(bartolocli.Root)
	custom.Register(bartolocli.Root)

	bartolocli.Root.Execute()
}
