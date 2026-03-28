package content_test

import (
	"testing"

	"github.com/daneel-ai/daneel/content"
)

func TestTextContent(t *testing.T) {
	c := content.TextContent("hello")
	if c.Type != content.ContentText {
		t.Fatalf("type = %v, want text", c.Type)
	}
	if c.Text != "hello" {
		t.Fatalf("text = %q", c.Text)
	}
	if len(c.Data) != 0 {
		t.Fatal("data should be empty")
	}
}

func TestImageContent(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47}
	c := content.ImageContent(data, "image/png")
	if c.Type != content.ContentImage {
		t.Fatalf("type = %v, want image", c.Type)
	}
	if c.MimeType != "image/png" {
		t.Fatalf("mime = %q", c.MimeType)
	}
	if len(c.Data) != 4 {
		t.Fatalf("data len = %d", len(c.Data))
	}
	if c.URL != "" {
		t.Fatal("url should be empty")
	}
}

func TestImageURLContent(t *testing.T) {
	c := content.ImageURLContent("https://example.com/img.jpg")
	if c.Type != content.ContentImage {
		t.Fatalf("type = %v, want image", c.Type)
	}
	if c.URL != "https://example.com/img.jpg" {
		t.Fatalf("url = %q", c.URL)
	}
	if c.MimeType != "image/jpeg" {
		t.Fatalf("mime = %q, want image/jpeg", c.MimeType)
	}
	if len(c.Data) != 0 {
		t.Fatal("data should be empty for URL content")
	}
}

func TestAudioContent(t *testing.T) {
	data := []byte{0xFF, 0xFB}
	c := content.AudioContent(data, "audio/mp3")
	if c.Type != content.ContentAudio {
		t.Fatalf("type = %v, want audio", c.Type)
	}
	if c.MimeType != "audio/mp3" {
		t.Fatalf("mime = %q", c.MimeType)
	}
	if len(c.Data) != 2 {
		t.Fatal("data len should be 2")
	}
}

func TestFileContent(t *testing.T) {
	data := []byte("file contents here")
	c := content.FileContent(data, "report.pdf", "application/pdf")
	if c.Type != content.ContentFile {
		t.Fatalf("type = %v, want file", c.Type)
	}
	if c.Filename != "report.pdf" {
		t.Fatalf("filename = %q", c.Filename)
	}
	if c.MimeType != "application/pdf" {
		t.Fatalf("mime = %q", c.MimeType)
	}
	if string(c.Data) != "file contents here" {
		t.Fatalf("data = %q", string(c.Data))
	}
}

func TestContentTypeConstants(t *testing.T) {
	if content.ContentText != "text" {
		t.Fatalf("ContentText = %q", content.ContentText)
	}
	if content.ContentImage != "image" {
		t.Fatalf("ContentImage = %q", content.ContentImage)
	}
	if content.ContentAudio != "audio" {
		t.Fatalf("ContentAudio = %q", content.ContentAudio)
	}
	if content.ContentFile != "file" {
		t.Fatalf("ContentFile = %q", content.ContentFile)
	}
}
