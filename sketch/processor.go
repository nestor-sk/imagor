package sketch

import (
	"context"

	"github.com/cshum/imagor"
	"github.com/cshum/imagor/imagorpath"
	"go.uber.org/zap"
)

// Implements the interface imagor.Processor
type Processor struct {
	Disabled bool
	Debug    bool
	Logger   *zap.Logger
}

func NewProcessor(options ...Option) *Processor {
	p := &Processor{
		Disabled: false,
		Debug:    false,
		Logger:   zap.NewNop(),
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}

// Startup implements imagor.Processor interface
func (p *Processor) Startup(_ context.Context) error {
	if p.Debug {
		p.Logger.Debug("sketch.Porcessor.Startup")
	}
	return nil
}

// Shutdown implements imagor.Processor interface
func (p *Processor) Shutdown(_ context.Context) error {
	if p.Debug {
		p.Logger.Debug("sketch.Porcessor.Shutdown")
	}
	return nil
}

// Process implements imagor.Processor interface
func (p *Processor) Process(ctx context.Context, blob *imagor.Blob, params imagorpath.Params, load imagor.LoadFunc) (*imagor.Blob, error) {
	if p.Debug {
		p.Logger.Debug("sketch.Processor.Process")
	}
	return blob, nil
}

type Option func(*Processor)

func WithDisableFormat(disabled bool) Option {
	return func(p *Processor) {
		p.Disabled = disabled
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(p *Processor) {
		if logger != nil {
			p.Logger = logger
		}
	}
}

func WithDebug(debug bool) Option {
	return func(p *Processor) {
		p.Debug = debug
	}
}
