package svcforge

import (
	"context"
	"fmt"
)

// Options overrides lifecycle paths used by Runner.
// Empty values select platform defaults.
type Options struct {
	SourcePath string
	StateDir   string
}

// Runner executes lifecycle operations for one application.
type Runner struct {
	App     App
	Options Options
}

// Execute runs one lifecycle operation.
func (r Runner) Execute(ctx context.Context, operation Operation) (Result, error) {
	switch operation {
	case OperationInstall, OperationRepair, OperationRemove, OperationStatus:
		return r.executePlatform(ctx, operation)
	default:
		return Result{}, fmt.Errorf("unsupported lifecycle operation %q", operation)
	}
}
