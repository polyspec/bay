#ifndef BAY_WINDOW_H
#define BAY_WINDOW_H

// Rendering a native window from Go means crossing into AppKit, which owns the
// main thread. Both calls below hand the JSON view model to the main queue and
// return immediately; the window is built and updated there.
//
// The JSON is the whole window: a title, and a list of sections, each with a
// header, optional buttons and a list of rows. Passing one string keeps the
// bridge to a single symbol instead of a widget API.
void ui_show(const char *json);
void ui_update(const char *json);
void ui_is_visible(int *out);

// ui_logs opens a plain scrolling text window. Logs do not belong in the panel
// layout: they are long, monospaced and read on their own.
void ui_logs(const char *title, const char *text);

// ui_prompt asks for one line of text. On OK it calls back with the action id
// and the value; on cancel it calls back with nothing at all.
void ui_prompt(const char *actionID, const char *title, const char *message,
               const char *placeholder, const char *initial);

// ui_confirm asks a yes/no question before something irreversible. On OK it
// delivers the action id the same way a button would.
void ui_confirm(const char *actionID, const char *title, const char *message,
                const char *okTitle, int destructive);

#endif
