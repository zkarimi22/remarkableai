package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func setStatus(message string) {
	log.Print(message)
	// This contains operational status only, never keys, screenshots or answers.
	_ = os.WriteFile("/run/remarkableai-status", []byte(time.Now().Format(time.RFC3339)+" "+message+"\n"), 0600)
}

func readConfig(path string) (config, error) {
	var cfg config
	info, err := os.Stat(path)
	if err != nil {
		return cfg, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return cfg, fmt.Errorf("config must have permissions 600")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid configuration JSON")
	}
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.APIKey == "" {
		return cfg, fmt.Errorf("missing API key")
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "America/Vancouver"
	}
	if cfg.SystemInstructions == "" {
		cfg.SystemInstructions = defaultSystemInstructions
	}
	if cfg.PDFFolderName == "" {
		cfg.PDFFolderName = "AI Notes"
	}
	if _, err = time.LoadLocation(cfg.Timezone); err != nil {
		return cfg, fmt.Errorf("invalid timezone")
	}
	if err = cfg.typingDefaults(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (cfg *config) typingDefaults() error {
	if cfg.TypingDelayMS == 0 {
		cfg.TypingDelayMS = 180
	}
	if cfg.LeadingLineBreaks == 0 {
		cfg.LeadingLineBreaks = 2
	}
	if cfg.TypingDelayMS < 100 || cfg.TypingDelayMS > 500 {
		return fmt.Errorf("typing_delay_ms must be 100..500")
	}
	if cfg.LeadingLineBreaks < 1 || cfg.LeadingLineBreaks > 6 {
		return fmt.Errorf("leading_line_breaks must be 1..6")
	}
	return nil
}

func replyText(cfg config, reply string) string {
	return strings.Repeat("\n", cfg.LeadingLineBreaks) + reply + "\n"
}

func replyPayload(cfg config, reply string, firstReply bool) string {
	if !firstReply {
		return reply + "\n"
	}
	return replyText(cfg, reply)
}

type replyUndo struct {
	Characters int
	Page       *image.Gray
}

type operationResult struct {
	Undo    *replyUndo
	WasUndo bool
	WasPDF  bool
	Err     error
}

func runReply(ctx context.Context, cfg config, directory string, firstReply bool) (*replyUndo, error) {
	setStatus("Capturing page")
	png, _, err := capturePage()
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	setStatus("Positioning answer")
	command := exec.CommandContext(ctx, "/home/root/ghostwriter-test",
		"--engine", "openai", "--engine-api-key", "local-test",
		"--engine-base-url", "http://127.0.0.1:9",
		"--input-png", filepath.Join(directory, "placeholder.png"),
		"--no-submit", "--no-svg", "--no-draw-progress", "--no-trigger", "--no-loop")
	var positioningError bytes.Buffer
	command.Stderr = &positioningError
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		detail := strings.ReplaceAll(strings.TrimSpace(positioningError.String()), cfg.APIKey, "[redacted]")
		if len(detail) > 2048 {
			detail = detail[len(detail)-2048:]
		}
		return nil, fmt.Errorf("cursor positioning failed: %w: %s", err, detail)
	}
	setStatus("Showing working indicator")
	leadingLines := 0
	if firstReply {
		leadingLines = cfg.LeadingLineBreaks
	}
	if err = typeText(ctx, cfg, directory, strings.Repeat("\n", leadingLines)+"...", false); err != nil {
		return nil, err
	}
	_, pendingPage, err := capturePage()
	if err != nil {
		return nil, err
	}
	setStatus("Asking OpenRouter; touch or write to cancel")
	reply, err := askModel(ctx, apiClient(), apiURL, cfg, png)
	if err != nil {
		// A failed request must not leave a permanent 'working' signal unless
		// the user moved/edited the page, in which case never touch its text.
		if ctx.Err() == nil {
			_, current, captureErr := capturePage()
			if captureErr == nil && !pageChanged(pendingPage, current) {
				_ = typeText(ctx, cfg, directory, "Request failed. Please try again.\n", true)
			}
		}
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if strings.HasPrefix(reply, pdfNoteMarker) {
		parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(reply, pdfNoteMarker)), "\n", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("PDF note response was malformed")
		}
		title := strings.TrimSpace(parts[0])
		body := strings.TrimSpace(parts[1])
		if len(title) < 1 || len(title) > 100 || len(body) < 1 {
			return nil, fmt.Errorf("PDF note response was empty")
		}
		if err = clearPending(ctx, cfg, directory); err != nil {
			return nil, err
		}
		setStatus("Creating PDF note in AI Notes")
		if err = createPDFNote(cfg.PDFFolderName, title, body); err != nil {
			return nil, err
		}
		setStatus("PDF note created in AI Notes")
		return nil, nil
	}
	_, after, err := capturePage()
	if err != nil {
		return nil, err
	}
	if pageChanged(pendingPage, after) {
		return nil, fmt.Errorf("page changed; reply cancelled (working dots left in place)")
	}
	setStatus("Typing answer; touch or write to stop")
	payload := replyPayload(cfg, reply, firstReply)
	if err = typeText(ctx, cfg, directory, reply+"\n", true); err != nil {
		return nil, err
	}
	_, page, err := capturePage()
	if err != nil {
		return nil, fmt.Errorf("reply typed, but undo snapshot unavailable: %w", err)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return &replyUndo{Characters: len(payload), Page: page}, nil
}

func typeText(ctx context.Context, cfg config, directory, text string, replacePending bool) error {
	args := []string{"--delay-ms", strconv.Itoa(cfg.TypingDelayMS)}
	if replacePending {
		args = append(args, "--replace-pending")
	}
	typer := exec.CommandContext(ctx, filepath.Join(directory, "type-text-rm2"), args...)
	typer.Cancel = func() error { return typer.Process.Signal(syscall.SIGTERM) }
	typer.WaitDelay = 3 * time.Second
	typer.Stdin = strings.NewReader(text)
	if err := typer.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("typing stopped: %w", err)
	}
	return ctx.Err()
}

