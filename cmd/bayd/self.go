package main

import (
	"context"
	"log"
	"os"
	"time"
)

// A resident agent runs the executable it was started from until it ends, so
// an installed new version would wait for the next login or boot. The agent
// therefore watches its own file and exits when the file is replaced; launchd
// (KeepAlive) and systemd (Restart=always) start the new one.

// selfCheck is how often the agent looks at its file.
const selfCheck = 10 * time.Second

func exitWhenReplaced(ctx context.Context) {
	path, err := os.Executable()
	if err != nil {
		return
	}
	first, err := os.Stat(path)
	if err != nil {
		return
	}
	t := time.NewTicker(selfCheck)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now, err := os.Stat(path)
		if err != nil {
			continue // being replaced; looked at again next time
		}
		if !os.SameFile(first, now) || !now.ModTime().Equal(first.ModTime()) {
			log.Printf("%s was replaced; exiting so the new version starts", path)
			os.Exit(0)
		}
	}
}
