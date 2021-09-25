// Copyright 2015 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"regexp"
	"strings"
)

// NewTree create a new tree according the repository and tree id
func NewTree(repo *Repository, id SHA1) *Tree {
	return &Tree{
		ID:   id,
		repo: repo,
	}
}

// SubTree get a sub tree by the sub dir path
func (t *Tree) SubTree(rpath string) (*Tree, error) {
	if len(rpath) == 0 {
		return t, nil
	}

	paths := strings.Split(rpath, "/")
	var (
		err error
		g   = t
		p   = t
		te  *TreeEntry
	)
	for _, name := range paths {
		te, err = p.GetTreeEntryByPath(name)
		if err != nil {
			return nil, err
		}

		g, err = t.repo.getTree(te.ID)
		if err != nil {
			return nil, err
		}
		g.ptree = p
		p = g
	}
	return g, nil
}

// LsTree checks if the given filenames are in the tree
func (repo *Repository) LsTree(ref string, filenames ...string) ([]string, error) {
	cmd := NewCommand("ls-tree", "-z", "--name-only", "--", ref)
	for _, arg := range filenames {
		if arg != "" {
			cmd.AddArguments(arg)
		}
	}
	res, err := cmd.RunInDirBytes(repo.Path)
	if err != nil {
		return nil, err
	}
	filelist := make([]string, 0, len(filenames))
	for _, line := range bytes.Split(res, []byte{'\000'}) {
		filelist = append(filelist, string(line))
	}

	return filelist, err
}

var objectUnavailable = regexp.MustCompile(`fatal: entry .* object ([0-9a-f]+) is unavailable`)

// MkTree creates a new tree from tree entries
func (repo *Repository) MkTree(entries []*TreeEntry) (SHA1, error) {
	cmd := NewCommand("mktree", "-z")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	buffer := new(bytes.Buffer)
	for _, entry := range entries {
		buffer.WriteString(entry.Mode().String())
		buffer.WriteString(" ")
		buffer.WriteString(entry.Type())
		buffer.WriteString(" ")
		buffer.WriteString(entry.ID.String())
		buffer.WriteString("\t")
		buffer.WriteString(entry.Name())
		buffer.WriteByte('\000')
	}

	err := cmd.RunInDirFullPipeline(repo.Path, stdout, stderr, bytes.NewReader(buffer.Bytes()))
	if err != nil {
		errString := stderr.String()
		if missingSha := objectUnavailable.FindStringSubmatch(errString); missingSha != nil {
			return SHA1{}, ErrNotExist{ID: missingSha[0]}
		}
		return SHA1{}, ConcatenateError(err, errString)
	}
	sha, err := NewIDFromString(strings.TrimSpace(stdout.String()))
	if err != nil {
		return SHA1{}, err
	}

	return sha, nil
}
