/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package bootid

import (
	"os"

	"github.com/ci4rail/moducop-core-api-server/internal/prefixfs"
)

const bootIDFilePath = "/proc/sys/kernel/random/boot_id"

func Get() (string, error) {
	data, err := os.ReadFile(prefixfs.Path(bootIDFilePath))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
