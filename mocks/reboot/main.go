/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"fmt"
	"os"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

func main() {
	fmt.Println("Rebooting...")

	if mockmender.ShouldKillParent() {
		_ = mockmender.KillParentProcess()
	}

	st, err := mockmender.LoadState()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	_, installed, trial := mockmender.Stage()
	switch st.Stage {
	case installed:
		if st.PendingUpdateType == string(mockmender.UpdateTypeRootfs) {
			// First reboot after rootfs install: trial boot into new rootfs.
			mockmender.TrialBoot(&st)
		} else if st.PendingUpdateType == string(mockmender.UpdateTypeCustomization) {
			// The real customization manager evaluates this after boot. The mock
			// only models manifest command health checks, not the /etc overlay.
			mockmender.EvaluateCustomizationHealth(&st, st.CustomizationHealthExpectedPass)
			if st.CustomizationHealthExpectedPass {
				fmt.Println("Core OS Customization health checks passed.")
			} else {
				fmt.Println("Core OS Customization health checks failed.")
			}
		} // Application transactions survive reboot for resume or rollback.
	case trial:
		// If not committed and rebooted again, rollback.
		mockmender.RollbackAfterFailedTrial(&st)
	default:
	}

	if err := mockmender.RotateBootID(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := mockmender.SaveState(st); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
