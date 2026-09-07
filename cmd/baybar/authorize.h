#ifndef BAY_AUTHORIZE_H
#define BAY_AUTHORIZE_H

// authorized_run executes path with administrator rights, putting up the
// system's own authentication dialog attributed to this application. argv is
// NULL-terminated and does not include the program name.
//
// It returns 0 on success. On failure it writes a description into err and
// returns non-zero; a cancelled dialog is reported as such rather than as a
// generic error.
int authorized_run(const char *path, const char *const *argv, char *err, int errlen);

#endif
