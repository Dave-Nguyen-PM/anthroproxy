package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/Dave-Nguyen-PM/anthroproxy/internal/pool"
)

const target = "https://api.anthropic.com"

type Handler struct {
	pool    *pool.Pool
	target  *url.URL
	reverse *httputil.ReverseProxy
}

func New(p *pool.Pool) (*Handler, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("parsing target URL: %w", err)
	}
	h := &Handler{pool: p, target: u}
	h.reverse = &httputil.ReverseProxy{
		Director:       func(r *http.Request) {}, // we handle this ourselves
		ModifyResponse: nil,
		ErrorHandler:   h.errorHandler,
	}
	return h, nil
}

func (h *Handler) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("proxy error: %v", err)
	http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Read body once so we can replay it on retry
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}
	}

	ts := h.pool.Next()
	if ts == nil {
		http.Error(w, "all tokens are rate-limited, try again later", http.StatusServiceUnavailable)
		log.Printf("[%s] - NO_TOKENS %s %s - 503 %s", time.Now().Format(time.RFC3339), r.Method, r.URL.Path, time.Since(start))
		return
	}

	for {
		status, err := h.doRequest(w, r, bodyBytes, ts, start)
		if err != nil {
			// errorHandler already wrote response
			return
		}
		if status != http.StatusTooManyRequests {
			return
		}

		// 429: mark cooldown and try next token
		log.Printf("[%s] token %q rate-limited, marking cooldown", time.Now().Format(time.RFC3339), ts.Token.Label)
		h.pool.MarkCooldown(ts)

		next := h.pool.NextAfter(ts)
		if next == nil {
			// Already wrote 429 response in doRequest, nothing more to do
			return
		}
		ts = next
		// retry with next token — doRequest will write the new response
	}
}

// doRequest performs the proxied request and writes the response.
// Returns the HTTP status code and whether a fatal error occurred.
func (h *Handler) doRequest(w http.ResponseWriter, r *http.Request, bodyBytes []byte, ts *pool.TokenState, start time.Time) (int, error) {
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, target+r.URL.RequestURI(), bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, "failed to build request", http.StatusInternalServerError)
		return 0, fmt.Errorf("build request: %w", err)
	}

	// Copy headers, replacing Authorization
	for k, vv := range r.Header {
		if http.CanonicalHeaderKey(k) == "Authorization" {
			continue
		}
		for _, v := range vv {
			outReq.Header.Add(k, v)
		}
	}
	outReq.Header.Set("Authorization", "Bearer "+ts.Token.BearerToken)
	outReq.Header.Set("Host", h.target.Host)
	outReq.Host = h.target.Host

	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(outReq)
	if err != nil {
		http.Error(w, "upstream request failed: "+err.Error(), http.StatusBadGateway)
		return 0, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()

	ts.RequestCount.Add(1)

	latency := time.Since(start)
	log.Printf("[%s] token=%q %s %s -> %d (%s)", time.Now().Format(time.RFC3339), ts.Token.Label, r.Method, r.URL.Path, resp.StatusCode, latency)

	// If 429, return the status so the caller can retry — but first write headers
	// For 429 we still forward the response body to the client IF this is our last retry.
	// We detect "last retry" by whether the caller will retry: let caller decide.
	// Here we just return the status; if caller retries it will overwrite the response.
	// Problem: we can't write partial response and then overwrite. So we buffer only on 429.

	if resp.StatusCode == http.StatusTooManyRequests {
		// check if there's a next token available
		next := h.pool.NextAfter(ts)
		if next != nil {
			// Drain body but don't write — we'll retry
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			return http.StatusTooManyRequests, nil
		}
		// No next token; fall through and write the 429 to client
	}

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck

	return resp.StatusCode, nil
}
