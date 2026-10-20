package youtube

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestUnconfiguredDoesNotPretendSuccess(t *testing.T) {
	var p Unconfigured
	if p.Configured() || p.Connected(context.Background()) {
		t.Fatal("should be off")
	}
	if _, err := p.Upload(context.Background(), "a.mp4", "title", ""); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if _, err := p.ProcessingStatus(context.Background(), "id"); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if _, err := p.UploadVideo(context.Background(), "a.mp4", UploadOptions{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if _, err := p.GetConnectedChannel(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if err := p.Disconnect(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
}

func TestMapAPIErrorQuota(t *testing.T) {
	err := mapAPIError(&googleapi.Error{
		Code:    403,
		Message: "Quota",
		Errors:  []googleapi.ErrorItem{{Reason: "quotaExceeded"}},
	})
	if !errors.Is(err, ErrQuota) {
		t.Fatalf("%v", err)
	}
	err = mapAPIError(&googleapi.Error{Code: 401, Message: "Unauthorized"})
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("%v", err)
	}
	err = mapAPIError(&googleapi.Error{Code: 503, Message: "Unavailable"})
	if !errors.Is(err, ErrTemporary) {
		t.Fatalf("%v", err)
	}
}
