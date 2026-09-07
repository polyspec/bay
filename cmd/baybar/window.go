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
	ID       string `json:"id"`
	Label    string `json:"label"`
	Dot      string `json:"dot,omitempty"`
	Count    string `json:"count,omitempty"`
	Selected bool   `json:"selected,omitempty"`
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
	Title  string  `json:"title"`
	Text   string  `json:"text"`
	Button *button `json:"button,omitempty"`
}

type section struct {
	Header  string   `json:"header"`
	Detail  string   `json:"detail,omitempty"`
	Note    string   `json:"note,omitempty"`
	Buttons []button `json:"buttons,omitempty"`
	Rows    []row    `json:"rows,omitempty"`
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
	Buttons  []button `json:"buttons,omitempty"`
}

type button struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Primary marks the screen's main action. AppKit draws it in the accent
	// colour.
	Primary  bool `json:"primary,omitempty"`
	Disabled bool `json:"disabled,omitempty"`
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

// showLogs opens the text window with a service's output.
func showLogs(title, body string) {
	t := C.CString(title)
	b := C.CString(body)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(b))
	C.ui_logs(t, b)
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
