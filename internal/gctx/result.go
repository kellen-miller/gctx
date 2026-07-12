package gctx

import "errors"

// ErrSelectionCanceled reports an intentional fuzzy-selection cancellation.
var ErrSelectionCanceled = errors.New("selection canceled")

// Result contains the safe, non-secret context fields shown after a switch.
type Result struct {
	Name         string
	Account      string
	Project      string
	QuotaProject string
	Warning      string
}
