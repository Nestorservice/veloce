package gemini

import "net/http"

// NewFlashClient returns the worker client (Gemini 2.5 Flash).
func NewFlashClient(apiKey string) Client {
	return &httpClient{
		name:    "flash",
		model:   "gemini-2.5-flash",
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}

// AttachLimiter wires a shared rate limiter into a client returned by NewFlashClient/NewProClient.
// No-op if the client is not the built-in HTTP implementation.
func AttachLimiter(c Client, l *RateLimiter) {
	if h, ok := c.(*httpClient); ok {
		h.SetRateLimiter(l)
	}
}
