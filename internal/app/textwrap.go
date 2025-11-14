package app

import "strings"

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		lines = append(lines, wrapParagraph(paragraph, width)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func wrapParagraph(paragraph string, width int) []string {
	words := strings.Fields(paragraph)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var current []string
	length := 0
	for _, word := range words {
		wordLen := len(word)
		if len(current) == 0 {
			current = append(current, word)
			length = wordLen
			continue
		}
		if length+1+wordLen <= width {
			current = append(current, word)
			length += 1 + wordLen
			continue
		}
		lines = append(lines, strings.Join(current, " "))
		current = []string{word}
		length = wordLen
	}
	if len(current) > 0 {
		lines = append(lines, strings.Join(current, " "))
	}
	return lines
}
