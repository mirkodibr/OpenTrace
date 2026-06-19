package opentrace

import "time"

// Option is a functional option applied to Config during New().
type Option func(*Config)

// WithCollectorEndpoint sets the URL of the OpenTrace collector service.
func WithCollectorEndpoint(endpoint string) Option {
	return func(c *Config) { c.CollectorEndpoint = endpoint }
}

// WithServiceName sets the service.name resource attribute on all emitted events.
func WithServiceName(name string) Option {
	return func(c *Config) { c.ServiceName = name }
}

// WithServiceVersion sets the service.version resource attribute.
func WithServiceVersion(version string) Option {
	return func(c *Config) { c.ServiceVersion = version }
}

// WithEnvironment sets the deployment environment tag (e.g. "production", "staging").
func WithEnvironment(env string) Option {
	return func(c *Config) { c.Environment = env }
}

// WithMinLevel sets the minimum severity level. Events below this level are
// discarded on the hot path with zero cost.
func WithMinLevel(level Level) Option {
	return func(c *Config) { c.MinLevel = level }
}

// WithBatchSize sets the maximum number of events per export batch.
func WithBatchSize(size int) Option {
	return func(c *Config) { c.BatchSize = size }
}

// WithBatchInterval sets the maximum time to wait before flushing a partial batch.
func WithBatchInterval(d time.Duration) Option {
	return func(c *Config) { c.BatchInterval = d }
}

// WithBufferSize sets the capacity of the internal event buffer. Events are
// dropped (and counted) when the buffer is full.
func WithBufferSize(size int) Option {
	return func(c *Config) { c.BufferSize = size }
}

// WithHTTPTimeout sets the per-request timeout for exporter HTTP calls.
func WithHTTPTimeout(d time.Duration) Option {
	return func(c *Config) { c.HTTPTimeout = d }
}

// WithMaxRetries sets the maximum number of retry attempts for transient errors.
func WithMaxRetries(n int) Option {
	return func(c *Config) { c.MaxRetries = n }
}

// WithHeader adds a custom header to all exporter HTTP requests. Call multiple
// times to add multiple headers (e.g. API keys, auth tokens).
func WithHeader(key, value string) Option {
	return func(c *Config) { c.Headers[key] = value }
}

// WithCompression enables or disables gzip compression of exported batches.
func WithCompression(enabled bool) Option {
	return func(c *Config) { c.CompressionEnabled = enabled }
}

// WithDebug enables SDK-internal debug logging to stderr. Not for production use.
func WithDebug(enabled bool) Option {
	return func(c *Config) { c.Debug = enabled }
}
