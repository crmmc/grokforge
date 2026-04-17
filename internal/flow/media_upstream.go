package flow

import (
	"errors"
	"strings"
)

func validateMediaUpstream(upstreamModel, upstreamMode string) error {
	if strings.TrimSpace(upstreamModel) == "" {
		return errors.New("upstream model is required")
	}
	if strings.TrimSpace(upstreamMode) == "" {
		return errors.New("upstream mode is required")
	}
	return nil
}
