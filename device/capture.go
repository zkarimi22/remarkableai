package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const screenWidth, screenHeight = 1404, 1872

// Firmware 3.24+ rM2 layout, also used by goMarkableStream v1.4.0.
// Read the current xochitl process each time so restarts do not leave stale pointers.
func framebufferAddress(maps string) (int64, error) {
	lines := strings.Split(maps, "\n")
	for i, line := range lines {
		if strings.HasSuffix(strings.TrimSpace(line), "/dev/fb0") && i+1 < len(lines) {
			fields := strings.Fields(lines[i+1])
			if len(fields) == 0 {
				break
			}
			base, err := strconv.ParseInt(strings.Split(fields[0], "-")[0], 16, 64)
			if err != nil {
				return 0, err
			}
			return base + 2629632 + 8, nil
		}
	}
	return 0, fmt.Errorf("screen memory mapping not found")
}

func capturePage() ([]byte, *image.Gray, error) {
	release, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return nil, nil, err
	}
	supported := false
	for _, line := range strings.Split(string(release), "\n") {
		if strings.HasPrefix(line, "IMG_VERSION=") {
			parts := strings.Split(strings.Trim(strings.TrimPrefix(line, "IMG_VERSION="), "\""), ".")
			if len(parts) >= 2 {
				minor, _ := strconv.Atoi(parts[1])
				supported = parts[0] == "3" && minor >= 24
			}
		}
	}
	if !supported {
		return nil, nil, fmt.Errorf("capture requires rM2 firmware 3.24+ in the 3.x series")
	}
	output, err := exec.Command("pidof", "xochitl").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("notebook application is not running")
	}
	pids := strings.Fields(string(output))
	if len(pids) == 0 {
		return nil, nil, fmt.Errorf("no notebook process")
	}
	maps, err := os.ReadFile("/proc/" + pids[0] + "/maps")
	if err != nil {
		return nil, nil, err
	}
	address, err := framebufferAddress(string(maps))
	if err != nil {
		return nil, nil, err
	}
	memory, err := os.Open("/proc/" + pids[0] + "/mem")
	if err != nil {
		return nil, nil, err
	}
	defer memory.Close()
	raw := make([]byte, screenWidth*screenHeight*4)
	if _, err = memory.ReadAt(raw, address); err != nil {
		return nil, nil, err
	}
	img := image.NewGray(image.Rect(0, 0, screenWidth, screenHeight))
	for i := range img.Pix {
		b, g, r := uint32(raw[i*4]), uint32(raw[i*4+1]), uint32(raw[i*4+2])
		img.Pix[i] = byte((299*r + 587*g + 114*b) / 1000)
	}
	var out bytes.Buffer
	if err = png.Encode(&out, img); err != nil {
		return nil, nil, err
	}
	return out.Bytes(), img, nil
}

// A second guard against navigation/scrolling during the request. Input events
// provide the primary cancellation; this ignores small cursor blinks and the toolbar.
func pageChanged(a, b *image.Gray) bool {
	if a.Bounds() != b.Bounds() {
		return true
	}
	changed, total := 0, 0
	for y := 100; y < screenHeight-100; y += 3 {
		for x := 120; x < screenWidth-100; x += 3 {
			d := int(a.GrayAt(x, y).Y) - int(b.GrayAt(x, y).Y)
			if d < -32 || d > 32 {
				changed++
			}
			total++
		}
	}
	return changed*1000 > total*3
}
