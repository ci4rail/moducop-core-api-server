/*
 * SPDX-FileCopyrightText: 2026 Ci4Rail GmbH
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package prefixfs

import "os"

func Path(p string) string {
	prefix := os.Getenv("MOCK_MENDER_STATE_DIR")
	if prefix != "" {
		prefix += "/fs"
	}
	return prefix + p
}
