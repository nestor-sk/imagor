package sketchconfig

import (
	"flag"

	"github.com/cshum/imagor"
	"github.com/cshum/imagor/sketch"
	"go.uber.org/zap"
)

// WithSketch with libvips processor config option
func WithSketch(fs *flag.FlagSet, cb func() (*zap.Logger, bool)) imagor.Option {
	var (
		sketchFormatDisabled = fs.Bool("sketch-file-format-disabled", false,
			"Disable Sketch file format processor")

		logger, isDebug = cb()
	)
	return imagor.WithProcessors(
		sketch.NewProcessor(
			sketch.WithDisableFormat(*sketchFormatDisabled),
			sketch.WithLogger(logger),
			sketch.WithDebug(isDebug),
		),
	)
}
