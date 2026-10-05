package chat

import (
	"fmt"
	"strings"

	"github.com/mtariq99/retrievion/internal/tokenizer"
	"github.com/mtariq99/retrievion/models"
)

type TokenCounter func(s string) int

func BuildContext(results []models.RetrievedChunk, maxTokens int, counter tokenizer.Counter) (string, int) {
	if len(results) == 0 || maxTokens <= 0 {
		return "", len(results)
	}

	var builder strings.Builder

	used := 0
	included := 0

	for i, result := range results {
		var entry strings.Builder

		entry.WriteString(fmt.Sprintf("[Source %d]\n", i+1))

		if result.DocumentID != [16]byte{} {
			entry.WriteString(
				fmt.Sprintf("Document ID: %s\n", result.DocumentID),
			)
		}

		if result.Page != nil {
			entry.WriteString(
				fmt.Sprintf("Page: %v\n", *result.Page),
			)
		}

		if result.Section != nil {
			entry.WriteString(
				fmt.Sprintf("Section: %v\n", *result.Section),
			)
		}

		entry.WriteString("Content:\n")
		entry.WriteString(result.Content)
		entry.WriteString("\n\n")

		entryText := entry.String()
		entryTokens := counter.Count(entryText)

		if used+entryTokens > maxTokens {
			break
		}

		builder.WriteString(entryText)

		used += entryTokens
		included++
	}

	return builder.String(), len(results) - included
}
