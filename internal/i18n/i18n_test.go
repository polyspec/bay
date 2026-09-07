package i18n

import (
	"regexp"
	"strings"
	"testing"
)

var verbs = regexp.MustCompile(`%[-+ #0]*[0-9.*]*[a-zA-Z]`)

// Every Korean line takes the same values as the English one it replaces, so a
// translated line cannot drop or reorder an argument.
func TestVerbsMatch(t *testing.T) {
	for en, ko := range korean {
		a, b := verbs.FindAllString(en, -1), verbs.FindAllString(ko, -1)
		if len(a) != len(b) {
			t.Errorf("%q takes %v, its translation takes %v", en, a, b)
			continue
		}
		for i := range a {
			if a[i] != b[i] {
				t.Errorf("%q takes %v, its translation takes %v", en, a, b)
				break
			}
		}
	}
}

func TestNoEmptyTranslation(t *testing.T) {
	for en, ko := range korean {
		if strings.TrimSpace(ko) == "" {
			t.Errorf("%q has no translation", en)
		}
	}
}

// banned lists the phrasings the project does not use. Interface text states
// the action and its subject directly.
var banned = []string{
	"묶임", "진술", "남처럼", "읽힌다", "비켜서고", "단언", "부재", "퇴역",
	"부여", "봉투", "게이트", "가장자리", "이름으로", "말한다", "돈다", "회수",
	"센다", "자리에", "싣는다", "싣고", "두지", "낸다", "답한다", "꽂으면",
	"나간다", "달고", "흘린다", "흘려보낸", "보낸다", "투영", "침묵", "가리킨다",
	"밝힌다", "소속은", "고아", "입양", "흉내", "박다", "무는지", "물어야",
	"잽니다", "죽이나", "죽이다", "놓으라", "유도", "덮이고", "실어",
}

func TestWordingRules(t *testing.T) {
	for en, ko := range korean {
		for _, w := range banned {
			if strings.Contains(ko, w) {
				t.Errorf("%q uses %q", en, w)
			}
		}
	}
}

func TestMatch(t *testing.T) {
	for tag, want := range map[string]Lang{
		"ko-KR": Ko, "ko": Ko, "en-US": En, "ja-JP": En, "": En,
	} {
		if got := Match(tag); got != want {
			t.Errorf("Match(%q) = %v, want %v", tag, got, want)
		}
	}
}

// P chooses the English wording by count and always uses the Korean one.
func TestPlural(t *testing.T) {
	en := Printer{Lang: En}
	if got := en.P("%d domain", "%d domains", 1, 1); got != "1 domain" {
		t.Errorf("one = %q", got)
	}
	if got := en.P("%d domain", "%d domains", 2, 2); got != "2 domains" {
		t.Errorf("other = %q", got)
	}
	ko := Printer{Lang: Ko}
	if got := ko.P("Serving %d domain", "Serving %d domains", 1, 1); got != "도메인 1개 제공 중" {
		t.Errorf("korean = %q", got)
	}
}

// A line with no translation is shown in English rather than as an empty row.
func TestFallsBackToEnglish(t *testing.T) {
	ko := Printer{Lang: Ko}
	if got := ko.T("a line with no translation"); got != "a line with no translation" {
		t.Errorf("fallback = %q", got)
	}
}
