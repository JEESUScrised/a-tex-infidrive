package youtube

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"google.golang.org/api/googleapi"
)

func mapAPIError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		switch gerr.Code {
		case http.StatusUnauthorized:
			return errors.Join(ErrAuth, err)
		case http.StatusForbidden:
			if isQuota(gerr) {
				return errors.Join(ErrQuota, err)
			}
			return errors.Join(ErrAuth, err)
		case http.StatusTooManyRequests:
			return errors.Join(ErrQuota, err)
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return errors.Join(ErrTemporary, err)
		}
		if isQuota(gerr) {
			return errors.Join(ErrQuota, err)
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return errors.Join(ErrTemporary, err)
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "quota") || strings.Contains(msg, "ratelimitexceeded") {
		return errors.Join(ErrQuota, err)
	}
	return err
}

func isQuota(gerr *googleapi.Error) bool {
	for _, e := range gerr.Errors {
		r := strings.ToLower(e.Reason)
		if strings.Contains(r, "quota") || strings.Contains(r, "ratelimit") || r == "dailylimitexceeded" {
			return true
		}
	}
	return strings.Contains(strings.ToLower(gerr.Message), "quota")
}

func isRetryable(err error) bool {
	return errors.Is(err, ErrTemporary) || errors.Is(err, ErrQuota)
}
