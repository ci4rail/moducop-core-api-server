package main

import (
	"fmt"
	"os"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

func main() {
	fmt.Println("Rebooting...")

	st, err := mockmender.LoadState()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	_, installed, trial := mockmender.Stage()
	switch st.Stage {
	case installed:
		// First reboot after install: trial boot into new rootfs.
		mockmender.TrialBoot(&st)
	case trial:
		// If not committed and rebooted again, rollback.
		mockmender.RollbackAfterFailedTrial(&st)
	default:
	}

	if err := mockmender.SaveState(st); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
