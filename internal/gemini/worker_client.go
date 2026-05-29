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
