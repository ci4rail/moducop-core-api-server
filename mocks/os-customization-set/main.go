/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ci4rail/moducop-core-api-server/mocks/mockmender"
)

type status struct {
	ActiveSlot        *string `json:"active_slot"`
	ActiveVersion     *string `json:"active_version"`
	LastGoodSlot      *string `json:"last_good_slot"`
	LastGoodVersion   *string `json:"last_good_version"`
	CandidateSlot     *string `json:"candidate_slot"`
	CandidateVersion  *string `json:"candidate_version"`
	CandidateState    *string `json:"candidate_state"`
	CandidateAttempts int     `json:"candidate_attempts"`
	FactoryVersion    *string `json:"factory_version"`
}

func main() {
	if len(os.Args) != 2 || os.Args[1] != "status" {
		fmt.Fprintln(os.Stderr, "Usage: os-customization-set status")
		os.Exit(2)
	}

	st, err := mockmender.LoadState()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	output := status{
		ActiveSlot:        stringOrNil(st.ActiveCustomizationSlot),
		ActiveVersion:     stringOrNil(st.ActiveCustomizationVersion),
		LastGoodSlot:      stringOrNil(st.LastGoodCustomizationSlot),
		LastGoodVersion:   stringOrNil(st.ActiveCustomizationVersion),
		CandidateSlot:     stringOrNil(st.CandidateCustomizationSlot),
		CandidateVersion:  stringOrNil(st.CandidateCustomizationVersion),
		CandidateState:    stringOrNil(st.CustomizationCandidateState),
		CandidateAttempts: st.CustomizationCandidateAttempts,
		FactoryVersion:    nil,
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func stringOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
