//go:build darwin && cgo

#include "name_events_darwin.h"
#include <errno.h>
#include <netinet/in.h>
#include <poll.h>

extern void goNameEvent(uintptr_t handle, uint32_t flags, int family, void *data, int size, int error);

static void name_callback(DNSServiceRef ref, DNSServiceFlags flags,
 uint32_t interface, DNSServiceErrorType error, const char *host,
 const struct sockaddr *address, uint32_t ttl, void *context) {
 if (error != 0 || address == NULL) {
  goNameEvent((uintptr_t)context, flags, 0, NULL, 0, error);
  return;
 }
 if (address->sa_family == AF_INET) {
  struct sockaddr_in *value = (struct sockaddr_in *)address;
  goNameEvent((uintptr_t)context, flags, AF_INET, &value->sin_addr, 4, 0);
 } else if (address->sa_family == AF_INET6) {
  struct sockaddr_in6 *value = (struct sockaddr_in6 *)address;
  goNameEvent((uintptr_t)context, flags, AF_INET6, &value->sin6_addr, 16, 0);
 } else {
  goNameEvent((uintptr_t)context, flags, address->sa_family, NULL, 0, 0);
 }
}

int name_start(DNSServiceRef *ref, const char *name, uint32_t protocols, uintptr_t handle) {
 return DNSServiceGetAddrInfo(ref, 0, kDNSServiceInterfaceIndexAny, protocols,
  name, name_callback, (void *)handle);
}

int name_next(DNSServiceRef ref, int cancel_fd) {
 struct pollfd fds[2] = {{DNSServiceRefSockFD(ref), POLLIN, 0}, {cancel_fd, POLLIN, 0}};
 for (;;) {
  int result = poll(fds, 2, -1);
  if (result < 0) {
   if (errno == EINTR) continue;
   return kDNSServiceErr_Unknown;
  }
  if (fds[1].revents != 0) return 1;
  if (fds[0].revents & POLLIN) return DNSServiceProcessResult(ref);
  if (fds[0].revents != 0) return kDNSServiceErr_ServiceNotRunning;
 }
}
