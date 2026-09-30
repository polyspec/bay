#include <dns_sd.h>
#include <stdint.h>

int name_start(DNSServiceRef *ref, const char *name, uint32_t protocols, uintptr_t handle);
int name_next(DNSServiceRef ref, int cancel_fd);
