//go:build !linux

// SPDX-License-Identifier: Apache-2.0

package systemd

// booted is false anywhere systemd cannot be the init system.
func booted() bool { return false }
