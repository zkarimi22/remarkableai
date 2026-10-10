package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLegacyConfigGetsTypingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"api_key":"test","model":"test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TypingDelayMS != 180 || cfg.LeadingLineBreaks != 2 {
		t.Fatalf("unexpected upgrade defaults: delay=%d lines=%d", cfg.TypingDelayMS, cfg.LeadingLineBreaks)
	}
	if got := replyText(cfg, "hello"); got != "\n\nhello\n" {
		t.Fatalf("missing spacing: %q", got)
	}
	if got := replyPayload(cfg, "follow-up", false); got != "follow-up\n" {
		t.Fatalf("follow-up received an extra spacing buffer: %q", got)
	}
	if got := replyPayload(cfg, "first", true); got != "\n\nfirst\n" {
		t.Fatalf("first reply spacing changed: %q", got)
	}
	if cfg.Model != "test" || cfg.SystemInstructions != defaultSystemInstructions || cfg.WebSearch {
		t.Fatal("legacy config must retain its model and not silently enable paid search")
	}
	// A maximum-size reply plus configurable spacing stays within the helper's
	// erase limit; count the actual payload so undo cannot leave extra newlines.
	cfg.LeadingLineBreaks = 6
	if got := len(replyText(cfg, strings.Repeat("a", 700))); got != 707 {
		t.Fatalf("unexpected maximum payload: %d", got)
	}
}

func TestTypingSettingsValidation(t *testing.T) {
	for _, cfg := range []config{
		{TypingDelayMS: -1}, {TypingDelayMS: 99}, {TypingDelayMS: 501},
		{LeadingLineBreaks: -1}, {LeadingLineBreaks: 7},
	} {
		if cfg.typingDefaults() == nil {
			t.Fatal("accepted out-of-range settings")
		}
	}
	cfg := config{TypingDelayMS: 250, LeadingLineBreaks: 1}
	if err := cfg.typingDefaults(); err != nil {
		t.Fatal(err)
	}
	if cfg.TypingDelayMS != 250 || cfg.LeadingLineBreaks != 1 {
		t.Fatal("overwrote user preferences")
	}
}

func TestFramebufferAddress(t *testing.T) {
	maps := "1000-2000 rw-s 0 00:00 1 /dev/fb0\n2000-ffffff rw-p 0 00:00 0\n"
	got, err := framebufferAddress(maps)
	if err != nil || got != 0x2000+2629632+8 {
		t.Fatalf("%d %v", got, err)
	}
	if _, err = framebufferAddress("not a mapping"); err == nil {
		t.Fatal("accepted bad mapping")
	}
}

func tap(s *touchState, x, y int, now time.Time) inputSignal {
	s.event(3, 47, 0, now)
	s.event(3, 57, 1, now)
	s.event(3, 53, int32(x), now)
	s.event(3, 54, int32(y), now)
	s.event(0, 0, 0, now)
	s.event(3, 57, -1, now.Add(80*time.Millisecond))
	return s.event(0, 0, 0, now.Add(80*time.Millisecond))
}

func TestDoubleTapAndSyntheticFocus(t *testing.T) {
	var s touchState
	now := time.Now()
	if tap(&s, 1350, 60, now).Trigger {
		t.Fatal("single tap triggered")
	}
	if !tap(&s, 1350, 60, now.Add(300*time.Millisecond)).Trigger {
		t.Fatal("double-tap missed")
	}
	if tap(&s, 1350, 60, now.Add(2*time.Second)).Trigger {
		t.Fatal("stale double-tap")
	}
	if tap(&s, 500, 500, now.Add(2300*time.Millisecond)).Trigger {
		t.Fatal("wrong corner")
	}
	if event := tap(&s, 702, 1, now.Add(3*time.Second)); event.Activity || event.Trigger {
		t.Fatal("own cursor tap would cancel request")
	}
}

func TestUndoGestureAndToolbarExclusion(t *testing.T) {
	var s touchState
	now := time.Now()
	first := tap(&s, 190, 180, now)
	if first.Undo || !first.UndoZone {
		t.Fatal("first undo tap misclassified")
	}
	second := tap(&s, 190, 180, now.Add(300*time.Millisecond))
	if !second.Undo || second.Trigger || !second.UndoZone {
		t.Fatal("undo gesture missed")
	}
	// Native menu area is deliberately outside the undo zone.
	tap(&s, 40, 180, now.Add(time.Second))
	menu := tap(&s, 40, 180, now.Add(1300*time.Millisecond))
	if menu.Undo || menu.UndoZone || menu.Trigger {
		t.Fatal("menu tap would delete text")
	}
	// One tap in each corner must not combine into a destructive gesture.
	tap(&s, 1350, 60, now.Add(2*time.Second))
	mixed := tap(&s, 190, 180, now.Add(2300*time.Millisecond))
	if mixed.Undo || mixed.Trigger {
		t.Fatal("mixed corners triggered")
	}
	// Typing/navigation outside the undo zone is visible to the caller for invalidation.
	other := tap(&s, 800, 500, now.Add(3*time.Second))
	if !other.Activity || other.UndoZone {
		t.Fatal("navigation was not detectable")
	}
}

