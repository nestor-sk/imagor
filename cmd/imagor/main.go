package main

import (
	"os"

	"github.com/cshum/imagor/config"
	"github.com/cshum/imagor/config/awsconfig"
	"github.com/cshum/imagor/config/vipsconfig"
)

func main() {
	var server = config.CreateServer(
		os.Args[1:],
		vipsconfig.WithVips,
		awsconfig.WithAWS,
	)
	if server != nil {
		server.Run()
	}
}
