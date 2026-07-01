package artifacts

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"strings"

	"github.com/chranama/MealCheck/internal/workflow/checker"
)

func writeMarkdown(path string, report reportDocument, e checker.Evaluation) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# MealCheck Report\n\n")
	fmt.Fprintf(&b, "- Case: `%s`\n", report.CaseID)
	fmt.Fprintf(&b, "- Decision: `%s`\n", report.Decision)
	fmt.Fprintf(&b, "- Guideline pack: `%s`\n\n", report.GuidelinePackID)
	fmt.Fprintf(&b, "## Summary\n\n%s\n\n", report.Sections[0].Body)
	fmt.Fprintf(&b, "## Checks Requiring Attention\n\n")
	for _, check := range failedChecks(e.Checks) {
		fmt.Fprintf(&b, "- `%s` %s: %s\n", check.CheckID, check.Status, check.Message)
	}
	fmt.Fprintf(&b, "\n## Unresolved Foods\n\n")
	if len(e.UnresolvedItems) == 0 {
		fmt.Fprintf(&b, "None.\n")
	} else {
		for _, item := range e.UnresolvedItems {
			fmt.Fprintf(&b, "- Day %d %s: `%s` (%s)\n", item.Day, item.Meal, item.Food, item.UnresolvedReason)
		}
	}
	fmt.Fprintf(&b, "\n## Excluded Unresolved Foods\n\n")
	if len(e.ExcludedUnresolvedItems) == 0 {
		fmt.Fprintf(&b, "None.\n")
	} else {
		for _, item := range e.ExcludedUnresolvedItems {
			fmt.Fprintf(&b, "- Day %d %s: `%s` %.1f %s / %.1f g excluded (%s)\n", item.Day, item.Meal, item.Food, item.Quantity, item.Unit, item.DeterministicGrams, item.ExclusionReason)
		}
	}
	fmt.Fprintf(&b, "\n## Estimated And Decomposed Foods\n\n")
	if approximateResolutionCount(e.ResolvedItems) == 0 {
		fmt.Fprintf(&b, "None.\n")
	} else {
		for _, item := range e.ResolvedItems {
			if item.ResolutionMethod != "estimated" && item.ResolutionMethod != "decomposed" {
				continue
			}
			if item.ProxyFood != "" {
				fmt.Fprintf(&b, "- Day %d %s: `%s` resolved as `%s` (%s, %s)\n", item.Day, item.Meal, item.Food, item.ProxyFood, item.ResolutionMethod, item.Confidence)
				continue
			}
			fmt.Fprintf(&b, "- Day %d %s: `%s` resolved by %s (%s, %d components)\n", item.Day, item.Meal, item.Food, item.ResolutionMethod, item.Confidence, len(item.Components))
		}
	}
	fmt.Fprintf(&b, "\n## Daily Totals\n\n")
	for _, total := range e.DailyTotals {
		fmt.Fprintf(&b, "- Day %d: %.1f kcal, %.1f g protein, %.1f mg sodium\n", total.Day, total.Nutrients.EnergyKcal, total.Nutrients.ProteinG, total.Nutrients.SodiumMG)
	}
	fmt.Fprintf(&b, "\n## Disclaimer\n\n%s\n", report.Disclaimer)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeHTML(path string, report reportDocument, e checker.Evaluation) error {
	var md bytes.Buffer
	md.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>MealCheck Report</title></head><body>")
	fmt.Fprintf(&md, "<h1>MealCheck Report</h1><p><strong>Case:</strong> %s</p><p><strong>Decision:</strong> %s</p>", html.EscapeString(report.CaseID), html.EscapeString(report.Decision))
	fmt.Fprintf(&md, "<h2>Summary</h2><p>%s</p>", html.EscapeString(report.Sections[0].Body))
	md.WriteString("<h2>Checks Requiring Attention</h2><ul>")
	for _, check := range failedChecks(e.Checks) {
		fmt.Fprintf(&md, "<li><code>%s</code> %s: %s</li>", html.EscapeString(check.CheckID), html.EscapeString(check.Status), html.EscapeString(check.Message))
	}
	md.WriteString("</ul><h2>Unresolved Foods</h2><ul>")
	for _, item := range e.UnresolvedItems {
		fmt.Fprintf(&md, "<li>Day %d %s: <code>%s</code> (%s)</li>", item.Day, html.EscapeString(item.Meal), html.EscapeString(item.Food), html.EscapeString(item.UnresolvedReason))
	}
	md.WriteString("</ul><h2>Excluded Unresolved Foods</h2><ul>")
	for _, item := range e.ExcludedUnresolvedItems {
		fmt.Fprintf(&md, "<li>Day %d %s: <code>%s</code> %.1f %s / %.1f g excluded (%s)</li>", item.Day, html.EscapeString(item.Meal), html.EscapeString(item.Food), item.Quantity, html.EscapeString(item.Unit), item.DeterministicGrams, html.EscapeString(item.ExclusionReason))
	}
	md.WriteString("</ul><h2>Estimated And Decomposed Foods</h2><ul>")
	for _, item := range e.ResolvedItems {
		if item.ResolutionMethod != "estimated" && item.ResolutionMethod != "decomposed" {
			continue
		}
		if item.ProxyFood != "" {
			fmt.Fprintf(&md, "<li>Day %d %s: <code>%s</code> resolved as <code>%s</code> (%s, %s)</li>", item.Day, html.EscapeString(item.Meal), html.EscapeString(item.Food), html.EscapeString(item.ProxyFood), html.EscapeString(item.ResolutionMethod), html.EscapeString(item.Confidence))
			continue
		}
		fmt.Fprintf(&md, "<li>Day %d %s: <code>%s</code> resolved by %s (%s, %d components)</li>", item.Day, html.EscapeString(item.Meal), html.EscapeString(item.Food), html.EscapeString(item.ResolutionMethod), html.EscapeString(item.Confidence), len(item.Components))
	}
	md.WriteString("</ul><h2>Daily Totals</h2><ul>")
	for _, total := range e.DailyTotals {
		fmt.Fprintf(&md, "<li>Day %d: %.1f kcal, %.1f g protein, %.1f mg sodium</li>", total.Day, total.Nutrients.EnergyKcal, total.Nutrients.ProteinG, total.Nutrients.SodiumMG)
	}
	fmt.Fprintf(&md, "</ul><h2>Disclaimer</h2><p>%s</p></body></html>\n", html.EscapeString(report.Disclaimer))
	return os.WriteFile(path, md.Bytes(), 0o644)
}

