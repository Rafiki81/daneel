// Package content defines multi-modal content types used across Daneel.
//
// Providers map Content to their native format:
//   - OpenAI: content array with "text" and "image_url" parts
//   - Anthropic: content array with "text" and "image" (base64) parts
//   - Gemini: parts array with "text" and "inlineData"
package content

// ContentType indicates the kind of multi-modal content.
type ContentType string

const (
	ContentText  ContentType = "text"
	ContentImage ContentType = "image"
	ContentAudio ContentType = "audio"
	ContentFile  ContentType = "file"
)

// Content represents a piece of multi-modal content that can be included
// in a message. Exactly one of Text/Data/URL should be populated depending
// on the content type.
type Content struct {
	Type     ContentType // Text, Image, Audio, File
	Text     string      // for text content
	Data     []byte      // raw bytes for binary content
	MimeType string      // "image/png", "audio/wav", etc.
	URL      string      // remote URL (alternative to Data)
	Filename string      // optional filename
}

// TextContent creates a text Content.
func TextContent(text string) Content {
	return Content{
		Type: ContentText,
		Text: text,
	}
}

// ImageContent creates an image Content from raw bytes.
func ImageContent(data []byte, mimeType string) Content {
	return Content{
		Type:     ContentImage,
		Data:     data,
		MimeType: mimeType,
	}
}

// ImageURLContent creates an image Content from a remote URL.
func ImageURLContent(url string) Content {
	return Content{
		Type:     ContentImage,
		URL:      url,
		MimeType: "image/jpeg",
	}
}

// AudioContent creates an audio Content from raw bytes.
func AudioContent(data []byte, mimeType string) Content {
	return Content{
		Type:     ContentAudio,
		Data:     data,
		MimeType: mimeType,
	}
}

// FileContent creates a file Content from raw bytes.
func FileContent(data []byte, filename string, mimeType string) Content {
	return Content{
		Type:     ContentFile,
		Data:     data,
		Filename: filename,
		MimeType: mimeType,
	}
}
