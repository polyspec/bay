package main

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "window.h"
#include "authorize.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"unsafe"
)

// The window is described as data. Go produces the view model, AppKit renders
// it. One JSON string crosses the boundary and one callback returns.
type panel struct {
	// Sidebar is the source list. The detail pane shows the selected item.
	Sidebar []sideGroup `json:"sidebar"`
	Header  header      `json:"header"`
	// Verdict is the dashboard's status line. It is empty on other screens.
	Verdict *verdict `json:"verdict,omitempty"`
	// Banner is shown when the machine setup is incomplete.
	Banner   *banner `json:"banner,omitempty"`
	Subtitle string  `json:"subtitle"`
	// Message is the outcome of the last action. Notifications are suppressed
	// for an unsigned application, so the outcome is shown in the window.
	Message     string    `json:"message,omitempty"`
	MessageKind string    `json:"messageKind,omitempty"` // error or info
	Sections    []section `json:"sections"`
}

type sideGroup struct {
	Title string     `json:"title"`
	Items []sideItem `json:"items"`
}

type sideItem struct {
	// ID is the action delivered when the row is clicked.
	ID    string `json:"id"`
	Label string `json:"label"`
	Dot   string `json:"dot,omitempty"`
	Count string `json:"count,omitempty"`
	// Sub indents the row under the project above it. A project lists its
	// services while it is the subject, so there is nothing to disclose by
	// clicking and no disclosure mark is drawn.
	Sub      bool `json:"sub,omitempty"`
	Selected bool `json:"selected,omitempty"`
}

type header struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	Buttons  []button `json:"buttons,omitempty"`
}

type verdict struct {
	Dot      string `json:"dot"`
	Headline string `json:"headline"`
	Subline  string `json:"subline,omitempty"`
}

type banner struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	// Kind is "bad" for a fault and empty for a warning.
	Kind    string   `json:"kind,omitempty"`
	Buttons []button `json:"buttons,omitempty"`
}

type section struct {
	Header  string   `json:"header"`
	Detail  string   `json:"detail,omitempty"`
	Note    string   `json:"note,omitempty"`
	Buttons []button `json:"buttons,omitempty"`
	Rows    []row    `json:"rows,omitempty"`
	// Log holds the tail of a container's output, drawn on the code background
	// under the section's rows.
	Log []string `json:"log,omitempty"`
	// LogNote names what the pane is showing; LogButtons act on it.
	LogNote    string   `json:"logNote,omitempty"`
	LogButtons []button `json:"logButtons,omitempty"`
}

type row struct {
	Text string `json:"text"`
	// Kind "kv" renders a label and value line instead of a status row.
	Kind string `json:"kind,omitempty"`
	// ID makes the row clickable when set.
	ID string `json:"id,omitempty"`
	// Chip is a bordered tag shown beside the name.
	Chip string `json:"chip,omitempty"`
	// Dots holds one state per service, for a project summary row.
	Dots     []string `json:"dots,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	Dot      string   `json:"dot,omitempty"` // on, warn, bad or empty
	Link     string   `json:"link,omitempty"`
	LinkText string   `json:"linkText,omitempty"`
	// Wide widens the name column, for a list of certificate names.
	Wide bool `json:"wide,omitempty"`
	// Faint is a tertiary suffix after a kv row's value.
	Faint string `json:"faint,omitempty"`
	// Hint is a tertiary second line under a kv row's value.
	Hint string `json:"hint,omitempty"`
	// Mono draws a kv row's value in the monospaced font.
	Mono bool `json:"mono,omitempty"`
	// Appearance places the Auto/Dark/Light control in a kv row. AppKit owns
	// that choice, so it is the one control the window builds itself.
	Appearance bool `json:"appearance,omitempty"`
	// Segment places a choice of one from several in a kv row.
	Segment *segment `json:"segment,omitempty"`
	// Toggle is the action a switch in a kv row delivers; On is its state.
	Toggle   string   `json:"toggle,omitempty"`
	On       bool     `json:"on,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
	Buttons  []button `json:"buttons,omitempty"`
}

// segment is a choice of one value from several. Choosing delivers the
// identifier with the chosen index appended.
type segment struct {
	ID       string   `json:"id"`
	Labels   []string `json:"labels"`
	Selected int      `json:"selected"`
}

