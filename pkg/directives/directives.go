package directives

import (
	"regexp"
	"strings"
)

var containerLine = regexp.MustCompile(`^(>|[-+*][ \t]|[0-9]+[.)][ \t]|<)`)
var indentedLine = regexp.MustCompile(`^( {4}| {0,3}\t)`)
var htmlCodeBlock = regexp.MustCompile(`^<(pre|script|style|textarea)([ \t>]|$)`)

// HasCodeownersApproval recognizes an unformatted, standalone directive line.
// Quotes, lists and HTML paragraphs must end with a blank line before a directive.
func HasCodeownersApproval(body string) bool {
	var fence byte
	fenceLength := 0
	blockedParagraph := false
	inComment := false
	htmlEnd := ""
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.Trim(raw, " \t")
		lower := strings.ToLower(line)
		if fence != 0 {
			run := len(line) - len(strings.TrimLeft(line, string(fence)))
			if run >= fenceLength && strings.Trim(line[run:], " \t") == "" &&
				!indentedLine.MatchString(raw) {
				fence = 0
			}
			continue
		}
		if htmlEnd == "" && !inComment {
			if match := htmlCodeBlock.FindStringSubmatch(lower); match != nil {
				htmlEnd = "</" + match[1] + ">"
			}
		}
		if htmlEnd != "" {
			if strings.Contains(lower, htmlEnd) {
				htmlEnd = ""
			}
			continue
		}
		if inComment || strings.Contains(line, "<!--") {
			inComment = !strings.Contains(line, "-->")
			blockedParagraph = true
			continue
		}
		if line == "" {
			blockedParagraph = false
			continue
		}
		if indentedLine.MatchString(raw) {
			continue
		}
		if containerLine.MatchString(line) {
			blockedParagraph = true
		}
		if blockedParagraph {
			continue
		}
		if line[0] == '`' || line[0] == '~' {
			run := len(line) - len(strings.TrimLeft(line, line[:1]))
			if run >= 3 && (line[0] == '~' || !strings.Contains(line[run:], "`")) {
				fence = line[0]
				fenceLength = run
				continue
			}
		}
		if lower == "codeowners-approved" {
			return true
		}
	}
	return false
}
