package main

import (
	"os"

	"github.com/cshum/imagor/config"
	"github.com/cshum/imagor/config/awsconfig"
	"github.com/cshum/imagor/config/gcloudconfig"
	"github.com/cshum/imagor/config/sketchconfig"
	"github.com/cshum/imagor/config/vipsconfig"
)

func main() {
	var server = config.CreateServer(
		os.Args[1:],
		sketchconfig.WithSketch,
		vipsconfig.WithVips,
		awsconfig.WithAWS,
		gcloudconfig.WithGCloud,
	)
	if server != nil {
		server.Run()
	}
}
