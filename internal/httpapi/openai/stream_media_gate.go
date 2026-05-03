package openai

import (
	"context"
	"strings"
)

type streamMediaGate struct {
	pending string
}

func (g *streamMediaGate) push(ctx context.Context, rewriter *mediaRewriter, text string) (string, error) {
	if text == "" {
		return "", nil
	}
	g.pending += text
	cut := streamingSafeFlushIndex(g.pending)
	out, err := rewriteContent(rewriter, ctx, g.pending[:cut])
	if err != nil {
		return "", err
	}
	g.pending = g.pending[cut:]
	return out, nil
}

func (g *streamMediaGate) flush(ctx context.Context, rewriter *mediaRewriter) (string, error) {
	if g.pending == "" {
		return "", nil
	}
	if err := g.rewrite(ctx, rewriter); err != nil {
		return "", err
	}
	out := g.pending
	g.pending = ""
	return out, nil
}

func (g *streamMediaGate) rewrite(ctx context.Context, rewriter *mediaRewriter) error {
	rewritten, err := rewriteContent(rewriter, ctx, g.pending)
	if err != nil {
		return err
	}
	g.pending = rewritten
	return nil
}

func streamingSafeFlushIndex(text string) int {
	lower := strings.ToLower(text)
	hold := len(text)
	for _, start := range []int{
		markdownImageHoldStart(text),
		activeAbsoluteURLStart(lower),
		activeRelativePathStart(lower),
		partialSuffixStart(lower, "https://"),
		partialSuffixStart(lower, "http://"),
		partialSuffixStart(lower, "users/"),
		partialSuffixStart(lower, "/users/"),
	} {
		if start >= 0 && start < hold {
			hold = start
		}
	}
	return hold
}

func markdownImageHoldStart(text string) int {
	if start := incompleteMarkdownImageStart(text); start >= 0 {
		return start
	}
	if strings.HasSuffix(text, "!") {
		return len(text) - 1
	}
	return -1
}

func incompleteMarkdownImageStart(text string) int {
	for search := 0; search < len(text); {
		start := strings.Index(text[search:], "![")
		if start < 0 {
			return -1
		}
		start += search
		closeBracket := strings.IndexByte(text[start+2:], ']')
		if closeBracket < 0 {
			return start
		}
		closeBracket += start + 2
		if closeBracket+1 >= len(text) {
			return start
		}
		if text[closeBracket+1] != '(' {
			search = start + 2
			continue
		}
		closeParen := strings.IndexByte(text[closeBracket+2:], ')')
		if closeParen < 0 {
			return start
		}
		search = closeBracket + 2 + closeParen + 1
	}
	return -1
}

func activeAbsoluteURLStart(text string) int {
	return activeDelimitedStart(text, []string{"https://", "http://"})
}

func activeRelativePathStart(text string) int {
	return activeDelimitedStart(text, []string{"users/", "/users/"})
}

func activeDelimitedStart(text string, markers []string) int {
	hold := -1
	for _, marker := range markers {
		for start := strings.Index(text, marker); start >= 0; {
			if !hasDelimiter(text[start:]) {
				hold = minNonNegative(hold, start)
			}
			next := start + len(marker)
			found := strings.Index(text[next:], marker)
			if found < 0 {
				break
			}
			start = next + found
		}
	}
	return hold
}

const streamMediaDelimiters = "'\" )<>\n\r\t"

func hasDelimiter(text string) bool {
	return strings.ContainsAny(text, streamMediaDelimiters)
}

func partialSuffixStart(text, marker string) int {
	return partialSuffixStartMin(text, marker, 1)
}

func partialSuffixStartMin(text, marker string, minLen int) int {
	maxLen := len(marker) - 1
	if len(text) < maxLen {
		maxLen = len(text)
	}
	for n := maxLen; n > 0; n-- {
		if n < minLen {
			break
		}
		if strings.HasSuffix(text, marker[:n]) {
			return len(text) - n
		}
	}
	return -1
}

func minNonNegative(current, next int) int {
	if current < 0 || next < current {
		return next
	}
	return current
}
