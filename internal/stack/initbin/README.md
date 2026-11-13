`make` builds `cmd/bayinit` here for linux/arm64 and linux/amd64 before it
builds the commands, which embed them. The binaries are not committed. A
build without them gives containers trust through variables only, and
`bay check` says so.
