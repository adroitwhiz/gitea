// Copyright 2018 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package structs

// GitEntry represents an existing git tree entry being returned
type GitEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	SHA  string `json:"sha"`
	URL  string `json:"url"`
}

// GitTreeResponse returns a git tree
type GitTreeResponse struct {
	SHA        string     `json:"sha"`
	URL        string     `json:"url"`
	Entries    []GitEntry `json:"tree"`
	Truncated  bool       `json:"truncated"`
	Page       int        `json:"page"`
	TotalCount int        `json:"total_count"`
}

// GitWriteTreeEntry represents a git tree entry to be written.
// You can provide either the SHA hash of an existing object, the base64-encoded contents of a new file to be written, or neither to delete the file from the tree.
// swagger:model GitWriteTreeEntry
type GitWriteTreeEntry struct {
	// The object's name.
	// required: true
	Name string `json:"name" binding:"Required"`
	// The file mode. This can be `100644` for a file, `100755` for an executable file, `120000` for a symlink,
	// `160000` for a commit/submodule, or `040000` for a directory/child tree.
	// required: true
	Mode string `json:"mode" binding:"Required;In(100644,100755,120000,160000,040000)"`
	// The SHA-1 hash of the referenced object. Provide either this or `content`.
	SHA string `json:"sha"`
	// The base64-encoded contents of the object. Provide either this or `sha`.
	Content string `json:"content"`
}

// GitWriteTreeOptions represents a request to write a git tree entry.
// swagger:model GitWriteTreeOptions
type GitWriteTreeOptions struct {
	// required: true
	Tree []*GitWriteTreeEntry `json:"tree" binding:"Required"`
	// The SHA hash of an existing tree. If provided, this tree's entries will be merged with it, overwriting or deleting entries from it.
	BaseTree string `json:"base_tree"`
}

// GitWriteTreeResponse is returned when a git tree is written
type GitWriteTreeResponse struct {
	SHA string `json:"sha"`
	URL string `json:"url"`
}
