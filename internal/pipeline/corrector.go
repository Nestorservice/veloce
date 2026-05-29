package pipeline

import (
	"context"

	"github.com/Nestorservice/veloce/internal/gemini"
)

type VerifyFn func(code string) VerifyResult

type CorrectInput struct {
	Target      string // "go" or "dart"
	InitialCode string
	InitialErr  string // optional, the build error of InitialCode
}

type CorrectResult struct {
	Success   bool
	FinalCode string
	Attempts  int // total Gemini calls (initial gen counts as 1)
	UsedPro   bool
	LastError string
	FlashIn   int
	FlashOut  int
	ProIn     int
	ProOut    int
}

type Corrector struct {
	flash  gemini.Client
	pro    gemini.Client
	verify VerifyFn
}

func NewCorrector(flash, pro gemini.Client, verify VerifyFn) *Corrector {
	return &Corrector{flash: flash, pro: pro, verify: verify}
}

// SetVerifyForRun swaps the verifier for the next Correct call.
func (c *Corrector) SetVerifyForRun(v VerifyFn) {
	c.verify = v
}

// Correct runs up to 4 attempts:
//  1. Verify initial code; if OK, return.
//  2. Flash correction #1.
//  3. Flash correction #2.
//  4. Pro correction.
func (c *Corrector) Correct(ctx context.Context, in CorrectInput) (*CorrectResult, error) {
	out := &CorrectResult{FinalCode: in.InitialCode, Attempts: 1}
	if v := c.verify(in.InitialCode); v.OK {
		out.Success = true
		return out, nil
	} else {
		out.LastError = v.Stderr
	}

	code := in.InitialCode
	lastErr := out.LastError

	for i := 0; i < 2; i++ {
		out.Attempts++
		req := gemini.CompletionRequest{Prompt: gemini.BuildCorrectionPrompt(gemini.CorrectionRequest{Target: in.Target, PreviousCode: code, BuildError: lastErr})}
		resp, err := c.flash.Complete(ctx, req)
		if err != nil {
			out.LastError = err.Error()
			return out, nil
		}
		out.FlashIn += resp.InputTokens
		out.FlashOut += resp.OutputTokens
		code = resp.Text
		if v := c.verify(code); v.OK {
			out.Success = true
			out.FinalCode = code
			return out, nil
		} else {
			lastErr = v.Stderr
			out.LastError = lastErr
		}
	}

	// Pro escalation.
	out.Attempts++
	out.UsedPro = true
	req := gemini.CompletionRequest{Prompt: gemini.BuildCorrectionPrompt(gemini.CorrectionRequest{Target: in.Target, PreviousCode: code, BuildError: lastErr})}
	resp, err := c.pro.Complete(ctx, req)
	if err != nil {
		out.LastError = err.Error()
		return out, nil
	}
	out.ProIn += resp.InputTokens
	out.ProOut += resp.OutputTokens
	code = resp.Text
	if v := c.verify(code); v.OK {
		out.Success = true
		out.FinalCode = code
	} else {
		out.LastError = v.Stderr
	}
	out.FinalCode = code
	return out, nil
}
