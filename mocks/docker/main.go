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

const expectedPSFormat = "{{.Names}}\\t{{.Labels}}"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "ps":
		if err := runPS(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "compose":
		if err := runCompose(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  docker ps --format '{{.Names}}\\t{{.Labels}}'")
	fmt.Fprintln(os.Stderr, "  docker compose -p <project> down")
}

func runPS(args []string) error {
	if len(args) != 2 || args[0] != "--format" {
		return fmt.Errorf("unsupported docker ps arguments")
	}
	if args[1] != expectedPSFormat {
		return fmt.Errorf("unsupported docker ps format: %s", args[1])
	}

	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}
	for _, c := range st.RunningContainers {
		fmt.Printf("%s\t%s\n", c.Name, c.Labels)
	}
	return nil
}

func runCompose(args []string) error {
	if len(args) != 3 || args[0] != "-p" || args[2] != "down" {
		return fmt.Errorf("unsupported docker compose arguments")
	}
	project := args[1]

	st, err := mockmender.LoadState()
	if err != nil {
		return err
	}

	filtered := make([]mockmender.ContainerState, 0, len(st.RunningContainers))
	for _, c := range st.RunningContainers {
		if !hasProjectLabel(c.Labels, project) {
			filtered = append(filtered, c)
		}
	}
	st.RunningContainers = filtered

	if err := mockmender.SaveState(st); err != nil {
		return err
	}
	return nil
}

func hasProjectLabel(labels, project string) bool {
	needle := "com.docker.compose.project=" + project
	if labels == needle {
		return true
	}
	if len(labels) <= len(needle) {
		return false
	}
	if labels[:len(needle)] == needle && labels[len(needle)] == ',' {
		return true
	}
	return false
}
