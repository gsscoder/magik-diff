package brief

import (
	"errors"
	"strings"
	"testing"
)

// overviewText builds an overview with one body per fixed heading.
func overviewText(bodies [7]string) string {
	var b strings.Builder
	for i, h := range headings {
		b.WriteString("## " + h + "\n" + bodies[i] + "\n")
	}
	return b.String()
}

func validOverview() string {
	return overviewText([7]string{"A tool.", "Developers.", "- scan", "One app.", "Go", "Beta", "None."})
}

func allNotDocumented() [7]string {
	var bodies [7]string
	for i := range bodies {
		bodies[i] = notDocumented
	}
	return bodies
}

func TestValidateOverview_CompleteIsValid(t *testing.T) {
	overview := validOverview()
	got, err := validateOverview("\n" + overview + "\n")
	if err != nil || got != strings.TrimSpace(overview) {
		t.Errorf("validateOverview() = %q, %v; want %q, nil", got, err, strings.TrimSpace(overview))
	}
}

func TestValidateOverview_FenceIsRemoved(t *testing.T) {
	overview := validOverview()
	got, err := validateOverview("```markdown\n" + overview + "```")
	if err != nil || got != strings.TrimSpace(overview) {
		t.Errorf("validateOverview() = %q, %v; want %q, nil", got, err, strings.TrimSpace(overview))
	}
}

func TestValidateOverview_SentinelEvenWhenFenced(t *testing.T) {
	for _, answer := range []string{
		"INSUFFICIENT_SOURCES",
		"  INSUFFICIENT_SOURCES\n",
		"```\nINSUFFICIENT_SOURCES\n```",
	} {
		if _, err := validateOverview(answer); !errors.Is(err, errInsufficientSources) {
			t.Errorf("validateOverview(%q) error = %v, want errInsufficientSources", answer, err)
		}
	}
}

func TestValidateOverview_MissingHeadingIsUnusable(t *testing.T) {
	for _, answer := range []string{
		strings.Replace(validOverview(), "## Status\n", "## State\n", 1),
		"A plain paragraph.",
	} {
		if _, err := validateOverview(answer); !errors.Is(err, errUnusableOverview) {
			t.Errorf("validateOverview(%q) error = %v, want errUnusableOverview", answer, err)
		}
	}
}

func TestValidateOverview_NoFactIsUnusable(t *testing.T) {
	if _, err := validateOverview(overviewText(allNotDocumented())); !errors.Is(err, errUnusableOverview) {
		t.Errorf("validateOverview() error = %v, want errUnusableOverview", err)
	}
}

func TestValidateOverview_OneDocumentedHeadingIsEnough(t *testing.T) {
	bodies := allNotDocumented()
	bodies[0] = "A tool."
	if _, err := validateOverview(overviewText(bodies)); err != nil {
		t.Errorf("validateOverview() error = %v, want nil", err)
	}
}

func TestValidateOverview_TextAfterUnknownHeadingIsNotAFact(t *testing.T) {
	answer := overviewText(allNotDocumented()) + "## Extra\nfacts\n"
	if _, err := validateOverview(answer); !errors.Is(err, errUnusableOverview) {
		t.Errorf("validateOverview() error = %v, want errUnusableOverview", err)
	}
}

func TestCheckHeadings_NotDocumentedWithOrWithoutPeriodHasNoFacts(t *testing.T) {
	for _, body := range []string{"Not documented", "Not documented."} {
		var bodies [7]string
		for i := range bodies {
			bodies[i] = body
		}
		if err := checkHeadings(overviewText(bodies)); !errors.Is(err, errUnusableOverview) {
			t.Errorf("checkHeadings(%q bodies) error = %v, want errUnusableOverview", body, err)
		}
	}
}

func TestOverviewPrompt_NamesEveryHeading(t *testing.T) {
	for _, h := range headings {
		if !strings.Contains(overviewPrompt, "\n## "+h) {
			t.Errorf("overviewPrompt lacks heading %q", h)
		}
	}
}

func TestPrompts_ForbidALineEndPeriod(t *testing.T) {
	for name, p := range map[string]string{"overviewPrompt": overviewPrompt, "briefPrompt": briefPrompt} {
		if !strings.Contains(p, "do not end a line with a period") {
			t.Errorf("%s does not forbid a line-end period", name)
		}
	}
}

func TestValidateBrief(t *testing.T) {
	for _, answer := range []string{"  \n", "```\n```"} {
		if _, err := validateBrief(answer); err == nil {
			t.Errorf("validateBrief(%q) error = nil, want an error for an empty brief", answer)
		}
	}
	if got, err := validateBrief("```\n- a\n- b\n```"); err != nil || got != "- a\n- b" {
		t.Errorf("validateBrief(fenced) = %q, %v; want %q, nil", got, err, "- a\n- b")
	}
}

func TestDropLineEndPeriods(t *testing.T) {
	tests := map[string]struct {
		text string
		want string
	}{
		"line-end period dropped": {
			text: "A tool.\r\nDevelopers. \n- scan.\n\nlast.",
			want: "A tool\r\nDevelopers \n- scan\n\nlast",
		},
		"ellipsis kept": {
			text: "wait..\nmore...\n",
			want: "wait..\nmore...\n",
		},
		"code fence kept": {
			text: "before.\n```go\nx := a.\n```\nafter.\n",
			want: "before\n```go\nx := a.\n```\nafter\n",
		},
		"period inside a line kept": {
			text: "see file.md for details\nv1. next\n- no period\n",
			want: "see file.md for details\nv1. next\n- no period\n",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := dropLineEndPeriods(tt.text); got != tt.want {
				t.Errorf("dropLineEndPeriods(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
