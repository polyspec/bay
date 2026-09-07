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

// ui_logs opens a plain scrolling text window. Long monospaced text does not
// belong in the panel layout and is read on its own. atEnd scrolls to the last
// line, which is where a log is read from; a file is read from the top.
void ui_logs(const char *title, const char *text, int atEnd);

// ui_prompt asks for one line of text. On OK it calls back with the action id
// and the value; on cancel it calls back with nothing at all.
void ui_prompt(const char *actionID, const char *title, const char *message,
               const char *placeholder, const char *initial);

// ui_confirm asks a yes/no question before something irreversible. On OK it
// delivers the action id the same way a button would.
void ui_confirm(const char *actionID, const char *title, const char *message,
                const char *okTitle, int destructive);

// ui_sheet asks for one line of text together with one choice, on a sheet
// attached to the window. suffix is appended to what is typed and shown beside
// the field, so the resulting name is visible before the sheet is accepted. On
// accept it calls back with the action id, the value and the choice; on cancel
// it calls back with nothing at all.
void ui_sheet(const char *actionID, const char *title, const char *message,
              const char *fieldLabel, const char *placeholder, const char *suffix,
              const char *optionLabel, int optionOn, const char *acceptTitle);

// ui_pick asks for a directory or a file with the open panel. On choose it
// calls back with the action id and the path; on cancel it calls back with
// nothing at all.
void ui_pick(const char *actionID, const char *title, const char *prompt);

// ui_language returns the locale identifier the system prefers, so the window
// picks the language the rest of the system uses.
void ui_language(char *out, int n);

// ui_labels sets the words the window itself supplies: the appearance control,
// the buttons on a dialog, and the placeholder in an empty log pane. It is a
// JSON object of name to text.
void ui_labels(const char *json);

// ui_copy puts one string on the general pasteboard.
void ui_copy(const char *text);

// ui_flag reads and writes one boolean in the defaults database, which is where
// the window's own settings live.
void ui_flag(const char *key, int *out);
void ui_set_flag(const char *key, int value);

// ui_text reads and writes one string in the defaults database.
void ui_text(const char *key, char *out, int n);
void ui_set_text(const char *key, const char *value);

#endif
