// Package flaresolverr is a small client for the local FlareSolverr /v1
// endpoint. It is used only as a poster-page fallback when the tracker blocks
// the direct HTTP request.
package flaresolverr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"torrentino/common"
)

const maxResponseBytes = 10 << 20

type request struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout,omitempty"`
}

type response struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL      string `json:"url"`
		Status   int    `json:"status"`
		Response string `json:"response"`
	} `json:"solution"`
}

// Fetch asks FlareSolverr to load target and returns the solved HTML.
func Fetch(ctx context.Context, target string) (string, error) {
	endpoint := strings.TrimSpace(common.Settings.FlareSolverr.URL)
	if endpoint == "" {
		return "", fmt.Errorf("flaresolverr is not configured")
	}
	timeout := common.Settings.FlareSolverrTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload := request{
		Cmd:        "request.get",
		URL:        target,
		MaxTimeout: int(timeout / time.Millisecond),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("flaresolverr returned %s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return "", fmt.Errorf("flaresolverr response is too large")
	}

	var result response
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if !strings.EqualFold(result.Status, "ok") {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = result.Status
		}
		return "", fmt.Errorf("flaresolverr: %s", message)
	}
	if result.Solution.Response == "" {
		return "", fmt.Errorf("flaresolverr returned an empty solution")
	}
	return result.Solution.Response, nil
}
