package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/nestor/veloce/internal/gemini"
)

type fakeClient struct {
	name  string
	resps []string
	calls int
}

func (f *fakeClient) Name() string { return f.name }
func (f *fakeClient) Complete(ctx context.Context, req gemini.CompletionRequest) (*gemini.CompletionResponse, error) {
	if f.calls >= len(f.resps) {
		return &gemini.CompletionResponse{Text: f.resps[len(f.resps)-1]}, nil
	}
	r := f.resps[f.calls]
	f.calls++
	return &gemini.CompletionResponse{Text: r, InputTokens: 10, OutputTokens: 5}, nil
}

func TestCorrect_SucceedsOnFirstRetry(t *testing.T) {
	flash := &fakeClient{name: "flash", resps: []string{"good code"}}
	pro := &fakeClient{name: "pro"}

	verify := func(code string) VerifyResult {
		if strings.Contains(code, "good") {
			return VerifyResult{OK: true}
		}
		return VerifyResult{OK: false, Stderr: "bad"}
	}

	c := NewCorrector(flash, pro, verify)
	res, err := c.Correct(context.Background(), CorrectInput{Target: "go", InitialCode: "bad code"})
	if err != nil {
		t.Fatal(err)
	}
	if res.FinalCode != "good code" || res.Attempts != 2 {
		t.Errorf("got %+v", res)
	}
	if res.UsedPro {
		t.Errorf("Pro should not have been called")
	}
}

func TestCorrect_EscalatesToProAfterFlashFails(t *testing.T) {
	flash := &fakeClient{name: "flash", resps: []string{"bad1", "bad2"}}
	pro := &fakeClient{name: "pro", resps: []string{"finally good"}}

	verify := func(code string) VerifyResult {
		if strings.Contains(code, "good") {
			return VerifyResult{OK: true}
		}
		return VerifyResult{OK: false, Stderr: "bad"}
	}

	c := NewCorrector(flash, pro, verify)
	res, _ := c.Correct(context.Background(), CorrectInput{Target: "go", InitialCode: "bad0"})
	if !res.UsedPro {
		t.Error("Pro should have been called")
	}
	if res.FinalCode != "finally good" {
		t.Errorf("FinalCode = %q", res.FinalCode)
	}
}

func TestCorrect_FailsAfterAllAttempts(t *testing.T) {
	flash := &fakeClient{name: "flash", resps: []string{"bad1", "bad2"}}
	pro := &fakeClient{name: "pro", resps: []string{"still bad"}}

	verify := func(code string) VerifyResult {
		return VerifyResult{OK: false, Stderr: "broken"}
	}

	c := NewCorrector(flash, pro, verify)
	res, _ := c.Correct(context.Background(), CorrectInput{Target: "go", InitialCode: "bad0"})
	if res.Success {
		t.Error("expected failure")
	}
	if res.Attempts != 4 {
		t.Errorf("Attempts = %d, want 4", res.Attempts)
	}
}
