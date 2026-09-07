// Package i18n holds the text the window shows. English is the canonical
// wording and is written at the call site; Korean is looked up by that English
// text and carries the same information.
package i18n

import (
	"fmt"
	"strings"
)

type Lang int

const (
	En Lang = iota
	Ko
)

// Match returns the language for a locale identifier such as "ko-KR". Anything
// that is not Korean uses English.
func Match(tag string) Lang {
	if strings.HasPrefix(strings.ToLower(tag), "ko") {
		return Ko
	}
	return En
}

// Choices are the values the language setting can hold, in the order the
// setting offers them. "system" follows the locale the system prefers.
var Choices = []string{"system", "en", "ko"}

// Resolve returns the language for a stored choice. system is the locale the
// system prefers, which is used for an unknown or empty choice as well.
func Resolve(choice, system string) Lang {
	switch choice {
	case "en":
		return En
	case "ko":
		return Ko
	default:
		return Match(system)
	}
}

// IndexOf returns the position of a stored choice in Choices, and 0 for one
// that is not recognised.
func IndexOf(choice string) int {
	for i, c := range Choices {
		if c == choice {
			return i
		}
	}
	return 0
}

// Printer renders text in one language.
type Printer struct{ Lang Lang }

// T returns text that does not depend on a count.
func (p Printer) T(text string, args ...any) string {
	return format(p.lookup(text), args)
}

// P returns the wording for a count. English distinguishes one from the rest;
// Korean does not, so it is looked up by the plural wording. args is the whole
// argument list, because the count is not always the first value in the line.
func (p Printer) P(one, other string, n int, args ...any) string {
	if p.Lang == Ko {
		return format(p.lookup(other), args)
	}
	if n == 1 {
		return format(one, args)
	}
	return format(other, args)
}

// lookup returns the translation, or the English text when there is none.
func (p Printer) lookup(text string) string {
	if p.Lang == Ko {
		if s, ok := korean[text]; ok {
			return s
		}
	}
	return text
}

func format(s string, args []any) string {
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}
