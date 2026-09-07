#import <Foundation/Foundation.h>
#import <Security/Security.h>
#include "authorize.h"

// AuthorizationExecuteWithPrivileges runs a command as root and presents the
// system authentication panel. The supported replacement, a privileged helper
// installed with SMJobBless, requires a Developer ID signature, which this
// application does not have.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

int authorized_run(const char *path, const char *const *argv, char *err, int errlen) {
  AuthorizationRef auth = NULL;
  OSStatus status = AuthorizationCreate(NULL, kAuthorizationEmptyEnvironment,
                                        kAuthorizationFlagDefaults, &auth);
  if (status != errAuthorizationSuccess) {
    snprintf(err, errlen, "could not start an authorization session (%d)", (int)status);
    return 1;
  }

  AuthorizationItem right = {kAuthorizationRightExecute, 0, NULL, 0};
  AuthorizationRights rights = {1, &right};
  AuthorizationFlags flags = kAuthorizationFlagDefaults |
                             kAuthorizationFlagInteractionAllowed |
                             kAuthorizationFlagPreAuthorize |
                             kAuthorizationFlagExtendRights;
  status = AuthorizationCopyRights(auth, &rights, kAuthorizationEmptyEnvironment, flags, NULL);
  if (status != errAuthorizationSuccess) {
    AuthorizationFree(auth, kAuthorizationFlagDefaults);
    if (status == errAuthorizationCanceled) {
      snprintf(err, errlen, "cancelled");
    } else {
      snprintf(err, errlen, "not authorized (%d)", (int)status);
    }
    return 1;
  }

  FILE *pipe = NULL;
  status = AuthorizationExecuteWithPrivileges(auth, path, kAuthorizationFlagDefaults,
                                              (char *const *)argv, &pipe);
  if (status != errAuthorizationSuccess) {
    AuthorizationFree(auth, kAuthorizationFlagDefaults);
    snprintf(err, errlen, "could not run %s as an administrator (%d)", path, (int)status);
    return 1;
  }

  // Collect the child's output; it carries the error text.
  NSMutableString *out = [NSMutableString string];
  if (pipe) {
    char buf[512];
    size_t n;
    while ((n = fread(buf, 1, sizeof(buf) - 1, pipe)) > 0) {
      buf[n] = 0;
      [out appendFormat:@"%s", buf];
    }
    fclose(pipe);
  }
  AuthorizationFree(auth, kAuthorizationFlagDefaults);

  NSString *trimmed = [out stringByTrimmingCharactersInSet:
                          [NSCharacterSet whitespaceAndNewlineCharacterSet]];
  // AuthorizationExecuteWithPrivileges does not report the child's exit
  // status, so a failure is detected from the output.
  if ([trimmed rangeOfString:@"bay:"].location != NSNotFound) {
    snprintf(err, errlen, "%s", [trimmed UTF8String]);
    return 1;
  }
  return 0;
}

#pragma clang diagnostic pop
