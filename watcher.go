// watches the current directory for changes and runs the specificed program on change
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"github.com/howeyc/fsnotify"
)

var help = `watcher [command to execute]`

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s [flags] [command to execute and args]\n", os.Args[0])
	flag.PrintDefaults()
}

var verbose = flag.Bool("v", false, "verbose")
var depth = flag.Int("depth", 1, "recursion depth")
var quiet = flag.Int("quiet", 800, "quiet period after command execution in milliseconds")
var dir = flag.String("dir", ".", "directory root to use for watching")

func main() {
	flag.Usage = usage
	flag.Parse()

	watcher, err := newWatcher()
	if err != nil {
		log.Fatal(err)
	}
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}
	cmd, args := flag.Args()[0], flag.Args()[1:]

	fileEvents := make(chan interface{}, 100)

	// pipe all events to fileEvents (for buffering and draining)
	go watcher.pipeEvents(fileEvents)

	go watchAndExecute(fileEvents, cmd, args)

	dir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal(err)
	}
	err = watcher.watchDirAndChildren(dir, *depth)
	if err != nil {
		log.Fatal(err)
	}
	<-make(chan struct{})
	watcher.Close()
}

type watcher struct {
	*fsnotify.Watcher
}

func newWatcher() (watcher, error) {
	fsnw, err := fsnotify.NewWatcher()
	return watcher{fsnw}, err
}

// Execute cmd with args when a file event occurs
func watchAndExecute(fileEvents chan interface{}, cmd string, args []string) {
	for {
		// execute command
		c := exec.Command(cmd, args...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin

		fmt.Fprintln(os.Stderr, "running", cmd, args)
		if err := c.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "error running:", err)
		}
		if *verbose {
			fmt.Fprintln(os.Stderr, "done.")
		}
		// drain until quiet period is over
		drainFor(*quiet, fileEvents)
		ev := <-fileEvents
		if *verbose {
			fmt.Fprintln(os.Stderr, "File changed:", ev)
		}
	}
}

// Add dir and children (recursively) to watcher
func (w watcher) watchDirAndChildren(path string, depth int) error {
	if err := w.Watch(path); err != nil {
		return err
	}
	baseNumSeps := strings.Count(path, string(os.PathSeparator))
	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			pathDepth := strings.Count(path, string(os.PathSeparator)) - baseNumSeps
			if pathDepth > depth {
				return filepath.SkipDir
			}
			if *verbose {
				fmt.Fprintln(os.Stderr, "Watching", path)
			}
			if err := w.Watch(path); err != nil {
				return err
			}
		}
		return nil
	})
}

func (w watcher) pipeEvents(events chan interface{}) {
	for {
		select {
		case ev := <-w.Event:
			events <- ev
			// @todo handle created/renamed/deleted dirs
		case err := <-w.Error:
			log.Println("fsnotify error:", err)
		}
	}
}

// drainFor drains events from channel with a until a period in ms has elapsed timeout
func drainFor(drainTimeMs int, c chan interface{}) {
	timeout := time.After(time.Duration(drainTimeMs) * time.Millisecond)
	for {
		select {
		case <-c:
		case <-timeout:
			return
		}
	}
}