func writePDF(path string, report reportDocument, e checker.Evaluation) error {
	lines := []string{
		"MealCheck Report",
		"Decision: " + report.Decision,
		"Guideline: " + report.GuidelinePackID,
		"",
		"Summary",
		report.Sections[0].Body,
		"",
		"Checks Requiring Attention",
	}
	failures := failedChecks(e.Checks)
	if len(failures) == 0 {
		lines = append(lines, "None.")
	} else {
		for _, check := range failures {
			lines = append(lines, fmt.Sprintf("%s: %s", strings.ToUpper(check.Status), check.Message))
		}
	}
	lines = append(lines, "", "Unresolved Foods")
	if len(e.UnresolvedItems) == 0 {
		lines = append(lines, "None.")
	} else {
		for _, item := range e.UnresolvedItems {
			lines = append(lines, fmt.Sprintf("Day %d %s: %s (%s)", item.Day, item.Meal, item.Food, item.UnresolvedReason))
		}
	}
	lines = append(lines, "", "Excluded Unresolved Foods")
	if len(e.ExcludedUnresolvedItems) == 0 {
		lines = append(lines, "None.")
	} else {
		for _, item := range e.ExcludedUnresolvedItems {
			lines = append(lines, fmt.Sprintf("Day %d %s: %s %.1f %s / %.1f g excluded (%s)", item.Day, item.Meal, item.Food, item.Quantity, item.Unit, item.DeterministicGrams, item.ExclusionReason))
		}
	}
	lines = append(lines, "", "Estimated And Decomposed Foods")
	if approximateResolutionCount(e.ResolvedItems) == 0 {
		lines = append(lines, "None.")
	} else {
		for _, item := range e.ResolvedItems {
			if item.ResolutionMethod != "estimated" && item.ResolutionMethod != "decomposed" {
				continue
			}
			if item.ProxyFood != "" {
				lines = append(lines, fmt.Sprintf("Day %d %s: %s resolved as %s (%s, %s)", item.Day, item.Meal, item.Food, item.ProxyFood, item.ResolutionMethod, item.Confidence))
				continue
			}
			lines = append(lines, fmt.Sprintf("Day %d %s: %s resolved by %s (%s, %d components)", item.Day, item.Meal, item.Food, item.ResolutionMethod, item.Confidence, len(item.Components)))
		}
	}
	lines = append(lines, "", "Daily Totals")
	for _, total := range e.DailyTotals {
		lines = append(lines, fmt.Sprintf("Day %d: %.1f kcal, %.1f g protein, %.1f mg sodium", total.Day, total.Nutrients.EnergyKcal, total.Nutrients.ProteinG, total.Nutrients.SodiumMG))
	}
	lines = append(lines, "", "Disclaimer", report.Disclaimer)
	return os.WriteFile(path, simplePDF(lines), 0o644)
}

func simplePDF(lines []string) []byte {
	var stream bytes.Buffer
	stream.WriteString("BT\n/F1 12 Tf\n72 760 Td\n14 TL\n")
	for _, line := range lines {
		for _, wrappedLine := range wrapPDFLine(line, 84) {
			fmt.Fprintf(&stream, "(%s) Tj\nT*\n", escapePDFText(wrappedLine))
		}
	}
	stream.WriteString("ET\n")
	content := stream.String()

	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects)+1)
	offsets = append(offsets, 0)
	for index, object := range objects {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xrefOffset := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(objects)+1)
	out.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return out.Bytes()
}

func wrapPDFLine(line string, limit int) []string {
	if line == "" {
		return []string{""}
	}
	var wrapped []string
	words := strings.Fields(line)
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if len(current)+1+len(word) > limit {
			wrapped = append(wrapped, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		wrapped = append(wrapped, current)
	}
	return wrapped
}

func escapePDFText(text string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)")
	return replacer.Replace(strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r < 32 || r > 126 {
			return '?'
		}
		return r
	}, text))
}
