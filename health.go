package svcforge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func runHealthCheck(ctx context.Context, check HealthCheck) error {
	if check.Check == nil && check.URL == "" {
		return nil
	}
	if check.InitialDelay > 0 {
		timer := time.NewTimer(check.InitialDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	timeout := check.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		if check.Check != nil {
			lastErr = check.Check(checkCtx)
		} else {
			lastErr = checkHTTP(checkCtx, check)
		}
		if lastErr == nil {
			return nil
		}
		select {
		case <-checkCtx.Done():
			if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("health check timed out after %s: %w", timeout, lastErr)
			}
			return checkCtx.Err()
		case <-ticker.C:
		}
	}
}

func checkHTTP(ctx context.Context, check HealthCheck) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, check.URL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	expected := check.ExpectedStatus
	if expected == 0 {
		expected = http.StatusOK
	}
	if resp.StatusCode != expected {
		return fmt.Errorf("health endpoint returned HTTP %d, expected %d", resp.StatusCode, expected)
	}
	return nil
}
