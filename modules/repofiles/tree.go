// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repofiles

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/setting"
	api "code.gitea.io/gitea/modules/structs"
)

// GetTreeBySHA get the GitTreeResponse of a repository using a sha hash.
func GetTreeBySHA(repo *models.Repository, sha string, page, perPage int, recursive bool) (*api.GitTreeResponse, error) {
	gitRepo, err := git.OpenRepository(repo.RepoPath())
	if err != nil {
		return nil, err
	}
	defer gitRepo.Close()
	gitTree, err := gitRepo.GetTree(sha)
	if err != nil || gitTree == nil {
		return nil, models.ErrSHANotFound{
			SHA: sha,
		}
	}
	tree := new(api.GitTreeResponse)
	tree.SHA = gitTree.ResolvedID.String()
	tree.URL = repo.APIURL() + "/git/trees/" + tree.SHA
	var entries git.Entries
	if recursive {
		entries, err = gitTree.ListEntriesRecursive()
	} else {
		entries, err = gitTree.ListEntries()
	}
	if err != nil {
		return nil, err
	}
	apiURL := repo.APIURL()
	apiURLLen := len(apiURL)

	// 51 is len(sha1) + len("/git/blobs/"). 40 + 11.
	blobURL := make([]byte, apiURLLen+51)
	copy(blobURL, apiURL)
	copy(blobURL[apiURLLen:], "/git/blobs/")

	// 51 is len(sha1) + len("/git/trees/"). 40 + 11.
	treeURL := make([]byte, apiURLLen+51)
	copy(treeURL, apiURL)
	copy(treeURL[apiURLLen:], "/git/trees/")

	// 40 is the size of the sha1 hash in hexadecimal format.
	copyPos := len(treeURL) - 40

	if perPage <= 0 || perPage > setting.API.DefaultGitTreesPerPage {
		perPage = setting.API.DefaultGitTreesPerPage
	}
	if page <= 0 {
		page = 1
	}
	tree.Page = page
	tree.TotalCount = len(entries)
	rangeStart := perPage * (page - 1)
	if rangeStart >= len(entries) {
		return tree, nil
	}
	var rangeEnd int
	if len(entries) > perPage {
		tree.Truncated = true
	}
	if rangeStart+perPage < len(entries) {
		rangeEnd = rangeStart + perPage
	} else {
		rangeEnd = len(entries)
	}
	tree.Entries = make([]api.GitEntry, rangeEnd-rangeStart)
	for e := rangeStart; e < rangeEnd; e++ {
		i := e - rangeStart

		tree.Entries[i].Path = entries[e].Name()
		tree.Entries[i].Mode = fmt.Sprintf("%06o", entries[e].Mode())
		tree.Entries[i].Type = entries[e].Type()
		tree.Entries[i].Size = entries[e].Size()
		tree.Entries[i].SHA = entries[e].ID.String()

		if entries[e].IsDir() {
			copy(treeURL[copyPos:], entries[e].ID.String())
			tree.Entries[i].URL = string(treeURL)
		} else {
			copy(blobURL[copyPos:], entries[e].ID.String())
			tree.Entries[i].URL = string(blobURL)
		}
	}
	return tree, nil
}

func WriteTree(repo *models.Repository, tree []*api.GitWriteTreeEntry, baseTreeSha string) (*api.GitWriteTreeResponse, error) {
	gitRepo, err := git.OpenRepository(repo.RepoPath())
	if err != nil {
		return nil, err
	}
	defer gitRepo.Close()

	// Initialize a map of tree entries.
	// This is so that we can overwrite and remove entries from the "base tree" if given.
	// This also prevents multiple entries with the same name from being placed into a tree,
	// which is a thing that can apparently happen but is not documented anywhere (ls-tree
	// reads back the tree as having multiple entries with the same file name but different
	// hashes, and checking out the tree seems to choose one arbitrarily).
	treeEntries := make(map[string]*git.TreeEntry)

	// Add entries from base tree if present
	if baseTreeSha != "" {
		baseTree, err := gitRepo.GetTree(baseTreeSha)
		if err != nil || baseTree == nil {
			return nil, models.ErrSHANotFound{
				SHA: baseTreeSha,
			}
		}

		entries, err := baseTree.ListEntries()
		if err != nil {
			return nil, err
		}

		for _, e := range entries {
			treeEntries[e.Name()] = e
		}
	}

	for _, e := range tree {
		if !ValidateUploadFileName(e.Name) {
			return nil, fmt.Errorf("invalid file name %s", e.Name)
		}
		mode, err := git.ToEntryMode(e.Mode)
		if err != nil {
			return nil, err
		}

		if e.SHA == "" {
			// SHA and content are null; delete the entry
			if e.Content == "" {
				delete(treeEntries, e.Name)
				continue
			}

			if !(mode == git.EntryModeBlob || mode == git.EntryModeExec || mode == git.EntryModeSymlink) {
				return nil, fmt.Errorf("file %s has content provided, but is not a blob, executable, or symlink", e.Name)
			}

			// Content was provided; store it
			content, err := base64.StdEncoding.DecodeString(e.Content)
			if err != nil {
				return nil, err
			}
			sha, err := gitRepo.HashObject(bytes.NewReader(content))
			if err != nil {
				return nil, err
			}
			entry := git.CreateTreeEntry(sha, e.Name, mode)
			treeEntries[entry.Name()] = &entry
			continue
		} else if e.Content != "" {
			return nil, errors.New("both content and SHA provided")
		}

		sha, err := git.NewIDFromString(e.SHA)
		if err != nil {
			return nil, err
		}

		entry := git.CreateTreeEntry(sha, e.Name, mode)

		treeEntries[entry.Name()] = &entry
	}

	entriesArr := make([]*git.TreeEntry, len(treeEntries))
	i := 0
	for _, e := range treeEntries {
		entriesArr[i] = e
		i++
	}

	sha, err := gitRepo.MkTree(entriesArr)
	if err != nil {
		if git.IsErrNotExist(err) {
			return nil, models.ErrSHANotFound{SHA: err.(git.ErrNotExist).ID}
		}
		return nil, err
	}
	shaString := sha.String()
	return &api.GitWriteTreeResponse{
		SHA: shaString,
		URL: repo.APIURL() + "/git/trees/" + shaString,
	}, nil
}