type button struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Style is "hero" for the screen's main action, "quiet" for a secondary
	// one, and empty for the standard weight.
	Style    string `json:"style,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

// hero returns the screen's main action.
func hero(id, title string, disabled bool) button {
	return button{ID: id, Title: title, Style: "hero", Disabled: disabled}
}

// quiet returns a secondary action.
func quiet(id, title string, disabled bool) button {
	return button{ID: id, Title: title, Style: "quiet", Disabled: disabled}
}

// actionHandler receives every button and link identifier the window produces.
// It is set at start-up.
var actionHandler func(string)

//export goUIAction
func goUIAction(id *C.char) {
	if actionHandler != nil {
		go actionHandler(C.GoString(id))
	}
}

// promptHandler receives an action identifier and the entered text. Cancelling
// produces no call.
var promptHandler func(id, value string)

//export goUIPrompt
func goUIPrompt(id, value *C.char) {
	if promptHandler != nil {
		go promptHandler(C.GoString(id), C.GoString(value))
	}
}

// sheetHandler receives the identifier, the typed value and the choice from the
// sheet. Cancelling produces no call.
var sheetHandler func(id, value string, option bool)

//export goUISheet
func goUISheet(id, value *C.char, option C.int) {
	if sheetHandler != nil {
		go sheetHandler(C.GoString(id), C.GoString(value), option != 0)
	}
}

// sheet asks for one line of text and one choice on a sheet attached to the
// window. suffix is shown beside the field so the resulting name is visible
// before the sheet is accepted.
func sheet(actionID, title, message, fieldLabel, placeholder, suffix, optionLabel, acceptTitle string, optionOn bool) {
	cs := make([]*C.char, 8)
	for i, s := range []string{actionID, title, message, fieldLabel, placeholder, suffix, optionLabel, acceptTitle} {
		cs[i] = C.CString(s)
		defer C.free(unsafe.Pointer(cs[i]))
	}
	var on C.int
	if optionOn {
		on = 1
	}
	C.ui_sheet(cs[0], cs[1], cs[2], cs[3], cs[4], cs[5], cs[6], on, cs[7])
}

// prompt asks for one line of text; the answer arrives at promptHandler.
func prompt(actionID, title, message, placeholder, initial string) {
	cs := make([]*C.char, 5)
	for i, s := range []string{actionID, title, message, placeholder, initial} {
		cs[i] = C.CString(s)
		defer C.free(unsafe.Pointer(cs[i]))
	}
	C.ui_prompt(cs[0], cs[1], cs[2], cs[3], cs[4])
}

// confirm asks before something irreversible; OK delivers actionID as a click.
func confirm(actionID, title, message, okTitle string, destructive bool) {
	cs := make([]*C.char, 4)
	for i, s := range []string{actionID, title, message, okTitle} {
		cs[i] = C.CString(s)
		defer C.free(unsafe.Pointer(cs[i]))
	}
	var d C.int
	if destructive {
		d = 1
	}
	C.ui_confirm(cs[0], cs[1], cs[2], cs[3], d)
}

func showWindow(p panel) { withJSON(p, func(s *C.char) { C.ui_show(s) }) }

// updateWindow redraws only while the window is on screen.
func updateWindow(p panel) { withJSON(p, func(s *C.char) { C.ui_update(s) }) }

func windowVisible() bool {
	var out C.int
	C.ui_is_visible(&out)
	return out != 0
}

// elevate runs the helper as root through the system authentication panel. An
// application has no terminal, so sudo cannot be used.
func elevate(exe string, args []string) error {
	cexe := C.CString(exe)
	defer C.free(unsafe.Pointer(cexe))

	// The argument vector is NULL-terminated and excludes the program name.
	cargs := make([]*C.char, len(args)+1)
	for i, a := range args {
		cargs[i] = C.CString(a)
		defer C.free(unsafe.Pointer(cargs[i]))
	}

	errbuf := (*C.char)(C.malloc(1024))
	defer C.free(unsafe.Pointer(errbuf))
	*errbuf = 0

	if C.authorized_run(cexe, (**C.char)(unsafe.Pointer(&cargs[0])), errbuf, 1024) != 0 {
		msg := C.GoString(errbuf)
		if msg == "cancelled" {
			return errCancelled
		}
		return errors.New(msg)
	}
	return nil
}

// errCancelled reports that the authentication panel was dismissed.
var errCancelled = errors.New("cancelled")

// pick asks for a directory or a file; the answer arrives at promptHandler.
func pick(actionID, title, promptTitle string) {
	cs := make([]*C.char, 3)
	for i, s := range []string{actionID, title, promptTitle} {
		cs[i] = C.CString(s)
		defer C.free(unsafe.Pointer(cs[i]))
	}
	C.ui_pick(cs[0], cs[1], cs[2])
}

// language returns the locale identifier the system prefers.
func language() string {
	buf := (*C.char)(C.malloc(64))
	defer C.free(unsafe.Pointer(buf))
	C.ui_language(buf, 64)
	return C.GoString(buf)
}

// setLabels supplies the words the window itself builds controls from.
func setLabels(m map[string]string) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	s := C.CString(string(b))
	defer C.free(unsafe.Pointer(s))
	C.ui_labels(s)
}

// boolSetting reads one of the window's own settings from the defaults
// database.
func boolSetting(key string) bool {
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	var out C.int
	C.ui_flag(k, &out)
	return out != 0
}

// setBoolSetting writes one of the window's own settings to the defaults
// database.
func setBoolSetting(key string, value bool) {
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	var v C.int
	if value {
		v = 1
	}
	C.ui_set_flag(k, v)
}

// stringSetting reads one of the window's own settings from the defaults
// database.
func stringSetting(key string) string {
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))
	buf := (*C.char)(C.malloc(64))
	defer C.free(unsafe.Pointer(buf))
	C.ui_text(k, buf, 64)
	return C.GoString(buf)
}

// setStringSetting writes one of the window's own settings to the defaults
// database.
func setStringSetting(key, value string) {
	k, v := C.CString(key), C.CString(value)
	defer C.free(unsafe.Pointer(k))
	defer C.free(unsafe.Pointer(v))
	C.ui_set_text(k, v)
}

// copyText puts one string on the general pasteboard.
func copyText(text string) {
	s := C.CString(text)
	defer C.free(unsafe.Pointer(s))
	C.ui_copy(s)
}

// showLogs opens the text window. atEnd scrolls to the last line, which is
// where a log is read from; a file is read from the top.
func showLogs(title, body string, atEnd bool) {
	t := C.CString(title)
	b := C.CString(body)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(b))
	var end C.int
	if atEnd {
		end = 1
	}
	C.ui_logs(t, b, end)
}

func withJSON(p panel, fn func(*C.char)) {
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	s := C.CString(string(b))
	defer C.free(unsafe.Pointer(s))
	fn(s)
}
