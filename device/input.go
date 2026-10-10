package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type inputSignal struct {
	Trigger  bool
	Undo     bool
	UndoZone bool
	Activity bool
}
type finger struct {
	x, y              int
	down, ended, seen bool
	started           time.Time
}
type touchState struct {
	slot     int
	fingers  [16]finger
	changed  bool
	lastTap  time.Time
	lastZone int
}

func inUndoZone(x, y int) bool {
	// About 2 cm right of the left screen edge and 2 cm above the bottom,
	// with a finger-sized target that avoids the native toolbar.
	return x >= 130 && x <= 250 && y >= 110 && y <= 250
}

func gestureZone(x, y int) int {
	if inTriggerCorner(x, y) {
		return 1
	}
	if inUndoZone(x, y) {
		return 2
	}
	return 0
}

func inTriggerCorner(x, y int) bool {
	// rM2 touch y increases upwards, unlike the portrait screenshot.
	return x >= 1264 && x <= 1404 && y >= 0 && y <= 140
}

func (s *touchState) event(kind, code uint16, value int32, now time.Time) inputSignal {
	if kind == 3 { // EV_ABS
		if code == 47 {
			s.slot = int(value)
			return inputSignal{}
		}
		if s.slot < 0 || s.slot >= len(s.fingers) {
			return inputSignal{}
		}
		f := &s.fingers[s.slot]
		switch code {
		case 53:
			f.x = int(value)
			s.changed = true
		case 54:
			f.y = int(value)
			s.changed = true
		case 57:
			if value >= 0 {
				f.down = true
				f.seen = true
				f.started = now
			} else {
				f.ended = f.down
				f.down = false
			}
			s.changed = true
		}
	}
	if kind != 0 || code != 0 || !s.changed {
		return inputSignal{}
	}
	s.changed = false
	out := inputSignal{UndoZone: true}
	for i := range s.fingers {
		f := &s.fingers[i]
		if !f.seen || (!f.down && !f.ended) {
			continue
		}
		// Ghostwriter's tested cursor-positioning tap writes this exact coordinate.
		syntheticFocus := f.x == 702 && f.y >= 0 && f.y <= 3
		if !syntheticFocus {
			out.Activity = true
			if !inUndoZone(f.x, f.y) {
				out.UndoZone = false
			}
		}
		if f.ended {
			f.ended = false
			zone := gestureZone(f.x, f.y)
			if !syntheticFocus && now.Sub(f.started) <= 450*time.Millisecond && zone != 0 {
				if !s.lastTap.IsZero() && s.lastZone == zone && now.Sub(s.lastTap) <= 900*time.Millisecond {
					out.Trigger = zone == 1
					out.Undo = zone == 2
					s.lastTap = time.Time{}
					s.lastZone = 0
				} else {
					s.lastTap = now
					s.lastZone = zone
				}
			} else {
				s.lastTap = time.Time{}
				s.lastZone = 0
			}
		}
	}
	if !out.Activity {
		out.UndoZone = false
	}
	return out
}

func findInputDevice(name string) (string, error) {
	paths, err := filepath.Glob("/sys/class/input/event*/device/name")
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		value, err := os.ReadFile(path)
		if err == nil && strings.Contains(strings.TrimSpace(string(value)), name) {
			return "/dev/input/" + filepath.Base(filepath.Dir(filepath.Dir(path))), nil
		}
	}
	return "", fmt.Errorf("input device %q not found", name)
}

func readInput(path string, pen bool, out chan<- inputSignal, errors chan<- error) {
	f, err := os.Open(path)
	if err != nil {
		errors <- err
		return
	}
	defer f.Close()
	size := 24
	if runtime.GOARCH == "arm" || runtime.GOARCH == "386" {
		size = 16
	}
	raw := make([]byte, size)
	var state touchState
	for {
		if _, err = io.ReadFull(f, raw); err != nil {
			errors <- err
			return
		}
		base := size - 8
		kind := binary.LittleEndian.Uint16(raw[base:])
		code := binary.LittleEndian.Uint16(raw[base+2:])
		value := int32(binary.LittleEndian.Uint32(raw[base+4:]))
		event := inputSignal{}
		if pen {
			if kind == 1 && code == 330 && value == 1 {
				event.Activity = true
			}
		} else {
			if kind == 0 && code == 3 { // SYN_DROPPED: cancel instead of trusting stale state.
				state = touchState{}
				event.Activity = true
			} else {
				event = state.event(kind, code, value, time.Now())
			}
		}
		if event.Activity || event.Trigger {
			select {
			case out <- event:
			default:
			}
		}
	}
}
