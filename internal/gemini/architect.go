package gemini

import "net/http"

// NewProClient returns the architect client (Gemini 2.5 Pro).
func NewProClient(apiKey string) Client {
	return &httpClient{
		name:    "pro",
		model:   "gemini-2.5-pro",
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}
