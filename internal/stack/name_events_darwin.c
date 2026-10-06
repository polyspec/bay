//go:build darwin && cgo

#include "name_events_darwin.h"
#include <errno.h>
#include <netinet/in.h>
#include <poll.h>
#include <sys/socket.h>

extern void goNameEvent(uintptr_t handle, uint32_t flags, int family, void *data, int size, int error);

static void name_callback(DNSServiceRef ref, DNSServiceFlags flags,
 uint32_t interface, DNSServiceErrorType error, const char *name,
 uint16_t type, uint16_t class, uint16_t length, const void *data,
 uint32_t ttl, void *context) {
 if (error != 0) {
  goNameEvent((uintptr_t)context, flags, 0, NULL, 0, error);
  return;
 }
 int family = AF_UNSPEC;
 if (type == kDNSServiceType_A) {
  family = AF_INET;
 } else if (type == kDNSServiceType_AAAA) {
  family = AF_INET6;
 }
 goNameEvent((uintptr_t)context, flags, family, (void *)data, length, 0);
}

// name_start creates one connection to the system resolver and registers a
// long-lived query for each requested address type on it. A long-lived query
// sends the question to the DNS server instead of answering it only from the
// cache, and the server's answer replaces the cached entries for the name: a
// negative answer cached before the container existed, or the addresses of a
// replaced container.
int name_start(DNSServiceRef *connection, const char *name, uint32_t protocols, uintptr_t handle) {
 DNSServiceErrorType error = DNSServiceCreateConnection(connection);
 if (error != kDNSServiceErr_NoError) return error;
 const uint32_t wanted[2] = {kDNSServiceProtocol_IPv4, kDNSServiceProtocol_IPv6};
 const uint16_t types[2] = {kDNSServiceType_A, kDNSServiceType_AAAA};
 for (int i = 0; i < 2; i++) {
  if ((protocols & wanted[i]) == 0) continue;
  DNSServiceRef query = *connection;
  error = DNSServiceQueryRecord(&query,
   kDNSServiceFlagsShareConnection | kDNSServiceFlagsLongLivedQuery,
   kDNSServiceInterfaceIndexAny, name, types[i], kDNSServiceClass_IN,
   name_callback, (void *)handle);
  if (error != kDNSServiceErr_NoError) {
   DNSServiceRefDeallocate(*connection);
   return error;
  }
 }
 return kDNSServiceErr_NoError;
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
