package knowledge

import (
	"strings"
	"unicode/utf8"
)

// Chunk is a section of a larger document.
type Chunk struct {
	Text   string
	Start  int
	End    int
	Source string
}

// Chunker splits text into chunks ready for embedding.
type Chunker interface {
	Chunk(text string) []Chunk
}

// ─── FixedSize ───────────────────────────────────────────────────────────────

type fixedSize struct{ maxRunes int }

// FixedSize returns a Chunker that splits on whitespace boundaries so each
// chunk contains at most maxRunes Unicode code-points.
func FixedSize(maxRunes int) Chunker { return fixedSize{maxRunes: maxRunes} }

func (c fixedSize) Chunk(text string) []Chunk {
	words := strings.Fields(text)
	var chunks []Chunk
	var buf strings.Builder
	for _, w := range words {
		if buf.Len() > 0 && utf8.RuneCountInString(buf.String())+1+utf8.RuneCountInString(w) > c.maxRunes {
			chunks = append(chunks, Chunk{Text: buf.String()})
			buf.Reset()
		}
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(w)
	}
	if buf.Len() > 0 {
		chunks = append(chunks, Chunk{Text: buf.String()})
	}
	return chunks
}

// ─── Paragraph ───────────────────────────────────────────────────────────────

type paragraph struct{}

// Paragraph returns a Chunker that splits on blank lines (\n\n).
func Paragraph() Chunker { return paragraph{} }

func (paragraph) Chunk(text string) []Chunk {
	parts := strings.Split(text, "\n\n")
	var chunks []Chunk
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			chunks = append(chunks, Chunk{Text: p})
		}
	}
	return chunks
}

// ─── Recursive ───────────────────────────────────────────────────────────────

type recursive struct {
	maxRunes     int
	overlapRunes int
}

// Recursive returns a Chunker that tries to split on paragraph boundaries first,
// then newlines, then sentences, then spaces, until each chunk fits in maxRunes.
func Recursive(maxRunes, overlapRunes int) Chunker {
	return recursive{maxRunes: maxRunes, overlapRunes: overlapRunes}
}

var separators = []string{"\n\n", "\n", ". ", " "}

func (c recursive) Chunk(text string) []Chunk {
	var raw []string
	splitRecursive(text, c.maxRunes, separators, &raw)
	cooked := make([]Chunk, len(raw))
	for i, t := range raw {
		cooked[i] = Chunk{Text: t}
	}
	if c.overlapRunes > 0 {
		cooked = addOverlap(cooked, c.overlapRunes)
	}
	return cooked
}

func splitRecursive(text string, maxRunes int, seps []string, out *[]string) {
	if utf8.RuneCountInString(text) <= maxRunes {
		t := strings.TrimSpace(text)
		if t != "" {
			*out = append(*out, t)
		}
		return
	}
	if len(seps) == 0 {
		*out = append(*out, text)
		return
	}
	parts := strings.Split(text, seps[0])
	for _, p := range parts {
		splitRecursive(p, maxRunes, seps[1:], out)
	}
}

func addOverlap(chunks []Chunk, overlapRunes int) []Chunk {
	if len(chunks) <= 1 {
		return chunks
	}
	out := make([]Chunk, len(chunks))
	for i, ch := range chunks {
		if i == 0 {
			out[i] = ch
			continue
		}
		prev := []rune(chunks[i-1].Text)
		var overlap string
		if len(prev) > overlapRunes {
			overlap = string(prev[len(prev)-overlapRunes:])
		} else {
			overlap = string(prev)
		}
		out[i] = Chunk{Text: overlap + " " + ch.Text}
	}
	return out
}
