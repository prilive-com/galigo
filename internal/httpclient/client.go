package httpclient

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// Config holds HTTP client configuration.
type Config struct {
	// Timeouts
	RequestTimeout time.Duration
	ConnectTimeout time.Duration
	TLSTimeout     time.Duration
	IdleTimeout    time.Duration

	// Connection pool
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int

	// TLS
	InsecureSkipVerify bool // Only for testing
}

// DefaultConfig returns sensible defaults for Telegram API.
func DefaultConfig() Config {
	return Config{
		RequestTimeout:      30 * time.Second,
		ConnectTimeout:      10 * time.Second,
		TLSTimeout:          10 * time.Second,
		IdleTimeout:         90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		InsecureSkipVerify:  false,
	}
}

// New creates a new HTTP client with the given configuration.
func New(cfg Config) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   cfg.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		},
		TLSHandshakeTimeout:   cfg.TLSTimeout,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       cfg.IdleTimeout,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout,
	}
}

// NewDefault creates a client with default configuration.
func NewDefault() *http.Client {
	return New(DefaultConfig())
}

// DoJSON performs a request and returns the response.
// Caller is responsible for closing the response body.
func DoJSON(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return client.Do(req.WithContext(ctx))
}

// DoMultipart performs a multipart form request.
// Caller is responsible for closing the response body.
func DoMultipart(ctx context.Context, client *http.Client, req *http.Request, contentType string) (*http.Response, error) {
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	return client.Do(req.WithContext(ctx))
}
