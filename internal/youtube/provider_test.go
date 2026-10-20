package youtube

import (
	"context"
	"errors"
	"testing"
)

func TestUnconfiguredDoesNotPretendSuccess(t *testing.T) {
	var p Unconfigured
	if _, err := p.Upload(context.Background(), "a.mp4", "title", ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if _, err := p.ProcessingStatus(context.Background(), "id"); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
}
