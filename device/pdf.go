package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const xochitlDirectory = "/home/root/.local/share/remarkable/xochitl"

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[:8], s[8:12], s[12:16], s[16:20], s[20:]), nil
}

func pdfEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	return strings.ReplaceAll(s, ")", "\\)")
}

func wrapPDF(text string, width int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if len(line)+1+len(word) > width {
				lines = append(lines, line)
				line = word
			} else {
				line += " " + word
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func makePDF(title, body string) ([]byte, int, error) {
	lines := append([]string{title, ""}, wrapPDF(body, 88)...)
	const perPage = 48
	pageCount := (len(lines) + perPage - 1) / perPage
	if pageCount < 1 {
		pageCount = 1
	}
	var objects [][]byte
	objects = append(objects, []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	pageRefs := make([]string, pageCount)
	for i := 0; i < pageCount; i++ {
		pageRefs[i] = strconv.Itoa(4+i*2) + " 0 R"
	}
	objects = append(objects, []byte("<< /Type /Pages /Kids ["+strings.Join(pageRefs, " ")+" ] /Count "+strconv.Itoa(pageCount)+" >>"))
	objects = append(objects, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	for i := 0; i < pageCount; i++ {
		var content strings.Builder
		content.WriteString("BT /F1 11 Tf 50 770 Td 14 TL\n")
		start, end := i*perPage, (i+1)*perPage
		if end > len(lines) {
			end = len(lines)
		}
		for _, line := range lines[start:end] {
			content.WriteString("(" + pdfEscape(line) + ") Tj T*\n")
		}
		content.WriteString("ET")
		stream := []byte(content.String())
		objects = append(objects, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents "+strconv.Itoa(5+i*2)+" 0 R >>"))
		objects = append(objects, []byte("<< /Length "+strconv.Itoa(len(stream))+" >>\nstream\n"+string(stream)+"\nendstream"))
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes(), pageCount, nil
}

func folderUUID(name string) (string, error) {
	entries, err := filepath.Glob(filepath.Join(xochitlDirectory, "*.metadata"))
	if err != nil {
		return "", err
	}
	for _, path := range entries {
		var m struct {
			VisibleName string `json:"visibleName"`
			Type        string `json:"type"`
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		if json.Unmarshal(raw, &m) == nil && m.VisibleName == name && m.Type == "CollectionType" {
			return strings.TrimSuffix(filepath.Base(path), ".metadata"), nil
		}
	}
	return "", fmt.Errorf("folder %q was not found", name)
}

func createPDFNote(folderName, title, body string) error {
	folder, err := folderUUID(folderName)
	newFolder := err != nil
	if newFolder {
		folder, err = newUUID()
		if err != nil {
			return err
		}
	}
	pdf, pages, err := makePDF(title, body)
	if err != nil {
		return err
	}
	id, err := newUUID()
	if err != nil {
		return err
	}
	metadata := map[string]any{"deleted": false, "lastModified": strconv.FormatInt(time.Now().UnixMilli(), 10), "metadatamodified": false, "modified": false, "parent": folder, "pinned": false, "synced": false, "type": "DocumentType", "version": 1, "visibleName": title, "pageCount": pages, "sizeInBytes": len(pdf)}
	content := map[string]any{"fileType": "pdf", "pageCount": pages, "lastOpenedPage": 0, "visibleName": title, "parent": folder, "extraMetadata": map[string]any{}}
	metaBytes, _ := json.Marshal(metadata)
	contentBytes, _ := json.Marshal(content)
	if err = exec.Command("systemctl", "stop", "xochitl").Run(); err != nil {
		return fmt.Errorf("could not stop xochitl: %w", err)
	}
	started := false
	defer func() {
		if !started {
			_ = exec.Command("systemctl", "start", "xochitl").Run()
		}
	}()
	if newFolder {
		folderMetadata := map[string]any{"deleted": false, "lastModified": strconv.FormatInt(time.Now().UnixMilli(), 10), "metadatamodified": false, "modified": false, "parent": "", "pinned": false, "synced": false, "type": "CollectionType", "version": 1, "visibleName": folderName}
		folderBytes, _ := json.Marshal(folderMetadata)
		if err = os.WriteFile(filepath.Join(xochitlDirectory, folder+".metadata"), folderBytes, 0600); err != nil {
			return err
		}
	}
	for suffix, data := range map[string][]byte{".pdf": pdf, ".metadata": metaBytes, ".content": contentBytes} {
		tmp := filepath.Join(xochitlDirectory, ".remarkableai-"+id+suffix+".tmp")
		if err = os.WriteFile(tmp, data, 0600); err != nil {
			return err
		}
		if err = os.Rename(tmp, filepath.Join(xochitlDirectory, id+suffix)); err != nil {
			return err
		}
	}
	_ = os.MkdirAll(filepath.Join(xochitlDirectory, id+".cache"), 0700)
	_ = os.MkdirAll(filepath.Join(xochitlDirectory, id+".thumbnails"), 0700)
	_ = os.MkdirAll(filepath.Join(xochitlDirectory, id+".highlights"), 0700)
	if err = exec.Command("systemctl", "start", "xochitl").Run(); err != nil {
		return err
	}
	started = true
	return nil
}