func TestReplyValidation(t *testing.T) {
	text, err := normalizeReply("It’s Friday — enjoy!")
	if err != nil || text != "It's Friday - enjoy!" {
		t.Fatalf("%q %v", text, err)
	}
	for _, bad := range []string{"", strings.Repeat("x", 12001), "\x1b[2J", "Hi 🐶"} {
		if _, err := normalizeReply(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if long, err := normalizeReply(strings.Repeat("x", 2000)); err != nil || len(long) != 2000 {
		t.Fatalf("rejected a reasonable long answer: %v", err)
	}
	note, err := normalizeReply("PDF_NOTE_TITLE: Alexander the Great\nAlexander was a Macedonian king.")
	if err != nil || note != "PDF_NOTE_TITLE:Alexander the Great\nAlexander was a Macedonian king." {
		t.Fatalf("did not preserve PDF note structure: %q %v", note, err)
	}
}

func TestPDFWriter(t *testing.T) {
	pdf, pages, err := makePDF("Test note", strings.Repeat("Alexander the Great expanded his empire. ", 200))
	if err != nil || pages < 2 {
		t.Fatalf("PDF generation failed: pages=%d err=%v", pages, err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Fatal("generated bytes are not a complete PDF")
	}
}

func TestAPIRequestAndErrors(t *testing.T) {
	cfg := config{APIKey: "test-key", Model: "test-model", Timezone: "America/Vancouver", SystemInstructions: defaultSystemInstructions, WebSearch: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "test-model" {
			t.Error("wrong model")
		}
		messages := body["messages"].([]any)
		prompt := messages[0].(map[string]any)["content"].(string)
		if !strings.HasPrefix(prompt, defaultSystemInstructions+"\n\n") || !strings.Contains(prompt, "Use web search") {
			t.Error("missing persona or search instructions")
		}
		tool := body["tools"].([]any)[0].(map[string]any)
		if tool["type"] != "openrouter:web_search" || body["max_tool_calls"] != float64(2) {
			t.Error("missing bounded server search tool")
		}
		parts := messages[1].(map[string]any)["content"].([]any)
		url := parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
		if url != "data:image/png;base64,cG5n" {
			t.Errorf("wrong image %s", url)
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"It is Friday."}}]}`)
	}))
	defer server.Close()
	got, err := askModel(context.Background(), server.Client(), server.URL, cfg, []byte("png"))
	if err != nil || got != "It is Friday." {
		t.Fatalf("%q %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := askModel(ctx, server.Client(), server.URL, cfg, []byte("png")); err == nil {
		t.Fatal("ignored cancellation")
	}
	for _, response := range []string{
		`{"error":{"message":"failure"}}`,
		`{"choices":[{"finish_reason":"length","message":{"content":"cut off"}}]}`,
		`{"choices":[{"finish_reason":"tool_calls","message":{"content":"I searched.","tool_calls":[{"function":{"name":"unknown"}}]}}]}`,
		`{"choices":[]}`, "invalid JSON",
	} {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response) }))
		_, err := askModel(context.Background(), bad.Client(), bad.URL, cfg, []byte("png"))
		bad.Close()
		if err == nil {
			t.Fatalf("accepted %s", response)
		}
	}
}

func TestDisabledSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["tools"]; ok {
			t.Error("search enabled despite setting")
		}
		messages := body["messages"].([]any)
		if !strings.Contains(messages[0].(map[string]any)["content"].(string), "Web search is disabled") {
			t.Error("prompt falsely advertises search")
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"Hello."}}]}`)
	}))
	defer server.Close()
	_, err := askModel(context.Background(), server.Client(), server.URL, config{Timezone: "UTC"}, []byte("png"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestPageChangeGuard(t *testing.T) {
	a := image.NewGray(image.Rect(0, 0, screenWidth, screenHeight))
	b := image.NewGray(a.Bounds())
	if pageChanged(a, b) {
		t.Fatal("same page changed")
	}
	b.Pix[500*screenWidth+500] = 255
	if pageChanged(a, b) {
		t.Fatal("single cursor pixel changed page")
	}
	for y := 300; y < 600; y++ {
		for x := 300; x < 600; x++ {
			b.Pix[y*screenWidth+x] = 255
		}
	}
	if !pageChanged(a, b) {
		t.Fatal("missed large change")
	}
}
