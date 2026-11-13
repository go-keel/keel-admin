package base64captcha

import (
	"context"
	"testing"
	"time"

	driver "github.com/mojocn/base64Captcha"
)

// newTestProvider builds a provider backed by base64Captcha's own memory
// store (not real Redis), injected through the unexported constructor —
// same seam-injection pattern as sys/data's newConfigRepo.
func newTestProvider(mem driver.Store, length int) *provider {
	return newProvider(func(context.Context) driver.Store { return mem }, length)
}

func TestIssueReturnsIDAndImage(t *testing.T) {
	mem := driver.NewMemoryStore(1000, time.Minute)
	p := newTestProvider(mem, 4)

	ch, err := p.Issue(context.Background())
	if err != nil {
		t.Fatalf("Issue: unexpected error: %v", err)
	}
	if ch == nil {
		t.Fatal("Issue: expected non-nil challenge")
	}
	if ch.ID == "" {
		t.Error("Issue: expected non-empty ID")
	}
	if ch.ImageBase64 == "" {
		t.Error("Issue: expected non-empty ImageBase64")
	}
}

func TestVerifyCorrectAnswerPasses(t *testing.T) {
	mem := driver.NewMemoryStore(1000, time.Minute)
	p := newTestProvider(mem, 4)
	ctx := context.Background()

	ch, err := p.Issue(ctx)
	if err != nil {
		t.Fatalf("Issue: unexpected error: %v", err)
	}
	answer := mem.Get(ch.ID, false) // peek without clearing
	if answer == "" {
		t.Fatal("expected the memory store to hold the issued answer")
	}

	ok, err := p.Verify(ctx, ch.ID, answer, "1.2.3.4")
	if err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("Verify: expected correct answer to pass")
	}
}

func TestVerifyWrongAnswerRejected(t *testing.T) {
	mem := driver.NewMemoryStore(1000, time.Minute)
	p := newTestProvider(mem, 4)
	ctx := context.Background()

	ch, err := p.Issue(ctx)
	if err != nil {
		t.Fatalf("Issue: unexpected error: %v", err)
	}
	answer := mem.Get(ch.ID, false)
	wrong := answer + "9" // guaranteed mismatch regardless of the real answer

	ok, err := p.Verify(ctx, ch.ID, wrong, "1.2.3.4")
	if err != nil {
		t.Fatalf("Verify: unexpected error: %v", err)
	}
	if ok {
		t.Fatal("Verify: expected wrong answer to be rejected")
	}
}

func TestVerifySameAnswerCannotBeRetried(t *testing.T) {
	mem := driver.NewMemoryStore(1000, time.Minute)
	p := newTestProvider(mem, 4)
	ctx := context.Background()

	ch, err := p.Issue(ctx)
	if err != nil {
		t.Fatalf("Issue: unexpected error: %v", err)
	}
	answer := mem.Get(ch.ID, false)

	ok1, err := p.Verify(ctx, ch.ID, answer, "1.2.3.4")
	if err != nil {
		t.Fatalf("Verify (first): unexpected error: %v", err)
	}
	if !ok1 {
		t.Fatal("Verify (first): expected correct answer to pass")
	}

	ok2, err := p.Verify(ctx, ch.ID, answer, "1.2.3.4")
	if err != nil {
		t.Fatalf("Verify (retry): unexpected error: %v", err)
	}
	if ok2 {
		t.Fatal("Verify (retry): same id/answer must be rejected once consumed (GETDEL semantics)")
	}
}

func TestReadyAlwaysNil(t *testing.T) {
	mem := driver.NewMemoryStore(1000, time.Minute)
	p := newTestProvider(mem, 4)

	if err := p.Ready(); err != nil {
		t.Fatalf("Ready: expected nil, got %v", err)
	}
}
