// SPDX-License-Identifier: Apache-2.0

package document

import (
	"fmt"
	"sort"
	"strings"
)

// Error is one problem found in a document, tied to where it was read from.
type Error struct {
	Position Position
	Msg      string
}

func (e Error) Error() string {
	return e.Position.String() + ": " + e.Msg
}

// Errors collects problems, so a repository with four mistakes takes one run to find
// them rather than four.
type Errors struct {
	list []Error
}

func (e *Errors) Add(pos Position, format string, args ...any) {
	e.list = append(e.list, Error{Position: pos, Msg: fmt.Sprintf(format, args...)})
}

func (e *Errors) Extend(other Errors) {
	e.list = append(e.list, other.list...)
}

func (e *Errors) Len() int { return len(e.list) }

func (e *Errors) List() []Error {
	sort.SliceStable(e.list, func(i, j int) bool {
		if e.list[i].Position.File != e.list[j].Position.File {
			return e.list[i].Position.File < e.list[j].Position.File
		}
		return e.list[i].Position.Line < e.list[j].Position.Line
	})
	return e.list
}

// Err returns nil when nothing was collected.
func (e *Errors) Err() error {
	if len(e.list) == 0 {
		return nil
	}
	return e
}

func (e *Errors) Error() string {
	var b strings.Builder
	for i, item := range e.List() {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(item.Error())
	}
	return b.String()
}
