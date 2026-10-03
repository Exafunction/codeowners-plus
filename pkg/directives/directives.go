package directives

import (
	"regexp"
	"strings"
)

var containerLine = regexp.MustCompile(`^(>|[-+*][ \t]|[0-9]+[.)][ \t]|<)`)
var listLine = regexp.MustCompile(`^( {0,3}(?:[-+*]|[0-9]{1,9}[.)]))([ \t]+|$)`)
var indentedLine = regexp.MustCompile(`^( {4}| {0,3}\t)`)
var htmlCodeBlock = regexp.MustCompile(`^<(pre|script|style|textarea)([ \t>]|$)`)

// HasCodeownersApproval recognizes an unformatted, standalone directive line.
// Quotes, lists and HTML paragraphs must end with a blank line before a directive.
func HasCodeownersApproval(body string) bool {
	var fence byte
	fenceLength := 0
	blockedParagraph := false
	listIndent := 0
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
			if listIndent > 0 {
				blockedParagraph = true
			}
			continue
		}
		if listIndent > 0 {
			if len(raw)-len(strings.TrimLeft(raw, " ")) >= listIndent {
				blockedParagraph = true
				continue
			}
			listIndent = 0
		}
		if match := listLine.FindStringSubmatch(raw); match != nil {
			listIndent = len(match[1])
			for _, char := range match[2] {
				if char == '\t' {
					listIndent += 4 - listIndent%4
				} else {
					listIndent++
				}
			}
			if listIndent == len(match[1]) || listIndent-len(match[1]) > 4 || len(match[0]) == len(raw) {
				listIndent = len(match[1]) + 1
			}
			blockedParagraph = true
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