func clearPending(ctx context.Context, cfg config, directory string) error {
	typer := exec.CommandContext(ctx, filepath.Join(directory, "type-text-rm2"), "--delay-ms", strconv.Itoa(cfg.TypingDelayMS), "--clear-pending")
	typer.Cancel = func() error { return typer.Process.Signal(syscall.SIGTERM) }
	typer.WaitDelay = 3 * time.Second
	if err := typer.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("could not clear working indicator: %w", err)
	}
	return ctx.Err()
}

func runUndo(ctx context.Context, directory string, record *replyUndo) error {
	if record == nil {
		return fmt.Errorf("no unchanged AI reply available to undo")
	}
	_, current, err := capturePage()
	if err != nil {
		return err
	}
	if pageChanged(record.Page, current) {
		return fmt.Errorf("page changed; undo skipped")
	}
	setStatus("Removing last AI reply; touch or write to stop")
	command := exec.CommandContext(ctx, filepath.Join(directory, "type-text-rm2"), "--erase", strconv.Itoa(record.Characters))
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 3 * time.Second
	if err = command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("undo stopped: %w", err)
	}
	return nil
}

func main() {
	configPath := flag.String("config", "/home/root/.config/remarkableai/config.json", "configuration file")
	capture := flag.String("capture", "", "save a diagnostic screenshot and exit (no model call)")
	once := flag.Bool("once", false, "answer once immediately instead of waiting for double-tap")
	delay := flag.Int("set-typing-delay", 0, "save delay between keys in milliseconds (100..500), then exit")
	lineBreaks := flag.Int("set-leading-lines", 0, "save line breaks before each reply (1..6), then exit")
	model := flag.String("set-model", "", "save OpenRouter model ID, then exit")
	persona := flag.String("set-system-instructions", "", "save system instructions, then exit")
	webSearch := flag.String("set-web-search", "", "save web search on/off, then exit")
	flag.Parse()
	if *capture != "" {
		png, _, err := capturePage()
		if err != nil {
			log.Fatal(err)
		}
		if err = os.WriteFile(*capture, png, 0600); err != nil {
			log.Fatal(err)
		}
		return
	}
	cfg, err := readConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if *delay != 0 || *lineBreaks != 0 || *model != "" || *persona != "" || *webSearch != "" {
		if *model != "" {
			cfg.Model = *model
		}
		if *persona != "" {
			cfg.SystemInstructions = *persona
		}
		if *webSearch != "" {
			if *webSearch != "on" && *webSearch != "off" {
				log.Fatal("web search must be on or off")
			}
			cfg.WebSearch = *webSearch == "on"
		}
		if *delay != 0 {
			cfg.TypingDelayMS = *delay
		}
		if *lineBreaks != 0 {
			cfg.LeadingLineBreaks = *lineBreaks
		}
		if err = cfg.typingDefaults(); err != nil {
			log.Fatal(err)
		}
		encoded, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		file, err := os.CreateTemp(filepath.Dir(*configPath), ".settings-*")
		if err != nil {
			log.Fatal(err)
		}
		name := file.Name()
		_, writeErr := file.Write(encoded)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(name)
			log.Fatal("could not save settings")
		}
		if err = os.Rename(name, *configPath); err != nil {
			_ = os.Remove(name)
			log.Fatal(err)
		}
		fmt.Printf("Saved: body text, %d leading line breaks, %d ms typing delay. Restart remarkableai to apply.\n", cfg.LeadingLineBreaks, cfg.TypingDelayMS)
		return
	}
	binary, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	directory := filepath.Dir(binary)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if *once {
		if _, err = runReply(ctx, cfg, directory, true); err != nil {
			log.Fatal(err)
		}
		setStatus("Answer saved by notebook application")
		return
	}
	touchPath, err := findInputDevice("pt_mt")
	if err != nil {
		log.Fatal(err)
	}
	penPath, err := findInputDevice("Wacom")
	if err != nil {
		log.Fatal(err)
	}
	input := make(chan inputSignal, 64)
	inputErrors := make(chan error, 2)
	go readInput(touchPath, false, input, inputErrors)
	go readInput(penPath, true, input, inputErrors)
	done := make(chan operationResult, 1)
	var undo *replyUndo
	pageHasAssistantReply := false
	var cancel context.CancelFunc
	busy := false
	interrupted := false
	cooldown := time.Now().Add(time.Second)
	setStatus("Ready: lower-right double-tap asks; inset lower-left double-tap undoes latest unchanged reply")
	for {
		select {
		case <-ctx.Done():
			if cancel != nil {
				cancel()
				<-done
			}
			setStatus("Stopped")
			return
		case err := <-inputErrors:
			if cancel != nil {
				cancel()
				<-done
			}
			log.Fatal(err)
		case event := <-input:
			if busy {
				if event.Activity {
					interrupted = true
					cancel()
				}
				continue
			}
			if event.Activity && !event.UndoZone {
				undo = nil
			}
			if event.Undo && time.Now().After(cooldown) {
				if undo == nil {
					setStatus("Undo unavailable: no completed reply, or page was edited/navigated")
					continue
				}
				record := undo
				undo = nil // one use only, including interrupted/failed deletion
				var requestCtx context.Context
				requestCtx, cancel = context.WithCancel(ctx)
				busy = true
				interrupted = false
				go func() { done <- operationResult{WasUndo: true, Err: runUndo(requestCtx, directory, record)} }()
				continue
			}
			if event.Trigger && time.Now().After(cooldown) {
				var requestCtx context.Context
				requestCtx, cancel = context.WithCancel(ctx)
				busy = true
				interrupted = false
				go func() {
					record, err := runReply(requestCtx, cfg, directory, !pageHasAssistantReply)
					done <- operationResult{Undo: record, WasPDF: err == nil && record == nil, Err: err}
				}()
			}
		case result := <-done:
			err := result.Err
			if interrupted && err == nil {
				err = context.Canceled
			}
			cancelled := false
			if cancel != nil {
				// runReply wraps process errors; cancellation can also show as signal killed.
				cancelled = errors.Is(err, context.Canceled)
				cancel()
				cancel = nil
			}
			busy = false
			cooldown = time.Now().Add(2 * time.Second)
			if err != nil {
				if cancelled {
					setStatus("Cancelled; ready for next double-tap")
				} else {
					setStatus("Request stopped: " + err.Error())
				}
			} else {
				if result.WasUndo {
					pageHasAssistantReply = false
					setStatus("Last reply removed; ready for next question")
				} else {
					undo = result.Undo
					pageHasAssistantReply = true
					if result.WasPDF {
						setStatus("PDF note created in AI Notes")
					} else {
						setStatus("Reply finished; inset lower-left double-tap can undo it")
					}
				}
			}
		}
	}
}
