// Command pulsar-indexer runs the Pulsar event indexer daemon.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	fmt.Fprintln(os.Stderr, "pulsar-indexer starting")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	fmt.Fprintf(os.Stderr, "pulsar-indexer stopping: received %s\n", sig)
	os.Exit(0)
}
